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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

// processTestTimeout bounds how long the process-set tests wait for a process
// to exit or for its capture transcript to flush.
const processTestTimeout = 5 * time.Second

// newProcessesTestExec builds a minimal non-init-mode Exec for exercising the
// process-set spawn path in isolation — no PTY, no RPC, no metadata persistence.
// The caller seeds metadata.Spec.Processes before calling startProcesses.
func newProcessesTestExec(t *testing.T, specs []api.ProcessSpec) *Exec {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sr := &Exec{
		ctx:       ctx,
		ctxCancel: cancel,
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		id:        "test",
		initMode:  false,
	}
	sr.metadata.Spec.Processes = specs
	return sr
}

// processByName returns the spawned procState with the given spec name.
func (sr *Exec) processByName(t *testing.T, name string) *procState {
	t.Helper()
	sr.processesMu.Lock()
	defer sr.processesMu.Unlock()
	for _, c := range sr.processes {
		if c.spec.Name == name {
			return c
		}
	}
	t.Fatalf("no spawned process named %q", name)
	return nil
}

// waitDone blocks until the process's exit is observed or processTestTimeout
// elapses.
func waitDone(t *testing.T, c *procState) {
	t.Helper()
	select {
	case <-c.doneCh:
	case <-time.After(processTestTimeout):
		t.Fatalf("process %q exit not observed within %s", c.spec.Name, processTestTimeout)
	}
}

// waitCapture polls the capture file until its contents equal want or
// processTestTimeout elapses. The drain goroutine finishes flushing slightly
// after the process exits, so a direct read right after waitDone can race the
// final write.
func waitCapture(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(processTestTimeout)
	var last string
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(path)
		if err == nil {
			last = string(b)
			if last == want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("capture %q = %q; want %q", path, last, want)
}

// TestStartProcesses_TwoProcessSpawn asserts a two-process spec spawns both
// processes, drains each process's stdout into its own capture file with the
// expected content, and observes each process's clean exit.
func TestStartProcesses_TwoProcessSpawn(t *testing.T) {
	dir := t.TempDir()
	capA := filepath.Join(dir, "a.cap")
	capB := filepath.Join(dir, "b.cap")

	sr := newProcessesTestExec(t, []api.ProcessSpec{
		{Name: "a", Command: "/bin/sh", CommandArgs: []string{"-c", "printf 'hello-A'"}, CaptureFile: capA},
		{Name: "b", Command: "/bin/sh", CommandArgs: []string{"-c", "printf 'hello-B'"}, CaptureFile: capB},
	})

	if err := sr.startProcesses(); err != nil {
		t.Fatalf("startProcesses: %v", err)
	}

	sr.processesMu.Lock()
	got := len(sr.processes)
	sr.processesMu.Unlock()
	if got != 2 {
		t.Fatalf("spawned %d processes; want 2", got)
	}

	ca := sr.processByName(t, "a")
	cb := sr.processByName(t, "b")
	waitDone(t, ca)
	waitDone(t, cb)
	if ca.exitErr != nil {
		t.Errorf("process a exitErr = %v; want nil (clean exit)", ca.exitErr)
	}
	if cb.exitErr != nil {
		t.Errorf("process b exitErr = %v; want nil (clean exit)", cb.exitErr)
	}

	waitCapture(t, capA, "hello-A")
	waitCapture(t, capB, "hello-B")
}

// TestStartProcesses_CaptureMode asserts the process capture file is chmod'd to
// the resolved ProcessSpec.CaptureMode after open.
func TestStartProcesses_CaptureMode(t *testing.T) {
	dir := t.TempDir()
	capPath := filepath.Join(dir, "c.cap")

	sr := newProcessesTestExec(t, []api.ProcessSpec{
		{
			Name:        "c",
			Command:     "/bin/sh",
			CommandArgs: []string{"-c", "printf x"},
			CaptureFile: capPath,
			CaptureMode: 0o640,
		},
	})
	if err := sr.startProcesses(); err != nil {
		t.Fatalf("startProcesses: %v", err)
	}
	waitDone(t, sr.processByName(t, "c"))

	info, err := os.Stat(capPath)
	if err != nil {
		t.Fatalf("stat capture: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o640 {
		t.Fatalf("capture mode = %o; want 0o640", perm)
	}
}

// TestStartProcesses_ExitObservation asserts a non-zero process exit is observed
// and surfaced via the process's exitErr.
func TestStartProcesses_ExitObservation(t *testing.T) {
	sr := newProcessesTestExec(t, []api.ProcessSpec{
		{Name: "fail", Command: "/bin/sh", CommandArgs: []string{"-c", "exit 7"}},
	})
	if err := sr.startProcesses(); err != nil {
		t.Fatalf("startProcesses: %v", err)
	}

	c := sr.processByName(t, "fail")
	waitDone(t, c)
	if c.exitErr == nil {
		t.Fatalf("exit 7 process exitErr = nil; want non-nil")
	}
}

// TestStartProcesses_TurnOrderedSpawn asserts the turn barrier: every process in
// turn N is fork+exec'd before any process in turn N+1. spawnProcess appends to
// sr.processes only after cmd.Start returns, and spawnGroup blocks until all of
// its group's spawnProcess calls have returned, so sr.processes's Turn values
// are non-decreasing across the slice. Same-turn processes (turns 0 and 1 each
// have two members here) may appear in any intra-group order.
func TestStartProcesses_TurnOrderedSpawn(t *testing.T) {
	sr := newProcessesTestExec(t, []api.ProcessSpec{
		{Name: "t1a", Command: "/bin/sh", CommandArgs: []string{"-c", "sleep 0.05"}, Turn: 1},
		{Name: "t0a", Command: "/bin/sh", CommandArgs: []string{"-c", "sleep 0.05"}, Turn: 0},
		{Name: "t2a", Command: "/bin/sh", CommandArgs: []string{"-c", "sleep 0.05"}, Turn: 2},
		{Name: "t0b", Command: "/bin/sh", CommandArgs: []string{"-c", "sleep 0.05"}, Turn: 0},
		{Name: "t1b", Command: "/bin/sh", CommandArgs: []string{"-c", "sleep 0.05"}, Turn: 1},
	})
	if err := sr.startProcesses(); err != nil {
		t.Fatalf("startProcesses: %v", err)
	}

	sr.processesMu.Lock()
	defer sr.processesMu.Unlock()
	if len(sr.processes) != 5 {
		t.Fatalf("spawned %d processes; want 5", len(sr.processes))
	}
	prev := -1
	for i, c := range sr.processes {
		if c.spec.Turn < prev {
			t.Fatalf("processes[%d] (%q) turn=%d follows turn=%d; spawn order not turn-monotonic",
				i, c.spec.Name, c.spec.Turn, prev)
		}
		prev = c.spec.Turn
	}
	// turn 0 processes must be the first two spawned.
	for i := range 2 {
		if sr.processes[i].spec.Turn != 0 {
			t.Fatalf("processes[%d] turn=%d; want turn 0 first", i, sr.processes[i].spec.Turn)
		}
	}
}

// TestStartProcesses_DrainClosesParentEndOnExit asserts the parent socket end
// is closed when the process exits (drain EOF), not deferred until runner
// Close. Regression guard for the early-exit leak: the runner ctx is left live
// here, so a parentEnd that only the ctx.Done watcher could close would stay
// open and this poll would time out.
func TestStartProcesses_DrainClosesParentEndOnExit(t *testing.T) {
	sr := newProcessesTestExec(t, []api.ProcessSpec{
		{Name: "x", Command: "/bin/sh", CommandArgs: []string{"-c", "printf done"}},
	})
	if err := sr.startProcesses(); err != nil {
		t.Fatalf("startProcesses: %v", err)
	}
	c := sr.processByName(t, "x")
	waitDone(t, c)

	// The drain teardown closes parentEnd shortly after the process EOFs. Poll
	// until a read reports the fd closed; io.EOF (data drained, not yet closed)
	// keeps polling. *os.File is safe for the concurrent read against the drain
	// goroutine still finishing up.
	deadline := time.Now().Add(processTestTimeout)
	var one [1]byte
	for time.Now().Before(deadline) {
		if _, err := c.parentEnd.Read(one[:]); errors.Is(err, os.ErrClosed) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("parentEnd not closed after process exit; fd/goroutine leaked until Close")
}

// TestOrderedTurns asserts distinct turns come back ascending and deduplicated.
func TestOrderedTurns(t *testing.T) {
	specs := []api.ProcessSpec{
		{Name: "a", Turn: 2},
		{Name: "b", Turn: 0},
		{Name: "c", Turn: 2},
		{Name: "d", Turn: 1},
		{Name: "e", Turn: 0},
	}
	got := orderedTurns(specs)
	want := []int{0, 1, 2}
	if len(got) != len(want) {
		t.Fatalf("orderedTurns = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("orderedTurns = %v; want %v", got, want)
		}
	}
}

// TestProcessesWithTurn asserts the group filter preserves declaration order.
func TestProcessesWithTurn(t *testing.T) {
	specs := []api.ProcessSpec{
		{Name: "a", Turn: 0},
		{Name: "b", Turn: 1},
		{Name: "c", Turn: 0},
	}
	group := processesWithTurn(specs, 0)
	if len(group) != 2 || group[0].Name != "a" || group[1].Name != "c" {
		t.Fatalf("processesWithTurn(0) = %+v; want [a c] in order", group)
	}
}
