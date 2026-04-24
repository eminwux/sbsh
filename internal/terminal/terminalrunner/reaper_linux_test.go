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

//go:build linux

package terminalrunner

import (
	"io"
	"log/slog"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// discardLogger returns a slog.Logger that drops every record so tests do not
// spam the go-test output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestReaper_DrainsOrphan verifies that a child whose os/exec.Cmd we never
// Wait on is still reaped by the reaper's WNOHANG loop.
func TestReaper_DrainsOrphan(t *testing.T) {
	r := newReaper(discardLogger())
	r.Start()
	defer r.Stop()

	// Spawn a short-lived child we intentionally do not Wait on. The
	// reaper is expected to pick up its SIGCHLD and reap it.
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start orphan child: %v", err)
	}

	// Release so os/exec does not hold a reference that we then race.
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()

	// Poll: within 2s the reaper should have reaped the child, so
	// syscall.Kill(pid, 0) (existence probe) returns ESRCH.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if err == syscall.ESRCH {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("orphan pid %d still exists after 2s; reaper did not drain it", pid)
}

// TestReaper_TrackedExitResolvesRace verifies that when the reaper consumes
// the tracked child's SIGCHLD (the race described in the acceptance
// criteria), the real exit code is still delivered via TrackedExitCh — the
// normal "cmd.Wait() ECHILD" failure mode is masked.
func TestReaper_TrackedExitResolvesRace(t *testing.T) {
	r := newReaper(discardLogger())
	r.Start()
	defer r.Stop()

	cmd := exec.Command("/bin/sh", "-c", "exit 7")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	r.RegisterChild(cmd.Process.Pid)

	select {
	case code, ok := <-r.TrackedExitCh():
		if !ok {
			t.Fatalf("TrackedExitCh closed without delivering a code")
		}
		if code != 7 {
			t.Fatalf("tracked exit code = %d, want 7", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for reaper to deliver tracked exit")
	}

	// Do NOT call cmd.Wait — the reaper has already consumed SIGCHLD.
	// Release to drop the os.Process handle cleanly.
	_ = cmd.Process.Release()
}
