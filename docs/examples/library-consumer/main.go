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

// library-consumer is a minimal end-to-end demo of driving sbsh as a
// Go library from an external module. It builds a TerminalSpec, spawns
// a detached Terminal, then exercises one of two facades against it:
//
//   - mode=rpc (default): spawns a Client that attaches to the
//     terminal, exchanges one Write/Read round-trip over the Terminal
//     RPC surface, drives a State/Stop round-trip on the Client RPC
//     surface, and tears both processes down via spawn.Handle.Close.
//   - mode=attach: drives the in-process attach façade by calling
//     pkg/attach.Run against the terminal's control socket with the
//     caller's TTY-backed stdio. The session ends on the detach
//     keystroke (^] twice), the remote terminal closing, or
//     SIGINT/SIGTERM, after which the terminal is torn down.
//
// The whole run is rooted at a caller-supplied StateRoot and never
// writes under $HOME/.sbsh.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
	"github.com/eminwux/sbsh/pkg/attach"
	"github.com/eminwux/sbsh/pkg/builder"
	clientrpc "github.com/eminwux/sbsh/pkg/rpcclient/client"
	terminalrpc "github.com/eminwux/sbsh/pkg/rpcclient/terminal"
	"github.com/eminwux/sbsh/pkg/spawn"
)

const (
	modeRPC    = "rpc"
	modeAttach = "attach"
)

type flags struct {
	sbshPath  string
	sbPath    string
	stateRoot string
	mode      string
	verbose   bool
}

func parseFlags() flags {
	var f flags
	flag.StringVar(&f.sbshPath, "sbsh", "", "absolute path to the sbsh binary (required)")
	flag.StringVar(&f.sbPath, "sb", "", "absolute path to the sb binary (defaults to <dir(sbsh)>/sb)")
	flag.StringVar(&f.stateRoot, "state-root", "",
		"run path for sbsh state (defaults to a fresh directory under $TMPDIR)")
	flag.StringVar(&f.mode, "mode", modeRPC,
		"demo mode: \"rpc\" (spawn sb client, drive RPC) or \"attach\" (drive pkg/attach.Run with TTY stdio)")
	flag.BoolVar(&f.verbose, "v", false, "log spawn/RPC internals at debug level")
	flag.Parse()
	return f
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "library-consumer:", err)
		os.Exit(1)
	}
}

func run() error {
	f := parseFlags()

	switch f.mode {
	case modeRPC, modeAttach:
	default:
		return fmt.Errorf("-mode must be %q or %q (got %q)", modeRPC, modeAttach, f.mode)
	}

	sbshPath, sbPath, err := resolveBinaries(f.sbshPath, f.sbPath, f.mode)
	if err != nil {
		return err
	}

	stateRoot, cleanup, err := resolveStateRoot(f.stateRoot)
	if err != nil {
		return err
	}
	defer cleanup()

	logger := buildLogger(f.verbose)
	logger.Info("library-consumer starting",
		"mode", f.mode, "sbsh", sbshPath, "sb", sbPath, "stateRoot", stateRoot)

	if f.mode == modeAttach {
		return runAttach(logger, stateRoot, sbshPath)
	}
	return runRPC(logger, stateRoot, sbshPath, sbPath)
}

// runRPC spawns a Terminal and a Client subprocess, then drives the
// Terminal/Client RPC surfaces end-to-end. The 60s ceiling matches the
// scripted nature of this path — the whole round-trip should complete
// promptly.
func runRPC(logger *slog.Logger, stateRoot, sbshPath, sbPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	term, err := startTerminal(ctx, logger, stateRoot, sbshPath)
	if err != nil {
		return fmt.Errorf("start terminal: %w", err)
	}
	defer closeWithLog(ctx, logger, "terminal", term)

	cli, err := startClient(ctx, logger, stateRoot, sbPath, term.Spec())
	if err != nil {
		return fmt.Errorf("start client: %w", err)
	}
	defer closeWithLog(ctx, logger, "client", cli)

	if err := driveTerminalRPC(ctx, logger, term); err != nil {
		return fmt.Errorf("terminal RPC round-trip: %w", err)
	}

	if err := driveClientRPC(ctx, logger, cli); err != nil {
		return fmt.Errorf("client RPC round-trip: %w", err)
	}

	logger.Info("library-consumer completed successfully")
	return nil
}

// runAttach spawns a Terminal and drives pkg/attach.Run against its
// control socket using the caller's TTY stdio. No timeout: the
// session is open-ended and ends on detach keystroke, remote close,
// or SIGINT/SIGTERM (mirroring cmd/sb/attach's signal policy — see
// pkg/attach/doc.go on why the package itself does not trap signals).
func runAttach(logger *slog.Logger, stateRoot, sbshPath string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	term, err := startTerminal(ctx, logger, stateRoot, sbshPath)
	if err != nil {
		return fmt.Errorf("start terminal: %w", err)
	}
	defer closeWithLog(ctx, logger, "terminal", term)

	if err := driveAttach(ctx, logger, term); err != nil {
		return fmt.Errorf("attach loop: %w", err)
	}

	logger.Info("library-consumer completed successfully")
	return nil
}

// resolveBinaries validates the sbsh/sb paths and fills sb from
// filepath.Dir(sbsh)/sb when the caller omits it. sbsh must always
// exist (spawn.NewTerminal does not consult $PATH); sb is only
// required in RPC mode, where spawn.NewClient execs it. Attach mode
// drives pkg/attach.Run in-process and never touches sb.
func resolveBinaries(sbsh, sb, mode string) (string, string, error) {
	if sbsh == "" {
		return "", "", errors.New("-sbsh is required (absolute path to the sbsh binary)")
	}
	abs, err := filepath.Abs(sbsh)
	if err != nil {
		return "", "", fmt.Errorf("resolve -sbsh: %w", err)
	}
	sbsh = abs
	if err := mustBeRegularFile(sbsh); err != nil {
		return "", "", fmt.Errorf("sbsh binary: %w", err)
	}

	if mode == modeAttach {
		return sbsh, "", nil
	}

	if sb == "" {
		sb = filepath.Join(filepath.Dir(sbsh), "sb")
	}
	absSb, err := filepath.Abs(sb)
	if err != nil {
		return "", "", fmt.Errorf("resolve -sb: %w", err)
	}
	sb = absSb
	if err := mustBeRegularFile(sb); err != nil {
		return "", "", fmt.Errorf("sb binary: %w", err)
	}
	return sbsh, sb, nil
}

func mustBeRegularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q is not a regular file", path)
	}
	return nil
}

// resolveStateRoot returns an absolute run path plus a cleanup func
// that removes it at shutdown when the example created it itself.
// If the caller supplies a path we use it verbatim and leave it in
// place — the caller is presumed to own that directory.
func resolveStateRoot(supplied string) (string, func(), error) {
	if supplied != "" {
		abs, err := filepath.Abs(supplied)
		if err != nil {
			return "", func() {}, fmt.Errorf("resolve -state-root: %w", err)
		}
		if err := os.MkdirAll(abs, 0o700); err != nil {
			return "", func() {}, fmt.Errorf("create -state-root: %w", err)
		}
		return abs, func() {}, nil
	}
	dir, err := os.MkdirTemp("", "sbsh-lib-example-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temp state root: %w", err)
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

func buildLogger(verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

// startTerminal builds a TerminalSpec inline (no profile lookup, no
// $HOME/.sbsh access) and spawns the terminal as a detached subprocess.
// It blocks until the terminal's control socket accepts RPC calls.
func startTerminal(
	ctx context.Context,
	logger *slog.Logger,
	stateRoot, sbshPath string,
) (*spawn.TerminalHandle, error) {
	spec, err := builder.BuildTerminalSpec(ctx, logger, stateRoot,
		builder.WithName("library-consumer-example"),
		builder.WithCommand([]string{"/bin/bash", "--norc", "--noprofile"}),
		builder.WithEnv(map[string]string{"PS1": "example> "}),
		builder.WithDisableSetPrompt(true),
	)
	if err != nil {
		return nil, fmt.Errorf("build terminal spec: %w", err)
	}
	logger.Info("terminal spec built",
		"id", spec.ID, "name", spec.Name, "socket", spec.SocketFile)

	term, err := spawn.NewTerminal(ctx, spec, spawn.TerminalOptions{
		BinaryPath:   sbshPath,
		ExtraArgs:    []string{"--run-path", stateRoot},
		Logger:       logger,
		ReadyTimeout: 15 * time.Second,
		// Stdout/Stderr intentionally nil so the child's output goes to
		// /dev/null; the capture file and log file under stateRoot hold
		// the real detail.
	})
	if err != nil {
		return nil, err
	}
	logger.Info("terminal spawned", "pid", term.PID(), "socket", term.SocketPath())

	if err := term.WaitReady(ctx); err != nil {
		_ = term.Close(ctx)
		return nil, fmt.Errorf("waitReady: %w", err)
	}
	logger.Info("terminal ready")
	return term, nil
}

// startClient builds a ClientDoc in AttachToTerminal mode, spawns the
// client process, and blocks until its control socket is ready.
func startClient(
	ctx context.Context,
	logger *slog.Logger,
	stateRoot, sbPath string,
	termSpec *api.TerminalSpec,
) (*spawn.ClientHandle, error) {
	doc, err := builder.BuildClientDoc(ctx, logger, stateRoot,
		builder.WithClientName("library-consumer-client"),
		builder.WithClientMode(api.AttachToTerminal),
		// Attach by Name only. The underlying spawn helper would also
		// accept ID, but the `sb attach` CLI currently requires a
		// positional name (the --id flag is rejected alongside
		// positional args and without them returns
		// ErrNoTerminalIdentifier), so Name is the path that works
		// end-to-end today.
		builder.WithClientTerminalSpec(&api.TerminalSpec{
			Name: termSpec.Name,
		}),
		// Disable the interactive detach keystroke — this client is
		// RPC-driven, not TTY-driven.
		builder.WithClientDetachKeystroke(false),
	)
	if err != nil {
		return nil, fmt.Errorf("build client doc: %w", err)
	}
	logger.Info("client doc built",
		"id", doc.Spec.ID, "name", doc.Metadata.Name, "socket", doc.Spec.SockerCtrl)

	cli, err := spawn.NewClient(ctx, doc, spawn.ClientOptions{
		BinaryPath:   sbPath,
		ExtraArgs:    []string{"--run-path", stateRoot},
		Logger:       logger,
		ReadyTimeout: 15 * time.Second,
		// Stdin/Stdout/Stderr intentionally nil — this client is
		// RPC-driven, not TTY-driven, and the per-client log under
		// stateRoot holds the detail.
	})
	if err != nil {
		return nil, err
	}
	logger.Info("client spawned", "pid", cli.PID(), "socket", cli.SocketPath())

	if err := cli.WaitReady(ctx); err != nil {
		_ = cli.Close(ctx)
		return nil, fmt.Errorf("waitReady: %w", err)
	}
	logger.Info("client ready")
	return cli, nil
}

// driveTerminalRPC demonstrates the Write/Read round-trip: it
// subscribes to the terminal's live byte stream, writes one shell
// command, and drains a bounded window of output until the marker
// echo lands or the deadline elapses.
func driveTerminalRPC(
	ctx context.Context,
	logger *slog.Logger,
	term *spawn.TerminalHandle,
) error {
	trpc := terminalrpc.NewUnix(term.SocketPath(), logger)

	subCtx, subCancel := context.WithTimeout(ctx, 5*time.Second)
	defer subCancel()
	stream, err := trpc.Subscribe(
		subCtx,
		&api.SubscribeRequest{ClientID: api.ID("library-consumer")},
		&api.Empty{},
	)
	if err != nil {
		return fmt.Errorf("Subscribe: %w", err)
	}
	defer stream.Close()

	const marker = "sbsh-library-consumer-marker"
	payload := []byte(fmt.Sprintf("echo %s\n", marker))
	writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
	defer writeCancel()
	if err := trpc.Write(writeCtx, &api.WriteRequest{Data: payload}); err != nil {
		return fmt.Errorf("Write: %w", err)
	}
	logger.Info("wrote to terminal", "bytes", len(payload))

	deadline := time.Now().Add(5 * time.Second)
	_ = stream.SetReadDeadline(deadline)
	var seen strings.Builder
	buf := make([]byte, 4096)
	for time.Now().Before(deadline) {
		n, readErr := stream.Read(buf)
		if n > 0 {
			seen.Write(buf[:n])
			if strings.Contains(seen.String(), marker) {
				logger.Info("observed marker in terminal output", "marker", marker)
				return nil
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return errors.New("terminal stream closed before marker arrived")
			}
			return fmt.Errorf("read subscribe stream: %w", readErr)
		}
	}
	return fmt.Errorf("marker %q not observed within deadline; got %q", marker, seen.String())
}

// driveAttach demonstrates the in-process attach façade: it points
// pkg/attach.Run at the spawned terminal's control socket using the
// process's own TTY-backed stdio. The call blocks until the user
// detaches (^]^]), the remote terminal closes, or ctx is cancelled
// (e.g. via SIGINT/SIGTERM). pkg/attach already classifies
// ctx-cancellation as a graceful exit (errdefs.ErrContextDone), so
// callers wrap that as nil to keep the program's exit code clean.
//
// The TTY precheck below is a UX courtesy: pkg/attach.Run requires a
// TTY-backed *os.File for stdin (it puts the fd in raw mode and
// reads window size via TIOCGWINSZ — see pkg/attach/doc.go). For
// headless contexts (CI runners, non-interactive shells), allocate a
// PTY pair from your test harness and pass the slave end as
// Options.Stdin/Stdout — the github.com/creack/pty package is the
// idiomatic choice; this example deliberately avoids that dep to
// keep the module footprint minimal.
func driveAttach(
	ctx context.Context,
	logger *slog.Logger,
	term *spawn.TerminalHandle,
) error {
	if !isTerminal(os.Stdin) {
		return errors.New(
			"-mode=attach requires stdin to be a TTY; run from an interactive shell or " +
				"see the README's headless-context note for how to wire a PTY pair",
		)
	}

	socketPath := term.SocketPath()
	logger.Info("driving pkg/attach.Run", "socket", socketPath)

	runErr := attach.Run(ctx, attach.Options{
		SocketPath: socketPath,
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Logger:     logger,
	})
	switch {
	case runErr == nil:
		return nil
	case errors.Is(runErr, context.Canceled):
		return nil
	default:
		return runErr
	}
}

// isTerminal reports whether f is a character device, the same gate
// pkg/attach.Run later applies via x/term.MakeRaw. Using
// os.ModeCharDevice keeps this example free of an extra dep on
// golang.org/x/term.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// driveClientRPC exercises ClientController.State and Stop. Calling
// Stop tears the client process down cleanly; the handle's deferred
// Close in main() then becomes a no-op safety net.
func driveClientRPC(
	ctx context.Context,
	logger *slog.Logger,
	cli *spawn.ClientHandle,
) error {
	crpc := clientrpc.NewUnix(cli.SocketPath())

	stateCtx, stateCancel := context.WithTimeout(ctx, 5*time.Second)
	defer stateCancel()
	var state api.ClientStatusMode
	if err := crpc.State(stateCtx, &state); err != nil {
		return fmt.Errorf("State: %w", err)
	}
	logger.Info("client state observed", "state", state.String())

	stopCtx, stopCancel := context.WithTimeout(ctx, 5*time.Second)
	defer stopCancel()
	if err := crpc.Stop(stopCtx, &api.StopArgs{Reason: "library-consumer done"}); err != nil {
		return fmt.Errorf("Stop: %w", err)
	}
	logger.Info("client stop requested")
	return nil
}

// handleCloser is the shape both TerminalHandle and ClientHandle
// already satisfy; it lets us share the deferred-close helper below.
type handleCloser interface {
	Close(context.Context) error
}

// closeWithLog runs during defer and must not shadow the primary
// error path. It uses a fresh short-deadline context so a caller's
// cancelled ctx doesn't prevent graceful shutdown.
func closeWithLog(_ context.Context, logger *slog.Logger, name string, h handleCloser) {
	closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.Close(closeCtx); err != nil {
		logger.Warn("close returned error", "handle", name, "error", err)
		return
	}
	logger.Info("closed", "handle", name)
}
