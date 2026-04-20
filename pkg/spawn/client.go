// Copyright 2025 Emiliano Spinella (eminwux)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package spawn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
	clientrpc "github.com/eminwux/sbsh/pkg/rpcclient/client"
)

// ClientOptions configures NewClient.
type ClientOptions struct {
	// BinaryPath is the absolute path to an sb binary that supports the
	// "attach" subcommand. Required — spawn does not consult $PATH so
	// callers can't accidentally cross a state-root boundary.
	BinaryPath string

	// ExtraArgs, if non-empty, are inserted before "attach" (useful
	// for e.g. "--verbose" or overriding root-level flags).
	ExtraArgs []string

	// Env is the environment for the spawned process. nil = inherit
	// the parent's os.Environ(). An explicit empty slice means "start
	// with no environment variables."
	Env []string

	// Stdin/Stdout/Stderr are wired to the child. Nil streams are
	// redirected to /dev/null. For interactive use the caller
	// typically hands in a PTY pair; for headless RPC-driven use,
	// leaving them nil is fine.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Logger is used for spawn-side diagnostic logging. nil = quiet.
	Logger *slog.Logger

	// ReadyTimeout caps how long ClientHandle.WaitReady waits for the
	// control socket to appear. 0 (the default) defers to the ctx
	// passed to WaitReady.
	ReadyTimeout time.Duration

	// ReadyPollInterval sets how frequently WaitReady checks for the
	// control socket. 0 means use a sensible default.
	ReadyPollInterval time.Duration

	// StopGracePeriod caps each step of the Close escalation
	// (Detach RPC, SIGTERM, SIGKILL). 0 uses defaultGracePeriod (2s)
	// per step, giving ~6s wall-clock before SIGKILL lands.
	StopGracePeriod time.Duration
}

// ClientHandle is a library-side handle on a spawned Client
// subprocess. Consumers can observe lifecycle via WaitReady/WaitClose,
// request shutdown via Close, and connect to SocketPath() for the
// subset of ClientController RPCs currently exposed (Detach today;
// Ping/State/Stop land in PR-E).
type ClientHandle struct {
	doc        *api.ClientDoc
	socketPath string

	opts ClientOptions
	proc *process
}

// NewClient spawns `<opts.BinaryPath> attach` with flags derived from
// doc, backed by a detached (Setsid) process. Only
// ClientMode=AttachToTerminal is supported in this release; compose
// NewTerminal + NewClient for the run-new-terminal flow.
//
// doc.Spec.SockerCtrl must be set to the desired control-socket path
// (spawn does not invent one); doc.Spec.TerminalSpec must carry either
// an ID or a Name identifying the terminal to attach to.
func NewClient(ctx context.Context, doc *api.ClientDoc, opts ClientOptions) (*ClientHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateClientInputs(doc, opts); err != nil {
		return nil, err
	}

	args := buildClientAttachArgs(doc, opts)

	// Deliberately using exec.Command (not CommandContext) — the
	// child is detached (Setsid) and must outlive the caller's ctx.
	//nolint:gosec,noctx // Caller controls BinaryPath and ExtraArgs; spawn
	// explicitly does not consult $PATH.
	cmd := exec.Command(opts.BinaryPath, args...)
	if opts.Env != nil {
		cmd.Env = opts.Env
	} else {
		cmd.Env = os.Environ()
	}

	opened, err := resolveStdio(cmd, opts.Stdin, opts.Stdout, opts.Stderr)
	if err != nil {
		return nil, fmt.Errorf("wire stdio: %w", err)
	}
	defer closeFiles(opened)

	proc, err := startProcess(cmd, opts.Logger)
	if err != nil {
		return nil, fmt.Errorf("start client process: %w", err)
	}

	return &ClientHandle{
		doc:        doc,
		socketPath: doc.Spec.SockerCtrl,
		opts:       opts,
		proc:       proc,
	}, nil
}

func validateClientInputs(doc *api.ClientDoc, opts ClientOptions) error {
	if opts.BinaryPath == "" {
		return ErrBinaryPathRequired
	}
	if doc == nil {
		return ErrDocRequired
	}
	if doc.Spec.ClientMode != api.AttachToTerminal {
		return fmt.Errorf("%w: only AttachToTerminal is supported", ErrUnsupportedClientMode)
	}
	if doc.Spec.SockerCtrl == "" {
		return ErrSocketPathRequired
	}
	if doc.Spec.TerminalSpec == nil ||
		(doc.Spec.TerminalSpec.ID == "" && doc.Spec.TerminalSpec.Name == "") {
		return ErrAttachTargetRequired
	}
	return nil
}

// buildClientAttachArgs translates a ClientDoc into the CLI argv for
// `sb [--run-path X] attach --socket S [--id I | <name>] [--disable-detach]`.
//
// The `--socket` flag is documented on `sb attach` as "for the
// terminal" but at runtime it sets the *client's* control socket
// (see cmd/sb/attach/attach.go:161,210). We rely on that actual
// behavior so the parent process can predict ClientHandle.SocketPath();
// the TerminalSpec.SocketFile in the child's doc is overridden from
// metadata.json during discovery in internal/client.createAttachTerminal,
// so the flag's conflated double-use does not break attach.
// If --id and --name are both set the CLI rejects the combination,
// so spawn deterministically prefers ID (matches CLI precedence).
func buildClientAttachArgs(doc *api.ClientDoc, opts ClientOptions) []string {
	const maxAttachArgs = 8 // --run-path X attach --socket S --id I --disable-detach
	args := make([]string, 0, maxAttachArgs+len(opts.ExtraArgs))

	if doc.Spec.RunPath != "" {
		args = append(args, "--run-path", doc.Spec.RunPath)
	}
	args = append(args, opts.ExtraArgs...)
	args = append(args, "attach", "--socket", doc.Spec.SockerCtrl)

	if !doc.Spec.DetachKeystroke {
		args = append(args, "--disable-detach")
	}

	term := doc.Spec.TerminalSpec
	switch {
	case term.ID != "":
		args = append(args, "--id", string(term.ID))
	case term.Name != "":
		args = append(args, term.Name)
	}

	return args
}

// PID returns the OS PID of the spawned client process.
func (h *ClientHandle) PID() int { return h.proc.pid() }

// SocketPath returns the client's control-socket path.
func (h *ClientHandle) SocketPath() string { return h.socketPath }

// Doc returns the ClientDoc used to spawn this handle. The returned
// pointer is shared — callers must not mutate it.
func (h *ClientHandle) Doc() *api.ClientDoc { return h.doc }

// WaitReady blocks until the client's control socket file is visible
// on disk. Until PR-E grows ClientController with a Ping RPC, socket
// presence is the best readiness signal spawn can offer without
// peeking at internal state. Caveat: the file appears the instant the
// Unix listener binds, which is slightly before the client has fully
// wired up its attach to the terminal. Callers that race into a
// Detach() RPC immediately after WaitReady may observe a transient
// connection refused; retry or use a small back-off until PR-E.
func (h *ClientHandle) WaitReady(ctx context.Context) error {
	if h.proc.exited() {
		return wrapProcessExited(h.proc.takeExitErr())
	}

	readyCtx, cancel := h.readyContext(ctx)
	defer cancel()

	poll := h.opts.ReadyPollInterval
	if poll <= 0 {
		poll = defaultReadyPollInterval
	}

	for {
		if h.proc.exited() {
			return wrapProcessExited(h.proc.takeExitErr())
		}
		if _, err := os.Stat(h.socketPath); err == nil {
			return nil
		}

		select {
		case <-readyCtx.Done():
			if errors.Is(readyCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
				return ErrReadyTimeout
			}
			return ctx.Err()
		case <-h.proc.exitCh:
			return wrapProcessExited(h.proc.takeExitErr())
		case <-time.After(poll):
		}
	}
}

func (h *ClientHandle) readyContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if h.opts.ReadyTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, h.opts.ReadyTimeout)
}

// Close requests graceful shutdown: it first sends a Detach RPC over
// the client's control socket (which causes the client to disconnect
// from its terminal and exit), then escalates to SIGTERM and SIGKILL
// if the child is still alive. Returns the process exit error (nil on
// clean exit) or ctx.Err() if the caller's context is canceled
// mid-shutdown.
//
// Once PR-E grows ClientController with an explicit Stop RPC, this
// method will switch to it for a cleaner shutdown signal.
func (h *ClientHandle) Close(ctx context.Context) error {
	return h.proc.gracefulShutdown(ctx, h.opts.StopGracePeriod, h.sendDetachRPC)
}

func (h *ClientHandle) sendDetachRPC(ctx context.Context) error {
	if _, err := os.Stat(h.socketPath); err != nil {
		return err
	}
	rpc := clientrpc.NewUnix(h.socketPath)
	return rpc.Detach(ctx)
}

// WaitClose blocks until the client subprocess has fully exited or
// ctx is canceled. It returns the process exit error (nil on clean
// exit) or ctx.Err().
func (h *ClientHandle) WaitClose(ctx context.Context) error {
	return h.proc.waitExit(ctx)
}
