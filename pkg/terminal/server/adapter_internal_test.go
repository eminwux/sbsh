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

// Internal tests exercise the unexported rpcAdapter forwarding layer and
// the Server's runLoop/bringUp/Stop branches directly, injecting a fake
// runner so the adapter and lifecycle paths are covered without spawning
// a real PTY (the wire-level integration coverage lives in server_test.go).
package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/terminal/terminalrunner"
	"github.com/eminwux/sbsh/pkg/api"
)

func newAdapterServer(t *testing.T) *Server {
	t.Helper()
	srv, err := New(&api.TerminalSpec{Command: "/bin/sh"}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv
}

func TestRPCAdapter_Run_WaitReady_WaitClose(t *testing.T) {
	a := &rpcAdapter{srv: newAdapterServer(t)}
	if err := a.Run(&api.TerminalSpec{}); err == nil {
		t.Fatal("Run: expected error, got nil")
	}
	if err := a.WaitReady(); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
	if err := a.WaitClose(); err != nil {
		t.Fatalf("WaitClose: %v", err)
	}
}

func TestRPCAdapter_Ping(t *testing.T) {
	a := &rpcAdapter{srv: newAdapterServer(t)}

	pong, err := a.Ping(&api.PingMessage{Message: "PING"})
	if err != nil {
		t.Fatalf("Ping(PING): %v", err)
	}
	if pong.Message != "PONG" {
		t.Fatalf("Ping reply = %q, want PONG", pong.Message)
	}

	if _, err := a.Ping(nil); err == nil {
		t.Fatal("Ping(nil): expected error")
	}
	if _, err := a.Ping(&api.PingMessage{Message: "nope"}); err == nil {
		t.Fatal("Ping(nope): expected error")
	}
}

func TestRPCAdapter_NilRunnerErrors(t *testing.T) {
	a := &rpcAdapter{srv: newAdapterServer(t)}

	// Resize on a nil runner is a no-op and must not panic.
	a.Resize(api.ResizeArgs{})

	if err := a.Detach(nil); err == nil {
		t.Fatal("Detach(nil runner): expected error")
	}
	if err := a.Attach(&api.AttachRequest{}, &api.ResponseWithFD{}); err == nil {
		t.Fatal("Attach(nil runner): expected error")
	}
	if err := a.Write(&api.WriteRequest{Data: []byte("x")}); err == nil {
		t.Fatal("Write(nil runner): expected error")
	}
	if err := a.Subscribe(&api.SubscribeRequest{}, &api.ResponseWithFD{}); err == nil {
		t.Fatal("Subscribe(nil runner): expected error")
	}
	if _, err := a.Screenshot(&api.ScreenshotArgs{}); err == nil {
		t.Fatal("Screenshot(nil runner): expected error")
	}
	if _, err := a.State(); err == nil {
		t.Fatal("State(nil runner): expected error")
	}
}

func TestRPCAdapter_Resize_ForwardsToRunner(t *testing.T) {
	srv := newAdapterServer(t)
	called := false
	srv.runner = &terminalrunner.Test{ResizeFunc: func(api.ResizeArgs) { called = true }}
	a := &rpcAdapter{srv: srv}

	a.Resize(api.ResizeArgs{Rows: 24, Cols: 80})
	if !called {
		t.Fatal("Resize did not forward to the runner")
	}
}

func TestRPCAdapter_Detach_Forwards(t *testing.T) {
	srv := newAdapterServer(t)
	srv.runner = &terminalrunner.Test{DetachFunc: func(*api.ID) error { return nil }}
	a := &rpcAdapter{srv: srv}
	if err := a.Detach(nil); err != nil {
		t.Fatalf("Detach: %v", err)
	}
}

func TestRPCAdapter_Attach(t *testing.T) {
	wantErr := errors.New("attach boom")

	// Attach itself errors → returned, PostAttachShell not reached.
	srv := newAdapterServer(t)
	srv.runner = &terminalrunner.Test{
		AttachFunc:          func(*api.AttachRequest, *api.ResponseWithFD) error { return wantErr },
		PostAttachShellFunc: func() error { return errors.New("should not be called") },
	}
	if err := (&rpcAdapter{srv: srv}).Attach(&api.AttachRequest{}, &api.ResponseWithFD{}); !errors.Is(err, wantErr) {
		t.Fatalf("Attach error = %v, want %v", err, wantErr)
	}

	// Attach succeeds but PostAttachShell errors → that error propagates.
	postErr := errors.New("post boom")
	srv2 := newAdapterServer(t)
	srv2.runner = &terminalrunner.Test{
		AttachFunc:          func(*api.AttachRequest, *api.ResponseWithFD) error { return nil },
		PostAttachShellFunc: func() error { return postErr },
	}
	if err := (&rpcAdapter{srv: srv2}).Attach(&api.AttachRequest{}, &api.ResponseWithFD{}); !errors.Is(err, postErr) {
		t.Fatalf("Attach post error = %v, want %v", err, postErr)
	}

	// Both succeed → nil.
	srv3 := newAdapterServer(t)
	srv3.runner = &terminalrunner.Test{
		AttachFunc:          func(*api.AttachRequest, *api.ResponseWithFD) error { return nil },
		PostAttachShellFunc: func() error { return nil },
	}
	if err := (&rpcAdapter{srv: srv3}).Attach(&api.AttachRequest{}, &api.ResponseWithFD{}); err != nil {
		t.Fatalf("Attach happy path: %v", err)
	}
}

func TestRPCAdapter_WriteVariants(t *testing.T) {
	srv := newAdapterServer(t)
	var got []byte
	srv.runner = &terminalrunner.Test{WritePTYFunc: func(d []byte) error { got = d; return nil }}
	a := &rpcAdapter{srv: srv}

	// nil request and empty data short-circuit to nil without forwarding.
	if err := a.Write(nil); err != nil {
		t.Fatalf("Write(nil): %v", err)
	}
	if err := a.Write(&api.WriteRequest{Data: nil}); err != nil {
		t.Fatalf("Write(empty): %v", err)
	}
	if got != nil {
		t.Fatal("Write forwarded an empty/nil payload")
	}

	if err := a.Write(&api.WriteRequest{Data: []byte("hello")}); err != nil {
		t.Fatalf("Write(hello): %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("WritePTY got %q, want hello", got)
	}
}

func TestRPCAdapter_Subscribe_Forwards(t *testing.T) {
	srv := newAdapterServer(t)
	srv.runner = &terminalrunner.Test{
		SubscribeFunc: func(*api.SubscribeRequest, *api.ResponseWithFD) error { return nil },
	}
	if err := (&rpcAdapter{srv: srv}).Subscribe(&api.SubscribeRequest{}, &api.ResponseWithFD{}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
}

func TestRPCAdapter_Screenshot_Forwards(t *testing.T) {
	srv := newAdapterServer(t)
	want := &api.ScreenshotResult{}
	srv.runner = &terminalrunner.Test{
		ScreenshotFunc: func(*api.ScreenshotArgs) (*api.ScreenshotResult, error) { return want, nil },
	}
	got, err := (&rpcAdapter{srv: srv}).Screenshot(&api.ScreenshotArgs{})
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	if got != want {
		t.Fatal("Screenshot did not return the runner's result")
	}
}

func TestRPCAdapter_State_Forwards(t *testing.T) {
	srv := newAdapterServer(t)
	srv.runner = &terminalrunner.Test{
		MetadataFunc: func() (*api.TerminalDoc, error) {
			return &api.TerminalDoc{Status: api.TerminalStatus{State: api.Ready}}, nil
		},
	}
	state, err := (&rpcAdapter{srv: srv}).State()
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if *state != api.Ready {
		t.Fatalf("State = %v, want Ready", *state)
	}
}

func TestRPCAdapter_Close_TriggersStop(t *testing.T) {
	srv := newAdapterServer(t)
	if err := (&rpcAdapter{srv: srv}).Close(errors.New("close reason")); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case got := <-srv.stopCh:
		if got == nil || !strings.Contains(got.Error(), "close reason") {
			t.Fatalf("stopCh got %v, want close reason", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not push a stop reason")
	}
}

func TestRPCAdapter_Stop(t *testing.T) {
	// Explicit reason in StopArgs is wrapped and propagated.
	srv := newAdapterServer(t)
	if err := (&rpcAdapter{srv: srv}).Stop(&api.StopArgs{Reason: "boom"}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	assertStopReasonContains(t, srv, "boom")

	// Nil args fall back to the default reason.
	srv2 := newAdapterServer(t)
	if err := (&rpcAdapter{srv: srv2}).Stop(nil); err != nil {
		t.Fatalf("Stop(nil): %v", err)
	}
	assertStopReasonContains(t, srv2, "stop requested")
}

func assertStopReasonContains(t *testing.T, srv *Server, want string) {
	t.Helper()
	select {
	case got := <-srv.stopCh:
		if got == nil || !strings.Contains(got.Error(), want) {
			t.Fatalf("stopCh got %v, want substring %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Stop did not propagate reason %q", want)
	}
}

func TestServer_runLoop_CtxCancelled(t *testing.T) {
	srv := newAdapterServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := srv.runLoop(ctx, make(chan terminalrunner.Event), make(chan error)); !errors.Is(err, context.Canceled) {
		t.Fatalf("runLoop ctx = %v, want context.Canceled", err)
	}
}

func TestServer_runLoop_TerminalEvents(t *testing.T) {
	srv := newAdapterServer(t)

	// EvError carrying an error propagates that error.
	evErr := errors.New("pty died")
	ev := make(chan terminalrunner.Event, 1)
	ev <- terminalrunner.Event{Type: terminalrunner.EvError, Err: evErr}
	if err := srv.runLoop(context.Background(), ev, make(chan error)); !errors.Is(err, evErr) {
		t.Fatalf("runLoop EvError = %v, want %v", err, evErr)
	}

	// A non-terminal event is ignored; the loop keeps blocking until a
	// terminal one (EvCmdExited with no error) arrives.
	ev2 := make(chan terminalrunner.Event, 2)
	ev2 <- terminalrunner.Event{Type: terminalrunner.EventType(99)}
	ev2 <- terminalrunner.Event{Type: terminalrunner.EvCmdExited}
	if err := srv.runLoop(context.Background(), ev2, make(chan error)); err == nil {
		t.Fatal("runLoop EvCmdExited: expected non-nil exit error")
	}
}

func TestServer_runLoop_PTYReadErrorPrefersCmdExited(t *testing.T) {
	srv := newAdapterServer(t)

	// A benign PTY-master read error (the EIO the child's tty-close triggers)
	// reliably preempts the authoritative EvCmdExited on the shared channel.
	// runLoop must return the EvCmdExited cause, not the benign read error.
	eioErr := fmt.Errorf("terminalManagerReader pty read error: %w: %w",
		terminalrunner.ErrPipeRead, syscall.EIO)
	exitErr := errors.New("shell process exited: code=0")

	ev := make(chan terminalrunner.Event, 2)
	ev <- terminalrunner.Event{Type: terminalrunner.EvError, Err: eioErr}
	ev <- terminalrunner.Event{Type: terminalrunner.EvCmdExited, Err: exitErr}
	if err := srv.runLoop(context.Background(), ev, make(chan error)); !errors.Is(err, exitErr) {
		t.Fatalf("runLoop pty-EIO then EvCmdExited = %v, want %v", err, exitErr)
	}
}

func TestServer_runLoop_PTYReadErrorWithoutExitReturnsError(t *testing.T) {
	srv := newAdapterServer(t)

	// A genuine PTY read error with no EvCmdExited following (the child is
	// still alive) must fall back to returning that error once the grace
	// window elapses — the preference only applies when an exit actually races.
	eioErr := fmt.Errorf("terminalManagerReader pty read error: %w: %w",
		terminalrunner.ErrPipeRead, syscall.EIO)
	ev := make(chan terminalrunner.Event, 1)
	ev <- terminalrunner.Event{Type: terminalrunner.EvError, Err: eioErr}
	if err := srv.runLoop(context.Background(), ev, make(chan error)); !errors.Is(err, eioErr) {
		t.Fatalf("runLoop pty-EIO alone = %v, want %v", err, eioErr)
	}
}

func TestServer_runLoop_RPCDone(t *testing.T) {
	srv := newAdapterServer(t)

	// RPC done with an error returns that error.
	rpcErr := errors.New("rpc boom")
	done := make(chan error, 1)
	done <- rpcErr
	if err := srv.runLoop(context.Background(), make(chan terminalrunner.Event), done); !errors.Is(err, rpcErr) {
		t.Fatalf("runLoop rpcDone err = %v, want %v", err, rpcErr)
	}

	// RPC done with nil and a live ctx is treated as an unexpected exit.
	done2 := make(chan error, 1)
	done2 <- nil
	if err := srv.runLoop(context.Background(), make(chan terminalrunner.Event), done2); err == nil {
		t.Fatal("runLoop rpcDone nil: expected non-nil error")
	}
}

func TestServer_runLoop_StopChannel(t *testing.T) {
	// Stop signal carrying an error returns it verbatim.
	srv := newAdapterServer(t)
	stopErr := errors.New("stop boom")
	srv.stopCh <- stopErr
	if err := srv.runLoop(context.Background(), make(chan terminalrunner.Event), make(chan error)); !errors.Is(err, stopErr) {
		t.Fatalf("runLoop stop err = %v, want %v", err, stopErr)
	}

	// Stop signal with nil error yields the generic "Stop called" cause.
	srv2 := newAdapterServer(t)
	srv2.stopCh <- nil
	if err := srv2.runLoop(context.Background(), make(chan terminalrunner.Event), make(chan error)); err == nil {
		t.Fatal("runLoop nil stop: expected non-nil error")
	}
}

func TestServer_bringUp_RejectsSecondServe(t *testing.T) {
	srv := newAdapterServer(t)
	srv.serveCalled = true
	if _, _, _, err := srv.bringUp(context.Background(), nil); err == nil {
		t.Fatal("bringUp after serveCalled: expected error")
	}
}

func TestServer_Stop_IdempotentAndDefaultReason(t *testing.T) {
	srv := newAdapterServer(t)

	// nil reason falls back to a default, non-nil cause.
	if err := srv.Stop(nil); err != nil {
		t.Fatalf("Stop(nil): %v", err)
	}
	// A second Stop collapses into the first (stopOnce); the buffered
	// channel still holds exactly the first reason.
	if err := srv.Stop(errors.New("second")); err != nil {
		t.Fatalf("Stop(second): %v", err)
	}
	select {
	case got := <-srv.stopCh:
		if got == nil {
			t.Fatal("Stop pushed a nil reason")
		}
		if strings.Contains(got.Error(), "second") {
			t.Fatal("second Stop reason leaked past stopOnce")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not push a reason")
	}
}

func TestServer_Metadata_NotStarted(t *testing.T) {
	srv := newAdapterServer(t)
	if _, err := srv.Metadata(); err == nil {
		t.Fatal("Metadata before start: expected error")
	}
}
