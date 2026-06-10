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

package terminalrunner

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

// newWaitOnTerminalTestExec builds a minimal Exec for exercising
// waitOnTerminal's receive in isolation. closeReqCh is UNBUFFERED to match the
// production wiring (terminal_runner_exec.go:209) — the leak only manifests on
// an unbuffered channel where a dropped sender send leaves the receiver with
// nothing to take.
func newWaitOnTerminalTestExec(t *testing.T, evCh chan Event) *Exec {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Exec{
		ctx:        ctx,
		ctxCancel:  cancel,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		id:         "test",
		closeReqCh: make(chan error), // unbuffered, as in production
		evCh:       evCh,
	}
}

// TestWaitOnTerminal_CtxCancel_NoLeak reproduces #397: on an externally-driven
// Close that escalates to SIGKILL, watchChildExit's send select can take its
// ctx.Done() arm and drop the exit error before waitOnTerminal — the sole
// receiver — ever receives it. With a bare `<-closeReqCh` the receiver parked
// forever, leaking one goroutine per affected Close. waitOnTerminal must now
// take the symmetric ctx.Done() escape and return.
func TestWaitOnTerminal_CtxCancel_NoLeak(t *testing.T) {
	sr := newWaitOnTerminalTestExec(t, make(chan Event, 1))

	done := make(chan struct{})
	go func() {
		sr.waitOnTerminal()
		close(done)
	}()

	// Simulate the dropped-send race: cancel ctx (as Close's ctxCancel does)
	// without ever sending on closeReqCh (as watchChildExit does when it picks
	// its own ctx.Done() arm).
	sr.ctxCancel()

	select {
	case <-done:
		// waitOnTerminal escaped via ctx.Done() — no leak.
	case <-time.After(5 * time.Second):
		t.Fatalf("waitOnTerminal did not return after ctx cancel; goroutine leaked (#397)")
	}
}

// TestWaitOnTerminal_NormalExit_PublishesEvent asserts the clean path is
// unchanged: when watchChildExit delivers the exit error on closeReqCh,
// waitOnTerminal receives it and publishes EvCmdExited carrying that error.
func TestWaitOnTerminal_NormalExit_PublishesEvent(t *testing.T) {
	evCh := make(chan Event, 1)
	sr := newWaitOnTerminalTestExec(t, evCh)

	done := make(chan struct{})
	go func() {
		sr.waitOnTerminal()
		close(done)
	}()

	wantErr := errors.New("shell process exited: code=0")
	select {
	case sr.closeReqCh <- wantErr:
	case <-time.After(5 * time.Second):
		t.Fatalf("waitOnTerminal never received on closeReqCh")
	}

	select {
	case ev := <-evCh:
		if ev.Type != EvCmdExited {
			t.Fatalf("event Type=%v; want EvCmdExited", ev.Type)
		}
		if !errors.Is(ev.Err, wantErr) {
			t.Fatalf("event Err=%v; want %v", ev.Err, wantErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("waitOnTerminal did not publish EvCmdExited")
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("waitOnTerminal did not return after publishing event")
	}
}
