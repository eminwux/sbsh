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

package terminal

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/terminal/terminalrunner"
	"github.com/eminwux/sbsh/pkg/api"
)

func newTestController(t *testing.T) *Controller {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return NewTerminalController(context.Background(), logger).(*Controller)
}

func Test_Ping(t *testing.T) {
	c := newTestController(t)

	resp, err := c.Ping(&api.PingMessage{Message: "PING"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message != "PONG" {
		t.Errorf("expected PONG, got %q", resp.Message)
	}

	_, err = c.Ping(&api.PingMessage{Message: "nope"})
	if err == nil {
		t.Error("expected error for unexpected ping message")
	}
}

func Test_WaitReady_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c := NewTerminalController(ctx, logger).(*Controller)

	cancel()

	if err := c.WaitReady(); err == nil {
		t.Error("expected error when context is cancelled before ready")
	}
}

func Test_WaitClose_Closed(t *testing.T) {
	c := newTestController(t)
	close(c.closedCh)

	if err := c.WaitClose(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_WaitClose_Closing(t *testing.T) {
	c := newTestController(t)
	c.closingCh <- errors.New("closing reason")

	if err := c.WaitClose(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_handleEvent_Unknown(t *testing.T) {
	c := newTestController(t)
	// An unrecognized event type hits the default branch and must not panic
	// or trigger a shutdown.
	c.handleEvent(terminalrunner.Event{
		ID:   api.ID("x"),
		Type: terminalrunner.EventType(9999),
		When: time.Now(),
	})
	if c.shuttingDown.Load() {
		t.Error("unknown event must not trigger shutdown")
	}
}

func Test_Delegates_NilRunner(t *testing.T) {
	c := newTestController(t)

	if err := c.Attach(&api.AttachRequest{}, &api.ResponseWithFD{}); err == nil {
		t.Error("Attach: expected error with nil runner")
	}
	nilRunnerID := api.ID("c1")
	if err := c.Detach(&nilRunnerID); err == nil {
		t.Error("Detach: expected error with nil runner")
	}
	if _, err := c.Metadata(); err == nil {
		t.Error("Metadata: expected error with nil runner")
	}
	if err := c.Write(&api.WriteRequest{Data: []byte("x")}); err == nil {
		t.Error("Write: expected error with nil runner")
	}
	if err := c.Subscribe(&api.SubscribeRequest{}, &api.ResponseWithFD{}); err == nil {
		t.Error("Subscribe: expected error with nil runner")
	}
	if _, err := c.Screenshot(&api.ScreenshotArgs{}); err == nil {
		t.Error("Screenshot: expected error with nil runner")
	}
	if _, err := c.State(); err == nil {
		t.Error("State: expected error with nil runner")
	}
	// Resize is a no-op when the runner is nil (no panic).
	c.Resize(api.ResizeArgs{})
}

func Test_Resize_Delegates(t *testing.T) {
	c := newTestController(t)
	called := false
	c.sr = &terminalrunner.Test{
		ResizeFunc: func(_ api.ResizeArgs) {
			called = true
		},
	}

	c.Resize(api.ResizeArgs{Rows: 24, Cols: 80})
	if !called {
		t.Error("expected Resize to delegate to runner")
	}
}

func Test_Attach_Delegates(t *testing.T) {
	c := newTestController(t)

	// Success path: Attach + PostAttachShell both succeed.
	c.sr = &terminalrunner.Test{
		AttachFunc: func(_ *api.AttachRequest, _ *api.ResponseWithFD) error {
			return nil
		},
		PostAttachShellFunc: func() error {
			return nil
		},
	}
	if err := c.Attach(&api.AttachRequest{ClientID: api.ID("c1")}, &api.ResponseWithFD{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Attach itself fails.
	wantAttach := errors.New("attach boom")
	c.sr = &terminalrunner.Test{
		AttachFunc: func(_ *api.AttachRequest, _ *api.ResponseWithFD) error {
			return wantAttach
		},
	}
	if err := c.Attach(&api.AttachRequest{}, &api.ResponseWithFD{}); !errors.Is(err, wantAttach) {
		t.Errorf("expected %v, got %v", wantAttach, err)
	}

	// Attach succeeds but PostAttachShell fails.
	wantPost := errors.New("post boom")
	c.sr = &terminalrunner.Test{
		AttachFunc: func(_ *api.AttachRequest, _ *api.ResponseWithFD) error {
			return nil
		},
		PostAttachShellFunc: func() error {
			return wantPost
		},
	}
	if err := c.Attach(&api.AttachRequest{}, &api.ResponseWithFD{}); !errors.Is(err, wantPost) {
		t.Errorf("expected %v, got %v", wantPost, err)
	}
}

func Test_Detach_Delegates(t *testing.T) {
	c := newTestController(t)
	want := errors.New("detach result")
	c.sr = &terminalrunner.Test{
		DetachFunc: func(_ *api.ID) error {
			return want
		},
	}
	id := api.ID("c1")
	if err := c.Detach(&id); !errors.Is(err, want) {
		t.Errorf("expected %v, got %v", want, err)
	}
}

func Test_Metadata_Delegates(t *testing.T) {
	c := newTestController(t)
	doc := &api.TerminalDoc{Status: api.TerminalStatus{State: api.Ready}}
	c.sr = &terminalrunner.Test{
		MetadataFunc: func() (*api.TerminalDoc, error) {
			return doc, nil
		},
	}
	got, err := c.Metadata()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != doc {
		t.Error("expected delegated metadata doc")
	}
}

func Test_Write_Delegates(t *testing.T) {
	c := newTestController(t)

	var gotData []byte
	c.sr = &terminalrunner.Test{
		WritePTYFunc: func(data []byte) error {
			gotData = data
			return nil
		},
	}

	// Nil request short-circuits to nil.
	if err := c.Write(nil); err != nil {
		t.Errorf("nil request: unexpected error: %v", err)
	}
	// Empty data short-circuits to nil.
	if err := c.Write(&api.WriteRequest{Data: nil}); err != nil {
		t.Errorf("empty data: unexpected error: %v", err)
	}
	if gotData != nil {
		t.Error("WritePTY must not be called for empty payloads")
	}

	// Non-empty data delegates.
	if err := c.Write(&api.WriteRequest{Data: []byte("hello")}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(gotData) != "hello" {
		t.Errorf("expected delegated data 'hello', got %q", string(gotData))
	}
}

func Test_Subscribe_Delegates(t *testing.T) {
	c := newTestController(t)
	want := errors.New("subscribe result")
	c.sr = &terminalrunner.Test{
		SubscribeFunc: func(_ *api.SubscribeRequest, _ *api.ResponseWithFD) error {
			return want
		},
	}
	if err := c.Subscribe(&api.SubscribeRequest{}, &api.ResponseWithFD{}); !errors.Is(err, want) {
		t.Errorf("expected %v, got %v", want, err)
	}
}

func Test_Screenshot_Delegates(t *testing.T) {
	c := newTestController(t)
	res := &api.ScreenshotResult{}
	c.sr = &terminalrunner.Test{
		ScreenshotFunc: func(_ *api.ScreenshotArgs) (*api.ScreenshotResult, error) {
			return res, nil
		},
	}
	got, err := c.Screenshot(&api.ScreenshotArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != res {
		t.Error("expected delegated screenshot result")
	}
}

func Test_State_MetadataError(t *testing.T) {
	c := newTestController(t)
	want := errors.New("metadata boom")
	c.sr = &terminalrunner.Test{
		MetadataFunc: func() (*api.TerminalDoc, error) {
			return nil, want
		},
	}
	if _, err := c.State(); !errors.Is(err, want) {
		t.Errorf("expected %v, got %v", want, err)
	}
}

func Test_Stop_Closes(t *testing.T) {
	c := newTestController(t)
	c.sr = &terminalrunner.Test{
		CloseFunc: func(_ error) error {
			return nil
		},
	}

	// Default reason (nil args).
	if err := c.Stop(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Close runs asynchronously; wait for it to flip the shutdown flag.
	deadline := time.Now().Add(time.Second)
	for !c.shuttingDown.Load() {
		if time.Now().After(deadline) {
			t.Fatal("Stop did not trigger shutdown")
		}
		time.Sleep(time.Millisecond)
	}
}

func Test_Stop_WithReason(t *testing.T) {
	c := newTestController(t)
	c.sr = &terminalrunner.Test{
		CloseFunc: func(_ error) error {
			return nil
		},
	}
	if err := c.Stop(&api.StopArgs{Reason: "custom"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for !c.shuttingDown.Load() {
		if time.Now().After(deadline) {
			t.Fatal("Stop did not trigger shutdown")
		}
		time.Sleep(time.Millisecond)
	}
}

func Test_Close_Idempotent(t *testing.T) {
	c := newTestController(t)
	c.sr = &terminalrunner.Test{
		CloseFunc: func(_ error) error {
			return nil
		},
	}
	if err := c.Close(errors.New("first")); err != nil {
		t.Fatalf("unexpected error on first close: %v", err)
	}
	// Second call must be a no-op (shutdown already in progress).
	if err := c.Close(errors.New("second")); err != nil {
		t.Fatalf("unexpected error on second close: %v", err)
	}
}

// Test_ControllerTest_Fake exercises the controller_fake.go test double: each
// method returns ErrFuncNotSet when its Func hook is unset, and delegates when
// the hook is provided.
func Test_ControllerTest_Fake(t *testing.T) {
	fakeID := api.ID("c1")

	// Hooks unset: every method reports ErrFuncNotSet (Resize is a no-op).
	empty := &ControllerTest{}
	if err := empty.Run(&api.TerminalSpec{ID: api.ID("z")}); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("Run: expected ErrFuncNotSet, got %v", err)
	}
	if empty.AddedSpec == nil || empty.AddedSpec.ID != api.ID("z") {
		t.Error("Run must record the spec it was called with")
	}
	if err := empty.WaitReady(); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("WaitReady: expected ErrFuncNotSet, got %v", err)
	}
	if err := empty.WaitClose(); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("WaitClose: expected ErrFuncNotSet, got %v", err)
	}
	if _, err := empty.Ping(&api.PingMessage{}); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("Ping: expected ErrFuncNotSet, got %v", err)
	}
	if err := empty.Close(nil); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("Close: expected ErrFuncNotSet, got %v", err)
	}
	empty.Resize(api.ResizeArgs{}) // no-op, must not panic
	if err := empty.Attach(&api.AttachRequest{}, &api.ResponseWithFD{}); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("Attach: expected ErrFuncNotSet, got %v", err)
	}
	if err := empty.Detach(&fakeID); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("Detach: expected ErrFuncNotSet, got %v", err)
	}
	if _, err := empty.Metadata(); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("Metadata: expected ErrFuncNotSet, got %v", err)
	}
	if _, err := empty.State(); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("State: expected ErrFuncNotSet, got %v", err)
	}
	if err := empty.Stop(&api.StopArgs{}); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("Stop: expected ErrFuncNotSet, got %v", err)
	}
	if err := empty.Write(&api.WriteRequest{}); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("Write: expected ErrFuncNotSet, got %v", err)
	}
	if err := empty.Subscribe(&api.SubscribeRequest{}, &api.ResponseWithFD{}); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("Subscribe: expected ErrFuncNotSet, got %v", err)
	}
	if _, err := empty.Screenshot(&api.ScreenshotArgs{}); !errors.Is(err, errdefs.ErrFuncNotSet) {
		t.Errorf("Screenshot: expected ErrFuncNotSet, got %v", err)
	}

	// Hooks set: every method delegates to its hook.
	sentinel := errors.New("hook")
	resizeCalled := false
	full := &ControllerTest{
		RunFunc:        func(_ *api.TerminalSpec) error { return sentinel },
		WaitReadyFunc:  func() error { return sentinel },
		WaitCloseFunc:  func() error { return sentinel },
		PingFunc:       func(_ *api.PingMessage) (*api.PingMessage, error) { return &api.PingMessage{Message: "PONG"}, nil },
		CloseFunc:      func(_ error) error { return sentinel },
		ResizeFunc:     func() { resizeCalled = true },
		AttachFunc:     func(_ *api.AttachRequest, _ *api.ResponseWithFD) error { return sentinel },
		DetachFunc:     func(_ *api.ID) error { return sentinel },
		MetadataFunc:   func() (*api.TerminalDoc, error) { return &api.TerminalDoc{}, sentinel },
		StateFunc:      func() (*api.TerminalStatusMode, error) { return nil, sentinel },
		StopFunc:       func(_ *api.StopArgs) error { return sentinel },
		WriteFunc:      func(_ *api.WriteRequest) error { return sentinel },
		SubscribeFunc:  func(_ *api.SubscribeRequest, _ *api.ResponseWithFD) error { return sentinel },
		ScreenshotFunc: func(_ *api.ScreenshotArgs) (*api.ScreenshotResult, error) { return nil, sentinel },
	}

	if err := full.Run(&api.TerminalSpec{}); !errors.Is(err, sentinel) {
		t.Errorf("Run hook: expected sentinel, got %v", err)
	}
	if err := full.WaitReady(); !errors.Is(err, sentinel) {
		t.Errorf("WaitReady hook: expected sentinel, got %v", err)
	}
	if err := full.WaitClose(); !errors.Is(err, sentinel) {
		t.Errorf("WaitClose hook: expected sentinel, got %v", err)
	}
	if resp, err := full.Ping(&api.PingMessage{}); err != nil || resp.Message != "PONG" {
		t.Errorf("Ping hook: expected PONG/nil, got %q/%v", resp.Message, err)
	}
	if err := full.Close(nil); !errors.Is(err, sentinel) {
		t.Errorf("Close hook: expected sentinel, got %v", err)
	}
	full.Resize(api.ResizeArgs{})
	if !resizeCalled {
		t.Error("Resize hook was not invoked")
	}
	if err := full.Attach(&api.AttachRequest{}, &api.ResponseWithFD{}); !errors.Is(err, sentinel) {
		t.Errorf("Attach hook: expected sentinel, got %v", err)
	}
	if err := full.Detach(&fakeID); !errors.Is(err, sentinel) {
		t.Errorf("Detach hook: expected sentinel, got %v", err)
	}
	if _, err := full.Metadata(); !errors.Is(err, sentinel) {
		t.Errorf("Metadata hook: expected sentinel, got %v", err)
	}
	if _, err := full.State(); !errors.Is(err, sentinel) {
		t.Errorf("State hook: expected sentinel, got %v", err)
	}
	if err := full.Stop(&api.StopArgs{}); !errors.Is(err, sentinel) {
		t.Errorf("Stop hook: expected sentinel, got %v", err)
	}
	if err := full.Write(&api.WriteRequest{}); !errors.Is(err, sentinel) {
		t.Errorf("Write hook: expected sentinel, got %v", err)
	}
	if err := full.Subscribe(&api.SubscribeRequest{}, &api.ResponseWithFD{}); !errors.Is(err, sentinel) {
		t.Errorf("Subscribe hook: expected sentinel, got %v", err)
	}
	if _, err := full.Screenshot(&api.ScreenshotArgs{}); !errors.Is(err, sentinel) {
		t.Errorf("Screenshot hook: expected sentinel, got %v", err)
	}
}
