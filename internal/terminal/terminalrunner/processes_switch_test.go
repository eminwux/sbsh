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
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

// syncBuf is a goroutine-safe io.Writer used as a stand-in operator attacher on
// the process-set fan-out: the supervisor's mirror goroutine writes to it while
// the test goroutine inspects what arrived.
type syncBuf struct {
	mu sync.Mutex
	b  []byte
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b = append(s.b, p...)
	return len(p), nil
}

func (s *syncBuf) bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.b...)
}

func (s *syncBuf) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.b)
}

// testOperatorPipes returns the operator-facing input pipe and output fan-out
// that startProcesses wired, failing the test if the relay was not set up.
func (sr *Exec) testOperatorPipes(t *testing.T) (*os.File, *DynamicMultiWriter) {
	t.Helper()
	sr.ptyPipesMu.RLock()
	defer sr.ptyPipesMu.RUnlock()
	if sr.ptyPipes == nil || sr.ptyPipes.pipeInW == nil || sr.ptyPipes.multiOutW == nil {
		t.Fatal("operator IO not wired by startProcesses")
	}
	return sr.ptyPipes.pipeInW, sr.ptyPipes.multiOutW
}

// waitUntil polls cond until it holds or processTestTimeout elapses.
func waitUntil(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(processTestTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s: %s", processTestTimeout, msg)
}

// TestStartProcesses_CurrentDefaultsToFirstInSpecOrder asserts the operator
// focus initializes to the first process in declaration order, independent of
// ProcessSpec.Turn (here the first-declared process has the highest turn, so a
// turn-ordered default would pick the wrong one).
func TestStartProcesses_CurrentDefaultsToFirstInSpecOrder(t *testing.T) {
	sr := newProcessesTestExec(t, []api.ProcessSpec{
		{Name: "first", Command: "/bin/sh", CommandArgs: []string{"-c", "sleep 0.05"}, Turn: 5},
		{Name: "second", Command: "/bin/sh", CommandArgs: []string{"-c", "sleep 0.05"}, Turn: 0},
	})
	if err := sr.startProcesses(); err != nil {
		t.Fatalf("startProcesses: %v", err)
	}

	sr.processesMu.Lock()
	got := sr.current
	sr.processesMu.Unlock()
	if got != "first" {
		t.Fatalf("current = %q; want %q (first in spec order, not lowest turn)", got, "first")
	}
}

// TestSwitch_InvalidNameRejected asserts Switch validates the process name and
// rejects an unknown one with a wrapped ErrUnknownProcess, leaving the current
// focus unchanged.
func TestSwitch_InvalidNameRejected(t *testing.T) {
	sr := newProcessesTestExec(t, []api.ProcessSpec{
		{Name: "a", Command: "/bin/sh", CommandArgs: []string{"-c", "sleep 0.05"}},
	})
	if err := sr.startProcesses(); err != nil {
		t.Fatalf("startProcesses: %v", err)
	}

	err := sr.Switch("ghost")
	if !errors.Is(err, ErrUnknownProcess) {
		t.Fatalf("Switch(ghost) error = %v; want wrapped ErrUnknownProcess", err)
	}

	sr.processesMu.Lock()
	got := sr.current
	sr.processesMu.Unlock()
	if got != "a" {
		t.Fatalf("current = %q after rejected switch; want unchanged %q", got, "a")
	}
}

// TestSwitch_OutputFollowsFocus asserts that only the current process's output
// reaches the operator while non-current processes keep draining to their own
// capture files, and that Switch flips which process the operator sees.
func TestSwitch_OutputFollowsFocus(t *testing.T) {
	dir := t.TempDir()
	capA := filepath.Join(dir, "a.cap")
	capB := filepath.Join(dir, "b.cap")

	// Two self-emitting processes so a non-current process is actively
	// producing output (the case that exercises operator-side isolation).
	sr := newProcessesTestExec(t, []api.ProcessSpec{
		{
			Name:        "a",
			Command:     "/bin/sh",
			CommandArgs: []string{"-c", "while :; do printf A; sleep 0.02; done"},
			CaptureFile: capA,
		},
		{
			Name:        "b",
			Command:     "/bin/sh",
			CommandArgs: []string{"-c", "while :; do printf B; sleep 0.02; done"},
			CaptureFile: capB,
		},
	})
	if err := sr.startProcesses(); err != nil {
		t.Fatalf("startProcesses: %v", err)
	}

	_, multiOutW := sr.testOperatorPipes(t)
	sink := &syncBuf{}
	multiOutW.Add(sink)

	// current == a: the operator must see only 'A', never 'B' from the
	// non-current process.
	waitUntil(t, func() bool { return sink.len() > 0 }, "operator received initial output")
	time.Sleep(60 * time.Millisecond) // a few emit cycles
	if got := sink.bytes(); bytes.ContainsRune(got, 'B') {
		t.Fatalf("operator saw 'B' from non-current process while a is current: %q", got)
	}

	if err := sr.Switch("b"); err != nil {
		t.Fatalf("Switch(b): %v", err)
	}

	// After the switch the operator must start seeing 'B'. Once a 'B' has
	// landed, every subsequent byte must be 'B' — the now-non-current 'a'
	// must no longer reach the operator.
	waitUntil(t, func() bool { return bytes.ContainsRune(sink.bytes(), 'B') },
		"operator received b output after switch")
	mark := sink.len()
	time.Sleep(60 * time.Millisecond)
	tail := sink.bytes()[mark:]
	if len(tail) == 0 {
		t.Fatal("operator received no output after switch settled")
	}
	if bytes.ContainsRune(tail, 'A') {
		t.Fatalf("operator saw 'A' from non-current process after switch: %q", tail)
	}

	// Both processes drain to their own capture files regardless of focus.
	waitUntil(t, func() bool {
		a, _ := os.ReadFile(capA)
		b, _ := os.ReadFile(capB)
		return len(a) > 0 && len(b) > 0
	}, "both capture files received output independent of focus")
}

// TestSwitch_InputFollowsFocus asserts operator input is delivered only to the
// current process, and that Switch redirects subsequent input. Each process is
// a cat that echoes its stdin to stdout (drained into its capture file), so the
// capture files are a faithful record of what input each process received.
func TestSwitch_InputFollowsFocus(t *testing.T) {
	dir := t.TempDir()
	capA := filepath.Join(dir, "a.cap")
	capB := filepath.Join(dir, "b.cap")

	sr := newProcessesTestExec(t, []api.ProcessSpec{
		{Name: "a", Command: "/bin/cat", CaptureFile: capA},
		{Name: "b", Command: "/bin/cat", CaptureFile: capB},
	})
	if err := sr.startProcesses(); err != nil {
		t.Fatalf("startProcesses: %v", err)
	}
	pipeInW, _ := sr.testOperatorPipes(t)

	// current == a: input reaches a only.
	if _, err := pipeInW.WriteString("to-a"); err != nil {
		t.Fatalf("write to-a: %v", err)
	}
	waitCapture(t, capA, "to-a")
	if b, _ := os.ReadFile(capB); len(b) != 0 {
		t.Fatalf("process b capture = %q; want empty while a is current", b)
	}

	// Switch to b: subsequent input reaches b only; a's record is unchanged.
	if err := sr.Switch("b"); err != nil {
		t.Fatalf("Switch(b): %v", err)
	}
	if _, err := pipeInW.WriteString("to-b"); err != nil {
		t.Fatalf("write to-b: %v", err)
	}
	waitCapture(t, capB, "to-b")
	waitCapture(t, capA, "to-a")
}

// TestSwitch_ConcurrentSwitchAndInput drives Switch and operator input
// concurrently. It is a race-detector guard: the focus state is shared between
// the Switch caller, the input relay, and the drain goroutines, all under
// processesMu. Every Switch onto a valid name must succeed.
func TestSwitch_ConcurrentSwitchAndInput(t *testing.T) {
	dir := t.TempDir()
	sr := newProcessesTestExec(t, []api.ProcessSpec{
		{Name: "a", Command: "/bin/cat", CaptureFile: filepath.Join(dir, "a.cap")},
		{Name: "b", Command: "/bin/cat", CaptureFile: filepath.Join(dir, "b.cap")},
	})
	if err := sr.startProcesses(); err != nil {
		t.Fatalf("startProcesses: %v", err)
	}
	pipeInW, _ := sr.testOperatorPipes(t)

	const iters = 300
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		names := []api.ProcessName{"a", "b"}
		for i := range iters {
			if err := sr.Switch(names[i%2]); err != nil {
				t.Errorf("Switch(%q): %v", names[i%2], err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range iters {
			if _, err := pipeInW.WriteString("x"); err != nil {
				t.Errorf("operator write: %v", err)
				return
			}
		}
	}()

	wg.Wait()
}
