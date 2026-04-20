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

package spawn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
	terminalrpc "github.com/eminwux/sbsh/pkg/rpcclient/terminal"
)

// TerminalOptions configures NewTerminal.
type TerminalOptions struct {
	// BinaryPath is the absolute path to an sbsh binary that supports
	// the "terminal -" subcommand (reads a TerminalSpec JSON on stdin).
	// Required — spawn does not consult $PATH so callers can't
	// accidentally cross a state-root boundary.
	BinaryPath string

	// ExtraArgs, if non-empty, are inserted before the "terminal" /
	// "-" args (useful for e.g. "--run-path <x>" when the caller
	// wants to override the root flag rather than rely on the spec).
	ExtraArgs []string

	// Env is the environment for the spawned process. If nil, the
	// child inherits the parent's os.Environ(). To start the child
	// with an empty environment, pass an explicit empty slice.
	Env []string

	// Stdout / Stderr receive the child's stdout/stderr. If nil, the
	// respective stream is redirected to /dev/null. Stdin is not
	// exposed — the TerminalSpec JSON is written there.
	Stdout io.Writer
	Stderr io.Writer

	// Logger is used for spawn-side diagnostic logging. nil = quiet.
	Logger *slog.Logger

	// ReadyTimeout caps how long TerminalHandle.WaitReady will poll
	// before returning ErrReadyTimeout. 0 (the default) disables the
	// internal cap and defers entirely to the ctx passed to
	// WaitReady.
	ReadyTimeout time.Duration

	// ReadyPollInterval sets how frequently WaitReady tries to dial
	// and Ping the terminal's control socket. 0 means use a sensible
	// default.
	ReadyPollInterval time.Duration

	// StopGracePeriod caps each step of the Close escalation
	// (Stop RPC, SIGTERM, SIGKILL). 0 uses the default.
	StopGracePeriod time.Duration
}

// TerminalHandle is a library-side handle on a spawned Terminal
// subprocess. Consumers can observe lifecycle via WaitReady/WaitClose,
// request shutdown via Close, and drive the RPC surface by connecting
// to SocketPath() with pkg/rpcclient/terminal.
type TerminalHandle struct {
	spec       *api.TerminalSpec
	socketPath string

	opts TerminalOptions
	proc *process
}

// NewTerminal spawns `<opts.BinaryPath> terminal -` with spec encoded
// as JSON on stdin and the child detached via Setsid. The returned
// handle is not ready yet — call WaitReady to block until the
// terminal's control socket accepts RPC calls.
func NewTerminal(ctx context.Context, spec *api.TerminalSpec, opts TerminalOptions) (*TerminalHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateTerminalInputs(spec, opts); err != nil {
		return nil, err
	}

	specBytes, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("marshal terminal spec: %w", err)
	}

	const terminalArgCount = 2 // "terminal", "-"
	args := make([]string, 0, len(opts.ExtraArgs)+terminalArgCount)
	args = append(args, opts.ExtraArgs...)
	args = append(args, "terminal", "-")

	// Deliberately using exec.Command (not CommandContext) — the child
	// is detached (Setsid) and must outlive the caller's ctx.
	//nolint:gosec,noctx // Caller controls BinaryPath and ExtraArgs; spawn
	// explicitly does not consult $PATH.
	cmd := exec.Command(opts.BinaryPath, args...)
	cmd.Stdin = bytes.NewReader(specBytes)
	if opts.Env != nil {
		cmd.Env = opts.Env
	} else {
		cmd.Env = os.Environ()
	}

	// stdin is already wired to the spec JSON; resolveStdio only
	// handles stdout/stderr fallback to /dev/null.
	opened, err := resolveStdio(cmd, cmd.Stdin, opts.Stdout, opts.Stderr)
	if err != nil {
		return nil, fmt.Errorf("wire stdio: %w", err)
	}
	defer closeFiles(opened)

	proc, err := startProcess(cmd, opts.Logger)
	if err != nil {
		return nil, fmt.Errorf("start terminal process: %w", err)
	}

	return &TerminalHandle{
		spec:       spec,
		socketPath: spec.SocketFile,
		opts:       opts,
		proc:       proc,
	}, nil
}

func validateTerminalInputs(spec *api.TerminalSpec, opts TerminalOptions) error {
	if opts.BinaryPath == "" {
		return ErrBinaryPathRequired
	}
	if spec == nil {
		return ErrSpecRequired
	}
	if spec.SocketFile == "" {
		return ErrSocketPathRequired
	}
	return nil
}

// PID returns the OS PID of the spawned terminal process.
func (h *TerminalHandle) PID() int { return h.proc.pid() }

// SocketPath returns the control-socket path for the terminal; the
// same value that appears in spec.SocketFile.
func (h *TerminalHandle) SocketPath() string { return h.socketPath }

// Spec returns the TerminalSpec used to spawn this handle. The
// returned pointer is shared — callers must not mutate it.
func (h *TerminalHandle) Spec() *api.TerminalSpec { return h.spec }

// WaitReady blocks until the spawned terminal's RPC server accepts a
// Ping on its control socket. It returns early with ErrProcessExited
// if the child dies before reaching Ready, ErrReadyTimeout if the
// internal cap elapses, or ctx.Err() on cancellation.
func (h *TerminalHandle) WaitReady(ctx context.Context) error {
	if h.proc.exited() {
		return fmt.Errorf("%w: %w", ErrProcessExited, h.proc.takeExitErr())
	}

	readyCtx, cancel := h.readyContext(ctx)
	defer cancel()

	poll := h.opts.ReadyPollInterval
	if poll <= 0 {
		poll = defaultReadyPollInterval
	}

	rpc := terminalrpc.NewUnix(h.socketPath, h.readyLogger())

	for {
		if h.proc.exited() {
			return fmt.Errorf("%w: %w", ErrProcessExited, h.proc.takeExitErr())
		}

		if _, err := os.Stat(h.socketPath); err == nil {
			pingCtx, pingCancel := context.WithTimeout(readyCtx, poll)
			pingErr := rpc.Ping(pingCtx, &api.PingMessage{Message: "spawn.WaitReady"}, &api.PingMessage{})
			pingCancel()
			if pingErr == nil {
				return nil
			}
			if h.opts.Logger != nil {
				h.opts.Logger.DebugContext(readyCtx, "terminal not ready yet",
					"socket", h.socketPath, "error", pingErr)
			}
		}

		select {
		case <-readyCtx.Done():
			if errors.Is(readyCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
				return ErrReadyTimeout
			}
			return ctx.Err()
		case <-h.proc.exitCh:
			return fmt.Errorf("%w: %w", ErrProcessExited, h.proc.takeExitErr())
		case <-time.After(poll):
		}
	}
}

func (h *TerminalHandle) readyContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if h.opts.ReadyTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, h.opts.ReadyTimeout)
}

func (h *TerminalHandle) readyLogger() *slog.Logger {
	if h.opts.Logger != nil {
		return h.opts.Logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

const defaultReadyPollInterval = 100 * time.Millisecond

// Close requests graceful shutdown of the terminal process: it first
// issues a Stop RPC over the control socket, then escalates to SIGTERM
// and SIGKILL if the child is still alive. Returns the process exit
// error (nil on clean exit) or ctx.Err() if the caller's context is
// canceled mid-shutdown.
func (h *TerminalHandle) Close(ctx context.Context) error {
	return h.proc.gracefulShutdown(ctx, h.opts.StopGracePeriod, h.sendStopRPC)
}

func (h *TerminalHandle) sendStopRPC(ctx context.Context) error {
	if _, err := os.Stat(h.socketPath); err != nil {
		// No socket — nothing to RPC. Escalate to signals.
		return err
	}
	rpc := terminalrpc.NewUnix(h.socketPath, h.readyLogger())
	return rpc.Stop(ctx, &api.StopArgs{Reason: "spawn.Close"})
}

// WaitClose blocks until the terminal subprocess has fully exited or
// ctx is canceled. It returns the process exit error (nil on clean
// exit) or ctx.Err().
func (h *TerminalHandle) WaitClose(ctx context.Context) error {
	return h.proc.waitExit(ctx)
}
