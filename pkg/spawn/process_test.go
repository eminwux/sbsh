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
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// TestGracefulShutdown_SIGTERMEscalation covers the middle escalation
// step: the pre-signal hook does nothing, so gracefulShutdown must
// fall through to SIGTERM and reap the process.
func TestGracefulShutdown_SIGTERMEscalation(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("/bin/sleep", "30")

	proc, err := startProcess(cmd, nil)
	if err != nil {
		t.Fatalf("startProcess: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	// We don't assert on the returned error — /bin/sleep terminated by
	// SIGTERM returns "signal: terminated" from cmd.Wait(), which is
	// expected here. What matters is that gracefulShutdown returns
	// promptly and the process is reaped.
	_ = proc.gracefulShutdown(ctx, 200*time.Millisecond, nil)

	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("gracefulShutdown took %s; expected prompt escalation", elapsed)
	}
	if !proc.exited() {
		t.Fatalf("process still reports alive after gracefulShutdown")
	}
}

// TestGracefulShutdown_PreSignalKillsChild covers the happy path: the
// pre-signal hook itself stops the child (here by sending SIGINT), so
// SIGTERM is never needed.
func TestGracefulShutdown_PreSignalKillsChild(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("/bin/sleep", "30")

	proc, err := startProcess(cmd, nil)
	if err != nil {
		t.Fatalf("startProcess: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	preSignalCalled := false
	_ = proc.gracefulShutdown(ctx, 2*time.Second, func(context.Context) error {
		preSignalCalled = true
		return proc.signal(syscall.SIGINT)
	})

	if !preSignalCalled {
		t.Fatalf("preSignal hook never ran")
	}
	if !proc.exited() {
		t.Fatalf("process still reports alive after gracefulShutdown")
	}
}

// TestGracefulShutdown_AlreadyExited is a fast-path: the process has
// already been reaped, so gracefulShutdown should return immediately
// with the recorded exit error (nil for /bin/true).
func TestGracefulShutdown_AlreadyExited(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("/bin/true")
	proc, err := startProcess(cmd, nil)
	if err != nil {
		t.Fatalf("startProcess: %v", err)
	}

	// Give the reaper a moment to observe the exit.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if waitErr := proc.waitExit(ctx); waitErr != nil {
		t.Fatalf("waitExit: %v", waitErr)
	}

	if shutdownErr := proc.gracefulShutdown(ctx, time.Second, nil); shutdownErr != nil {
		t.Fatalf("gracefulShutdown after exit returned %v; want nil", shutdownErr)
	}
}
