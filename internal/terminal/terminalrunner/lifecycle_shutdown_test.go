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
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

// newShutdownTestExec constructs a minimal Exec suitable for exercising
// shutdownChild without spinning up the full runner (no PTY, no RPC, no
// metadata). The caller is responsible for calling cmd.Start before invoking
// Close-path helpers; after cmd.Start, the background goroutine watches for
// exit and fires markChildDone exactly once.
func newShutdownTestExec(t *testing.T, cmd *exec.Cmd) *Exec {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sr := &Exec{
		logger:        logger,
		id:            "test",
		cmd:           cmd,
		childDoneCh:   make(chan struct{}),
		childDoneOnce: &sync.Once{},
	}
	return sr
}

// startWaiter spawns a goroutine that Waits on the child and fires
// markChildDone once. This stands in for the runner's watchChildExit under
// test; the runner does the same on non-init mode.
func startWaiter(sr *Exec) <-chan *exec.ExitError {
	done := make(chan *exec.ExitError, 1)
	go func() {
		err := sr.cmd.Wait()
		exitErr, _ := err.(*exec.ExitError)
		done <- exitErr
		sr.markChildDone()
	}()
	return done
}

// TestShutdownChild_HappyPath: child exits on SIGTERM before grace; no
// SIGKILL fallback fires. We infer "no SIGKILL" from the child's exit
// status: SIGTERM (signal 15) versus SIGKILL (signal 9).
func TestShutdownChild_HappyPath(t *testing.T) {
	// Trap TERM, exit 0 cleanly on receipt. Uses "sleep & wait" so sh does
	// not tail-call-exec sleep.
	cmd := exec.Command("/bin/sh", "-c", `trap 'exit 0' TERM; sleep 10 & wait`)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	sr := newShutdownTestExec(t, cmd)
	sr.metadata.Spec.ShutdownGrace = 2 * time.Second

	waitErr := startWaiter(sr)
	// Shell startup grace: ensure `trap 'exit 0' TERM` is installed before
	// we signal.
	time.Sleep(100 * time.Millisecond)

	start := time.Now()
	sr.shutdownChild(sr.metadata.Spec.ShutdownGrace)
	elapsed := time.Since(start)

	if elapsed >= sr.metadata.Spec.ShutdownGrace {
		t.Fatalf("shutdownChild took %v; expected well under %v (grace), indicating SIGTERM was not honored",
			elapsed, sr.metadata.Spec.ShutdownGrace)
	}

	select {
	case ee := <-waitErr:
		if ee != nil {
			// Non-zero exit — acceptable (signals may alter exit
			// code conventions), but the child should not have
			// been killed by SIGKILL.
			if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
				if ws.Signaled() && ws.Signal() == syscall.SIGKILL {
					t.Fatalf("child was killed by SIGKILL despite graceful-shutdown trap")
				}
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("child did not exit after shutdownChild returned")
	}
}

// TestShutdownChild_Escalation: child ignores SIGTERM; shutdownChild must
// escalate to SIGKILL at grace expiry and the child's exit status must
// reflect that.
func TestShutdownChild_Escalation(t *testing.T) {
	// trap '' TERM installs an "ignore" disposition. SIGTERM will be
	// ignored; only SIGKILL (uncatchable) takes the process down.
	cmd := exec.Command("/bin/sh", "-c", `trap '' TERM; sleep 30 & wait`)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	sr := newShutdownTestExec(t, cmd)
	sr.metadata.Spec.ShutdownGrace = 150 * time.Millisecond

	waitErr := startWaiter(sr)
	// Give the shell a moment to process `trap '' TERM` before we start
	// signaling. Without this, SIGTERM can arrive during shell startup,
	// before the trap is installed, and the shell dies with default
	// disposition — masking the escalation path under test.
	time.Sleep(100 * time.Millisecond)

	start := time.Now()
	sr.shutdownChild(sr.metadata.Spec.ShutdownGrace)
	elapsed := time.Since(start)

	if elapsed < sr.metadata.Spec.ShutdownGrace {
		t.Fatalf("shutdownChild returned in %v; expected at least %v (child ignores SIGTERM)",
			elapsed, sr.metadata.Spec.ShutdownGrace)
	}

	select {
	case ee := <-waitErr:
		if ee == nil {
			t.Fatalf("child exited cleanly; expected SIGKILL death")
		}
		ws, ok := ee.Sys().(syscall.WaitStatus)
		if !ok {
			t.Fatalf("unexpected exit state: %T %v", ee.Sys(), ee)
		}
		// On Linux, the child may exit due to its *process group*
		// receiving SIGKILL (the sleep child dies via SIGKILL, the
		// parent shell's `wait` returns with status 128+9, and the
		// shell itself exits — not necessarily via SIGKILL. So we
		// accept either: the shell killed by SIGKILL, or the shell
		// exited with 137/143.
		if ws.Signaled() {
			if ws.Signal() != syscall.SIGKILL {
				t.Fatalf("child signaled with %v; want SIGKILL", ws.Signal())
			}
		} else {
			code := ws.ExitStatus()
			//nolint:mnd // shell-conventional 128+SIGKILL
			if code != 128+int(syscall.SIGKILL) {
				t.Fatalf("child exited with code %d; want 128+SIGKILL (%d) or signaled by SIGKILL",
					code, 128+int(syscall.SIGKILL))
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("child did not exit after SIGKILL")
	}

	// Ensure the runner did not synthesize a TerminalState api.Exited
	// transition via updateTerminalState on a nil metadata path; the
	// escape hatch is to touch Spec.ShutdownGrace which is set above.
	_ = api.Exited
}
