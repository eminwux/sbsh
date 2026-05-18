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
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"
)

// newWatchChildExitTestExec builds a minimal non-init-mode Exec suitable for
// exercising watchChildExit's cmd.Wait() branch in isolation — no PTY, no RPC,
// no metadata. The caller is responsible for cmd.Start before calling
// watchChildExit; watchChildExit then Waits the child and publishes the
// real exit error on closeReqCh.
func newWatchChildExitTestExec(t *testing.T, cmd *exec.Cmd) *Exec {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Exec{
		ctx:           ctx,
		ctxCancel:     cancel,
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		id:            "test",
		cmd:           cmd,
		closeReqCh:    make(chan error, 1),
		childDoneCh:   make(chan struct{}),
		childDoneOnce: &sync.Once{},
		initMode:      false,
	}
}

// TestWatchChildExit_NonInitMode_PropagatesExitCode asserts the non-init-mode
// branch of watchChildExit propagates cmd.Wait()'s *exec.ExitError so the
// exit code is visible on closeReqCh (and therefore in EvCmdExited.Err).
// Regression: this branch previously discarded cmd.Wait() and substituted a
// fixed string, losing the code.
func TestWatchChildExit_NonInitMode_PropagatesExitCode(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 42")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	sr := newWatchChildExitTestExec(t, cmd)
	go sr.watchChildExit()

	select {
	case got := <-sr.closeReqCh:
		if got == nil {
			t.Fatalf("watchChildExit sent nil err for non-zero exit; want wrapped ExitError")
		}
		var ee *exec.ExitError
		if !errors.As(got, &ee) {
			t.Fatalf("watchChildExit err does not wrap *exec.ExitError: %v (%T)", got, got)
		}
		if ee.ExitCode() != 42 {
			t.Fatalf("ExitCode()=%d; want 42 (full err: %v)", ee.ExitCode(), got)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("watchChildExit did not publish on closeReqCh within timeout")
	}
}

// TestWatchChildExit_NonInitMode_PropagatesSignal asserts watchChildExit
// surfaces signal info when the child dies on a signal — SIGSEGV here per
// the issue AC. The wrapped *exec.ExitError's WaitStatus must report
// Signaled() == true and the originating signal.
func TestWatchChildExit_NonInitMode_PropagatesSignal(t *testing.T) {
	// Spawn a sh that suspends, then deliver SIGSEGV from the test process.
	// Using "kill -SEGV $$" from inside the shell is also possible but the
	// shell can intercept the SIGSEGV in some implementations; signaling the
	// process from outside is more deterministic.
	cmd := exec.Command("/bin/sh", "-c", "sleep 30 & wait")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	sr := newWatchChildExitTestExec(t, cmd)
	go sr.watchChildExit()

	// Give the shell a moment to install its wait before signaling so the
	// kernel routes the signal to a fully-set-up process.
	time.Sleep(100 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGSEGV); err != nil {
		t.Fatalf("signal: %v", err)
	}

	select {
	case got := <-sr.closeReqCh:
		if got == nil {
			t.Fatalf("watchChildExit sent nil err for signal death; want wrapped ExitError")
		}
		var ee *exec.ExitError
		if !errors.As(got, &ee) {
			t.Fatalf("watchChildExit err does not wrap *exec.ExitError: %v (%T)", got, got)
		}
		ws, ok := ee.Sys().(syscall.WaitStatus)
		if !ok {
			t.Fatalf("ExitError.Sys() not a syscall.WaitStatus: %T", ee.Sys())
		}
		if !ws.Signaled() {
			t.Fatalf("WaitStatus.Signaled()=false; want true for SIGSEGV death (status=%v)", ws)
		}
		if ws.Signal() != syscall.SIGSEGV {
			t.Fatalf("WaitStatus.Signal()=%v; want SIGSEGV", ws.Signal())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("watchChildExit did not publish on closeReqCh within timeout")
	}
}

// TestWatchChildExit_NonInitMode_CleanExit asserts the zero-exit path sends a
// non-nil sentinel error (the prior shape — fixed string — is preserved for
// consumers that just log the event; the relevant signal here is "non-nil so
// downstream knows the shell ended", not the literal string).
func TestWatchChildExit_NonInitMode_CleanExit(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	sr := newWatchChildExitTestExec(t, cmd)
	go sr.watchChildExit()

	select {
	case got := <-sr.closeReqCh:
		if got == nil {
			t.Fatalf("watchChildExit sent nil for clean exit; want non-nil sentinel")
		}
		// Clean exit must not surface as a wrapped *exec.ExitError — cmd.Wait
		// returns nil on code 0.
		var ee *exec.ExitError
		if errors.As(got, &ee) {
			t.Fatalf("watchChildExit wrapped an ExitError on clean exit: %v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("watchChildExit did not publish on closeReqCh within timeout")
	}
}
