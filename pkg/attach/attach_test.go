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

package attach_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/eminwux/sbsh/pkg/attach"
)

// fakeTerminalController is a minimal in-process stand-in for the real
// internal/terminal.Controller's RPC surface. It implements just enough
// of the JSON-RPC contract for pkg/attach.Run to dial the control
// socket, ping it, observe the terminal as Ready, then attempt Attach
// (where we deliberately return an error so Run exits without ever
// needing a real PTY).
type fakeTerminalController struct {
	pingCalls   atomic.Int32
	stateCalls  atomic.Int32
	attachCalls atomic.Int32
	state       api.TerminalStatusMode
	attachErr   error
	// attachBlock, if non-nil, makes Attach block until the channel is
	// closed. Used to pin Run inside the controller's setup phase so
	// the ctx-cancel race in pkg/attach.Run's shutdown select is
	// reachable from a test.
	attachBlock <-chan struct{}
}

func (f *fakeTerminalController) Ping(in *api.PingMessage, out *api.PingMessage) error {
	f.pingCalls.Add(1)
	if in != nil && in.Message == "PING" {
		*out = api.PingMessage{Message: "PONG"}
		return nil
	}
	*out = api.PingMessage{}
	return errors.New("fake: unexpected ping")
}

func (f *fakeTerminalController) State(_ *api.Empty, out *api.TerminalStatusMode) error {
	f.stateCalls.Add(1)
	*out = f.state
	return nil
}

func (f *fakeTerminalController) Attach(_ *api.ID, _ *api.Empty) error {
	f.attachCalls.Add(1)
	if f.attachBlock != nil {
		<-f.attachBlock
	}
	return f.attachErr
}

// startFakeTerminalServer spins up a JSON-RPC server on a fresh Unix
// socket inside dir, registers fake under api.TerminalService, and
// returns the absolute socket path. The server is torn down when the
// test ends.
func startFakeTerminalServer(t *testing.T, dir string, fake *fakeTerminalController) string {
	t.Helper()
	sockPath := filepath.Join(dir, "terminal.sock")

	srv := rpc.NewServer()
	if err := srv.RegisterName(api.TerminalService, fake); err != nil {
		t.Fatalf("RegisterName: %v", err)
	}

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen unix %s: %v", sockPath, err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, errAccept := ln.Accept()
			if errAccept != nil {
				return
			}
			go srv.ServeCodec(jsonrpc.NewServerCodec(conn))
		}
	}()

	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})

	return sockPath
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestRun_RequiresSocketPath(t *testing.T) {
	t.Parallel()
	err := attach.Run(context.Background(), attach.Options{Logger: discardLogger()})
	if err == nil {
		t.Fatal("expected error when SocketPath is empty, got nil")
	}
	if !errors.Is(err, attach.ErrSocketPathRequired) {
		t.Fatalf("expected error matching attach.ErrSocketPathRequired, got: %v", err)
	}
}

func TestRun_DialFailsOnMissingSocket(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "no-such.sock")

	// Stdin needs to be a *os.File, but the loop never reaches raw
	// mode because Ping fails first. A pipe satisfies the type.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runErr := attach.Run(ctx, attach.Options{
		SocketPath: missing,
		Stdin:      r,
		Stdout:     w,
		Stderr:     w,
		Logger:     discardLogger(),
	})
	if runErr == nil {
		t.Fatal("expected error dialing missing socket, got nil")
	}
}

// TestRun_AttachFailureReturnsError exercises the happy connect path
// (Ping + State) against a fake control socket and verifies pkg/attach
// surfaces the Attach failure as an error wrapped with errdefs.ErrAttach.
// This is the canonical "façade against a fake control socket" coverage
// asked for in the issue.
func TestRun_AttachFailureReturnsError(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	fake := &fakeTerminalController{
		state:     api.Ready,
		attachErr: errors.New("fake: attach refused"),
	}
	sock := startFakeTerminalServer(t, tempDir, fake)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runErr := attach.Run(ctx, attach.Options{
		SocketPath:             sock,
		Stdin:                  r,
		Stdout:                 w,
		Stderr:                 w,
		DisableDetachKeystroke: true,
		Logger:                 discardLogger(),
	})
	if runErr == nil {
		t.Fatal("expected attach failure, got nil")
	}
	if !errors.Is(runErr, errdefs.ErrAttach) {
		t.Fatalf("expected error wrapping errdefs.ErrAttach, got: %v", runErr)
	}
	if got := fake.pingCalls.Load(); got == 0 {
		t.Errorf("expected at least one Ping call, got %d", got)
	}
	if got := fake.stateCalls.Load(); got == 0 {
		t.Errorf("expected at least one State call, got %d", got)
	}
	if got := fake.attachCalls.Load(); got == 0 {
		t.Errorf("expected at least one Attach call, got %d", got)
	}
}

// TestClassifySessionEnd_NilPassthrough confirms the helper is safe to
// call with nil input — the same shape Run uses on the happy path.
func TestClassifySessionEnd_NilPassthrough(t *testing.T) {
	t.Parallel()
	if got := attach.ClassifySessionEnd(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

// TestClassifySessionEnd_ClientDetached confirms a controller error
// chained with errdefs.ErrClientDetached surfaces as attach.ErrDetached
// while preserving the inner sentinel and the substrings kukeon's
// pre-migration string match relies on.
func TestClassifySessionEnd_ClientDetached(t *testing.T) {
	t.Parallel()
	in := fmt.Errorf("%w: %w", errdefs.ErrCloseReq,
		fmt.Errorf("%w: %w", errdefs.ErrClientDetached, errors.New("read/write routines exited")))
	got := attach.ClassifySessionEnd(in)
	if !errors.Is(got, attach.ErrDetached) {
		t.Fatalf("expected attach.ErrDetached match, got: %v", got)
	}
	if errors.Is(got, attach.ErrPeerClosed) {
		t.Fatalf("did not expect attach.ErrPeerClosed match, got: %v", got)
	}
	if !errors.Is(got, errdefs.ErrClientDetached) {
		t.Fatalf("expected chain to preserve errdefs.ErrClientDetached, got: %v", got)
	}
	for _, sub := range []string{"close requested", "read/write routines exited"} {
		if !strings.Contains(got.Error(), sub) {
			t.Fatalf("expected error text to contain %q, got %q", sub, got.Error())
		}
	}
}

// TestClassifySessionEnd_PeerClosed confirms a controller error chained
// with errdefs.ErrPeerClosed surfaces as attach.ErrPeerClosed while
// preserving the inner sentinel and the legacy substrings.
func TestClassifySessionEnd_PeerClosed(t *testing.T) {
	t.Parallel()
	in := fmt.Errorf("%w: %w", errdefs.ErrCloseReq,
		fmt.Errorf("%w: %w", errdefs.ErrPeerClosed, errors.New("read/write routines exited")))
	got := attach.ClassifySessionEnd(in)
	if !errors.Is(got, attach.ErrPeerClosed) {
		t.Fatalf("expected attach.ErrPeerClosed match, got: %v", got)
	}
	if errors.Is(got, attach.ErrDetached) {
		t.Fatalf("did not expect attach.ErrDetached match, got: %v", got)
	}
	if !errors.Is(got, errdefs.ErrPeerClosed) {
		t.Fatalf("expected chain to preserve errdefs.ErrPeerClosed, got: %v", got)
	}
	for _, sub := range []string{"close requested", "read/write routines exited"} {
		if !strings.Contains(got.Error(), sub) {
			t.Fatalf("expected error text to contain %q, got %q", sub, got.Error())
		}
	}
}

// TestClassifySessionEnd_SetupErrorPassesThrough confirms setup-time
// failures (no session-end sentinel chained) keep their original
// identity so existing errdefs.ErrAttach matchers still fire.
func TestClassifySessionEnd_SetupErrorPassesThrough(t *testing.T) {
	t.Parallel()
	in := fmt.Errorf("%w: %w", errdefs.ErrAttach, errors.New("fake: attach refused"))
	got := attach.ClassifySessionEnd(in)
	if errors.Is(got, attach.ErrDetached) || errors.Is(got, attach.ErrPeerClosed) {
		t.Fatalf("did not expect public session-end sentinel, got: %v", got)
	}
	if !errors.Is(got, errdefs.ErrAttach) {
		t.Fatalf("expected chain to preserve errdefs.ErrAttach, got: %v", got)
	}
	if !errors.Is(got, in) {
		t.Fatalf("expected pass-through of input, got: %v", got)
	}
}

// TestRun_CtxCancelReturnsErrContextDone is the regression for #246: the
// shutdown select used to race on ctx-cancel — half the runs returned
// errdefs.ErrContextDone (the documented behavior) and half returned nil
// because the errCh arm short-circuited on errors.Is(ctrlErr,
// context.Canceled). The fake's Attach blocks until t.Cleanup so Run
// stays in the controller's setup phase; cancelling ctx from the test
// must surface as errdefs.ErrContextDone every time.
func TestRun_CtxCancelReturnsErrContextDone(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })

	fake := &fakeTerminalController{
		state:       api.Ready,
		attachBlock: block,
	}
	sock := startFakeTerminalServer(t, tempDir, fake)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- attach.Run(ctx, attach.Options{
			SocketPath:             sock,
			Stdin:                  r,
			Stdout:                 w,
			Stderr:                 w,
			DisableDetachKeystroke: true,
			Logger:                 discardLogger(),
		})
	}()

	// Wait for Run to reach the Attach RPC so the cancel races against
	// a controller pinned in setup, not a controller that finished early.
	deadline := time.Now().Add(2 * time.Second)
	for fake.attachCalls.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("Attach was never called; Run did not progress past setup")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()

	select {
	case runErr := <-runDone:
		if runErr == nil {
			t.Fatal("expected non-nil error after ctx-cancel, got nil")
		}
		if !errors.Is(runErr, errdefs.ErrContextDone) {
			t.Fatalf("expected error wrapping errdefs.ErrContextDone, got: %v", runErr)
		}
		if !errors.Is(runErr, context.Canceled) {
			t.Fatalf("expected context.Canceled in chain, got: %v", runErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s of ctx-cancel")
	}
}

// TestRun_NilLoggerDoesNotPanic verifies the Options.Logger == nil
// branch falls back to a discard logger rather than dereferencing nil.
func TestRun_NilLoggerDoesNotPanic(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "no-such.sock")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Logger intentionally left nil.
	_ = attach.Run(ctx, attach.Options{
		SocketPath: missing,
		Stdin:      r,
		Stdout:     w,
		Stderr:     w,
	})
	// No panic == pass.
}
