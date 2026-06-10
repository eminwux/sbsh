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
	"sync"
	"syscall"
	"testing"
	"time"
)

// childPID reads the started child's PID under runtimeMu. Returns 0 when no
// child was forked (startup aborted, or never reached cmd.Start). Safe to call
// after the StartTerminal/Close goroutines have joined: nothing writes sr.cmd
// or cmd.Process at that point in non-init mode.
func childPID(sr *Exec) int {
	sr.runtimeMu.Lock()
	defer sr.runtimeMu.Unlock()
	if sr.cmd == nil || sr.cmd.Process == nil {
		return 0
	}
	return sr.cmd.Process.Pid
}

// assertPgroupReaped polls syscall.Kill(-pid, 0) until the child's process
// group is gone (ESRCH) or the deadline expires. Setsid in
// prepareTerminalCommand makes pgid == pid, so -pid targets the whole group.
func assertPgroupReaped(t *testing.T, pid int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		err := syscall.Kill(-pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return // no process left in the group
		}
		if time.Now().After(deadline) {
			t.Fatalf("child process group %d survived Close (Kill(-%d,0)=%v); orphaned child", pid, pid, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestStartTerminal_StopDuringStartup_NoRace is the AC1 regression for #396.
//
// A Stop RPC during startup runs Close on its own goroutine (controller.go's
// Stop -> go c.Close) while StartTerminal is still inside
// prepareTerminalCommand/startPty. Pre-fix the two goroutines touched
// sr.cmd / sr.ptmx / sr.capture / sr.stopSignalForwarder with no
// synchronization, and `go test -race` flagged the write/read pairs. This
// drives the exact interleaving across a swept startup window; the value is in
// the race detector staying silent (and no panic).
func TestStartTerminal_StopDuringStartup_NoRace(t *testing.T) {
	const iterations = 24
	for i := range iterations {
		// Sweep the close timing across the startup window: cmd.Start, the
		// PTY wiring, and the capture/metadata steps each take on the order
		// of tens of microseconds, so a staggered delay lands Close at a
		// different point in prepareTerminalCommand/startPty each iteration.
		stagger := time.Duration(i) * 20 * time.Microsecond

		spec := newLiveSpec(t, "/bin/cat") // idles on the PTY, dies on SIGTERM
		spec.EnvInherit = true
		sr := NewTerminalRunnerExec(context.Background(), newDiscardLogger(), spec).(*Exec)
		if err := sr.CreateMetadata(); err != nil {
			t.Fatalf("CreateMetadata: %v", err)
		}

		evCh := make(chan Event, 8)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = sr.StartTerminal(evCh) // may fail when Close wins the race; fine
		}()
		go func() {
			defer wg.Done()
			time.Sleep(stagger)
			_ = sr.Close(nil)
		}()
		wg.Wait()

		// Whatever the interleaving, a forked child must not survive Close.
		if pid := childPID(sr); pid > 0 {
			assertPgroupReaped(t, pid, 3*time.Second)
		}
	}
}

// TestStartPty_AbortsWhenClosing_NoOrphanFork is the deterministic AC2 proof
// for #396: once Close has latched `closing`, startPty must not fork a child.
//
// Pre-fix, shutdownChild returned early while cmd.Process was still nil, Close
// passed its only kill site, and a cmd.Start that then won the race forked a
// child nothing would ever signal (cmd.Cancel is a no-op). The `closing`
// latch, checked under runtimeMu in the same critical section as cmd.Start,
// closes that window: startPty aborts before the fork.
func TestStartPty_AbortsWhenClosing_NoOrphanFork(t *testing.T) {
	spec := newLiveSpec(t, "/bin/cat")
	spec.EnvInherit = true
	sr := NewTerminalRunnerExec(context.Background(), newDiscardLogger(), spec).(*Exec)
	if err := sr.CreateMetadata(); err != nil {
		t.Fatalf("CreateMetadata: %v", err)
	}

	// Stage the cmd as StartTerminal would, then simulate Close having
	// latched `closing` before startPty reaches its fork critical section.
	if err := sr.prepareTerminalCommand(); err != nil {
		t.Fatalf("prepareTerminalCommand: %v", err)
	}
	sr.runtimeMu.Lock()
	sr.closing = true
	sr.runtimeMu.Unlock()

	if err := sr.startPty(); err == nil {
		t.Fatal("startPty should abort once closing is latched, but returned nil")
	}
	if pid := childPID(sr); pid != 0 {
		t.Fatalf("startPty forked a child (pid %d) despite closing being latched; orphan hole", pid)
	}
}

// TestStartTerminal_StopDuringStartup_SignalIgnoringChildAlwaysDies is the
// end-to-end AC2 regression for #396 with a child that ignores SIGTERM and
// SIGHUP. Whether startPty aborts (no child) or forks before Close observes
// it, no process may survive once Close returns: the graceful sweep escalates
// SIGTERM -> SIGKILL on the child's process group, and SIGKILL is uncatchable.
func TestStartTerminal_StopDuringStartup_SignalIgnoringChildAlwaysDies(t *testing.T) {
	const iterations = 16
	for i := range iterations {
		stagger := time.Duration(i) * 25 * time.Microsecond

		// trap '' TERM HUP installs an "ignore" disposition for both
		// signals; only SIGKILL takes the process down. The bare `sleep`
		// loop keeps it alive and in the foreground PTY pgroup.
		spec := newLiveSpec(t, "/bin/sh", "-c", `trap '' TERM HUP INT; while :; do sleep 0.05; done`)
		spec.EnvInherit = true
		spec.ShutdownGrace = 150 * time.Millisecond
		sr := NewTerminalRunnerExec(context.Background(), newDiscardLogger(), spec).(*Exec)
		if err := sr.CreateMetadata(); err != nil {
			t.Fatalf("CreateMetadata: %v", err)
		}

		evCh := make(chan Event, 8)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = sr.StartTerminal(evCh)
		}()
		go func() {
			defer wg.Done()
			time.Sleep(stagger)
			_ = sr.Close(nil)
		}()
		wg.Wait()

		if pid := childPID(sr); pid > 0 {
			// grace (150ms) + SIGKILL propagation + reaping margin.
			assertPgroupReaped(t, pid, 3*time.Second)
		}
	}
}
