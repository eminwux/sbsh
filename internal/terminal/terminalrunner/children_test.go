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
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

// childTestTimeout bounds how long the child-set tests wait for a child to
// exit or for its capture transcript to flush.
const childTestTimeout = 5 * time.Second

// newChildrenTestExec builds a minimal non-init-mode Exec for exercising the
// child-set spawn path in isolation — no PTY, no RPC, no metadata persistence.
// The caller seeds metadata.Spec.Children before calling startChildren.
func newChildrenTestExec(t *testing.T, specs []api.ChildSpec) *Exec {
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
	sr.metadata.Spec.Children = specs
	return sr
}

// childByName returns the spawned childProc with the given spec name.
func (sr *Exec) childByName(t *testing.T, name string) *childProc {
	t.Helper()
	sr.childrenMu.Lock()
	defer sr.childrenMu.Unlock()
	for _, c := range sr.children {
		if c.spec.Name == name {
			return c
		}
	}
	t.Fatalf("no spawned child named %q", name)
	return nil
}

// waitDone blocks until the child's exit is observed or childTestTimeout
// elapses.
func waitDone(t *testing.T, c *childProc) {
	t.Helper()
	select {
	case <-c.doneCh:
	case <-time.After(childTestTimeout):
		t.Fatalf("child %q exit not observed within %s", c.spec.Name, childTestTimeout)
	}
}

// waitCapture polls the capture file until its contents equal want or
// childTestTimeout elapses. The drain goroutine finishes flushing slightly
// after the child exits, so a direct read right after waitDone can race the
// final write.
func waitCapture(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(childTestTimeout)
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

// TestStartChildren_TwoChildSpawn asserts a two-child spec spawns both
// children, drains each child's stdout into its own capture file with the
// expected content, and observes each child's clean exit.
func TestStartChildren_TwoChildSpawn(t *testing.T) {
	dir := t.TempDir()
	capA := filepath.Join(dir, "a.cap")
	capB := filepath.Join(dir, "b.cap")

	sr := newChildrenTestExec(t, []api.ChildSpec{
		{Name: "a", Command: "/bin/sh", CommandArgs: []string{"-c", "printf 'hello-A'"}, CaptureFile: capA},
		{Name: "b", Command: "/bin/sh", CommandArgs: []string{"-c", "printf 'hello-B'"}, CaptureFile: capB},
	})

	if err := sr.startChildren(); err != nil {
		t.Fatalf("startChildren: %v", err)
	}

	sr.childrenMu.Lock()
	got := len(sr.children)
	sr.childrenMu.Unlock()
	if got != 2 {
		t.Fatalf("spawned %d children; want 2", got)
	}

	ca := sr.childByName(t, "a")
	cb := sr.childByName(t, "b")
	waitDone(t, ca)
	waitDone(t, cb)
	if ca.exitErr != nil {
		t.Errorf("child a exitErr = %v; want nil (clean exit)", ca.exitErr)
	}
	if cb.exitErr != nil {
		t.Errorf("child b exitErr = %v; want nil (clean exit)", cb.exitErr)
	}

	waitCapture(t, capA, "hello-A")
	waitCapture(t, capB, "hello-B")
}

// TestStartChildren_CaptureMode asserts the child capture file is chmod'd to
// the resolved ChildSpec.CaptureMode after open.
func TestStartChildren_CaptureMode(t *testing.T) {
	dir := t.TempDir()
	capPath := filepath.Join(dir, "c.cap")

	sr := newChildrenTestExec(t, []api.ChildSpec{
		{
			Name:        "c",
			Command:     "/bin/sh",
			CommandArgs: []string{"-c", "printf x"},
			CaptureFile: capPath,
			CaptureMode: 0o640,
		},
	})
	if err := sr.startChildren(); err != nil {
		t.Fatalf("startChildren: %v", err)
	}
	waitDone(t, sr.childByName(t, "c"))

	info, err := os.Stat(capPath)
	if err != nil {
		t.Fatalf("stat capture: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o640 {
		t.Fatalf("capture mode = %o; want 0o640", perm)
	}
}

// TestStartChildren_ExitObservation asserts a non-zero child exit is observed
// and surfaced via the child's exitErr.
func TestStartChildren_ExitObservation(t *testing.T) {
	sr := newChildrenTestExec(t, []api.ChildSpec{
		{Name: "fail", Command: "/bin/sh", CommandArgs: []string{"-c", "exit 7"}},
	})
	if err := sr.startChildren(); err != nil {
		t.Fatalf("startChildren: %v", err)
	}

	c := sr.childByName(t, "fail")
	waitDone(t, c)
	if c.exitErr == nil {
		t.Fatalf("exit 7 child exitErr = nil; want non-nil")
	}
}

// TestStartChildren_TurnOrderedSpawn asserts the turn barrier: every child in
// turn N is fork+exec'd before any child in turn N+1. spawnChild appends to
// sr.children only after cmd.Start returns, and spawnGroup blocks until all of
// its group's spawnChild calls have returned, so sr.children's Turn values are
// non-decreasing across the slice. Same-turn children (turns 0 and 1 each have
// two members here) may appear in any intra-group order.
func TestStartChildren_TurnOrderedSpawn(t *testing.T) {
	sr := newChildrenTestExec(t, []api.ChildSpec{
		{Name: "t1a", Command: "/bin/sh", CommandArgs: []string{"-c", "sleep 0.05"}, Turn: 1},
		{Name: "t0a", Command: "/bin/sh", CommandArgs: []string{"-c", "sleep 0.05"}, Turn: 0},
		{Name: "t2a", Command: "/bin/sh", CommandArgs: []string{"-c", "sleep 0.05"}, Turn: 2},
		{Name: "t0b", Command: "/bin/sh", CommandArgs: []string{"-c", "sleep 0.05"}, Turn: 0},
		{Name: "t1b", Command: "/bin/sh", CommandArgs: []string{"-c", "sleep 0.05"}, Turn: 1},
	})
	if err := sr.startChildren(); err != nil {
		t.Fatalf("startChildren: %v", err)
	}

	sr.childrenMu.Lock()
	defer sr.childrenMu.Unlock()
	if len(sr.children) != 5 {
		t.Fatalf("spawned %d children; want 5", len(sr.children))
	}
	prev := -1
	for i, c := range sr.children {
		if c.spec.Turn < prev {
			t.Fatalf("children[%d] (%q) turn=%d follows turn=%d; spawn order not turn-monotonic",
				i, c.spec.Name, c.spec.Turn, prev)
		}
		prev = c.spec.Turn
	}
	// turn 0 children must be the first two spawned.
	for i := range 2 {
		if sr.children[i].spec.Turn != 0 {
			t.Fatalf("children[%d] turn=%d; want turn 0 first", i, sr.children[i].spec.Turn)
		}
	}
}

// TestOrderedTurns asserts distinct turns come back ascending and deduplicated.
func TestOrderedTurns(t *testing.T) {
	specs := []api.ChildSpec{
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

// TestChildrenWithTurn asserts the group filter preserves declaration order.
func TestChildrenWithTurn(t *testing.T) {
	specs := []api.ChildSpec{
		{Name: "a", Turn: 0},
		{Name: "b", Turn: 1},
		{Name: "c", Turn: 0},
	}
	group := childrenWithTurn(specs, 0)
	if len(group) != 2 || group[0].Name != "a" || group[1].Name != "c" {
		t.Fatalf("childrenWithTurn(0) = %+v; want [a c] in order", group)
	}
}
