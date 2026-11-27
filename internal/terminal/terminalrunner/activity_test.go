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
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

// newActivityExec wires an Exec whose PTY master is the read end of an
// os.Pipe, with terminalManagerReader fanning it out to the Exec's live
// multiOutW (no endpoints unless the test adds one). Bytes
// written to the returned *os.File reach the reader verbatim — no line
// discipline (ONLCR, echo) or shell prompt to perturb the count — so tests
// can assert exact BytesOut. startedAt is stamped the way
// NewTerminalRunnerExec stamps it.
func newActivityExec(t *testing.T) (*Exec, *os.File) {
	t.Helper()
	sr := newWiredExec(t)
	sr.startedAt = time.Now()

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	sr.ptmx = pr

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		sr.terminalManagerReader(sr.ptyPipes.multiOutW)
	}()
	t.Cleanup(func() {
		// Cancelling ctx makes the reader's ctx goroutine close ptmx, which
		// unblocks Read and returns the reader.
		sr.ctxCancel()
		_ = pw.Close()
		select {
		case <-readerDone:
		case <-time.After(5 * time.Second):
			t.Error("terminalManagerReader did not exit after ctx cancel")
		}
	})
	return sr, pw
}

func mustActivity(t *testing.T, sr *Exec) *api.ActivityResult {
	t.Helper()
	res, err := sr.Activity(&api.ActivityArgs{})
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if res == nil {
		t.Fatal("Activity returned nil result")
	}
	return res
}

// waitBytesOut polls Activity until BytesOut reaches want or the deadline
// passes; the reader goroutine accounts bytes asynchronously.
func waitBytesOut(t *testing.T, sr *Exec, want uint64) *api.ActivityResult {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		res := mustActivity(t, sr)
		if res.BytesOut == want {
			return res
		}
		if res.BytesOut > want {
			t.Fatalf("BytesOut = %d, overshot want %d", res.BytesOut, want)
		}
		if time.Now().After(deadline) {
			t.Fatalf("BytesOut = %d after 5s, want %d", res.BytesOut, want)
		}
		time.Sleep(time.Millisecond)
	}
}

// syncBuffer is an io.Writer standing in for an attached client's output
// endpoint on the PTY fan-out.
type syncBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *syncBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.buf)
}

func writeAll(t *testing.T, w io.Writer, p []byte) {
	t.Helper()
	if _, err := w.Write(p); err != nil {
		t.Fatalf("write to pty stand-in: %v", err)
	}
}

// A freshly constructed runner answers Activity before StartTerminal: the
// counters are zero, no output has been seen, and StartedAt marks where
// NewTerminalRunnerExec zeroed the counters.
func TestActivity_ConstructorZeroState(t *testing.T) {
	before := time.Now()
	tr := NewTerminalRunnerExec(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		&api.TerminalSpec{ID: "activity-ctor", RunPath: t.TempDir()},
	)
	sr, ok := tr.(*Exec)
	if !ok {
		t.Fatalf("NewTerminalRunnerExec returned %T, want *Exec", tr)
	}
	t.Cleanup(sr.ctxCancel)
	after := time.Now()

	res := mustActivity(t, sr)
	if res.StartedAt.IsZero() {
		t.Fatal("StartedAt is zero; want construction time")
	}
	if res.StartedAt.Before(before) || res.StartedAt.After(after) {
		t.Errorf("StartedAt = %v, want within constructor call [%v, %v]", res.StartedAt, before, after)
	}
	if res.BytesOut != 0 || res.BytesIn != 0 {
		t.Errorf("BytesOut/BytesIn = %d/%d, want 0/0", res.BytesOut, res.BytesIn)
	}
	if !res.LastOutputAt.IsZero() {
		t.Errorf("LastOutputAt = %v, want zero before any output", res.LastOutputAt)
	}
	if res.IdleFor != 0 {
		t.Errorf("IdleFor = %v, want 0 before any output", res.IdleFor)
	}
}

func TestActivity_ZeroState(t *testing.T) {
	sr, _ := newActivityExec(t)
	res := mustActivity(t, sr)
	if res.StartedAt.IsZero() {
		t.Error("StartedAt is zero")
	}
	if res.BytesOut != 0 {
		t.Errorf("BytesOut = %d, want 0", res.BytesOut)
	}
	if !res.LastOutputAt.IsZero() {
		t.Errorf("LastOutputAt = %v, want zero", res.LastOutputAt)
	}
	if res.IdleFor != 0 {
		t.Errorf("IdleFor = %v, want 0", res.IdleFor)
	}
}

func TestActivity_OutputWithNoClientAttached(t *testing.T) {
	sr, pw := newActivityExec(t)
	start := mustActivity(t, sr)

	data := []byte("hello\nworld\n")
	writeAll(t, pw, data)
	res := waitBytesOut(t, sr, uint64(len(data)))
	if res.LastOutputAt.IsZero() {
		t.Error("LastOutputAt is zero after output")
	}
	if res.LastOutputAt.Before(start.StartedAt) {
		t.Errorf("LastOutputAt %v precedes StartedAt %v", res.LastOutputAt, start.StartedAt)
	}

	more := []byte("0123456789")
	writeAll(t, pw, more)
	waitBytesOut(t, sr, uint64(len(data)+len(more)))
}

func TestActivity_OutputWithClientAttached(t *testing.T) {
	sr, pw := newActivityExec(t)
	attached := &syncBuffer{}
	sr.ptyPipes.multiOutW.Add(attached)

	data := []byte("attached output\n")
	writeAll(t, pw, data)
	waitBytesOut(t, sr, uint64(len(data)))
	deadline := time.Now().Add(5 * time.Second)
	for attached.Len() < len(data) {
		if time.Now().After(deadline) {
			t.Fatalf("attached endpoint got %d bytes, want %d", attached.Len(), len(data))
		}
		time.Sleep(time.Millisecond)
	}
	if got := mustActivity(t, sr).BytesOut; got != uint64(len(data)) {
		t.Errorf("BytesOut = %d with a client attached, want %d", got, len(data))
	}
}

func TestActivity_IdleForGrows(t *testing.T) {
	sr, pw := newActivityExec(t)
	writeAll(t, pw, []byte("x"))
	first := waitBytesOut(t, sr, 1)
	time.Sleep(5 * time.Millisecond)
	second := mustActivity(t, sr)
	if second.BytesOut != first.BytesOut {
		t.Fatalf("BytesOut changed with no output: %d -> %d", first.BytesOut, second.BytesOut)
	}
	if !second.LastOutputAt.Equal(first.LastOutputAt) {
		t.Errorf("LastOutputAt changed with no output: %v -> %v", first.LastOutputAt, second.LastOutputAt)
	}
	if second.IdleFor <= first.IdleFor {
		t.Errorf("IdleFor did not grow: %v -> %v", first.IdleFor, second.IdleFor)
	}
}

func TestActivity_StartedAtConstant(t *testing.T) {
	sr, pw := newActivityExec(t)
	first := mustActivity(t, sr)
	writeAll(t, pw, []byte("abc"))
	waitBytesOut(t, sr, 3)
	second := mustActivity(t, sr)
	if !second.StartedAt.Equal(first.StartedAt) {
		t.Errorf("StartedAt changed across calls: %v -> %v", first.StartedAt, second.StartedAt)
	}
}

func TestActivity_OutputGateOffStillCounts(t *testing.T) {
	sr, pw := newActivityExec(t)
	sr.obsMu.Lock()
	sr.gates.OutputOn = false
	sr.obsMu.Unlock()

	data := []byte("drained but counted")
	writeAll(t, pw, data)
	res := waitBytesOut(t, sr, uint64(len(data)))
	if res.LastOutputAt.IsZero() {
		t.Error("LastOutputAt is zero after gated-off output")
	}
}

// Activity is a pure in-memory read: it must not add an attacher, move
// LastAttachedAt, or rewrite metadata.json.
func TestActivity_NoSideEffects(t *testing.T) {
	sr, pw := newActivityExec(t)
	if err := sr.CreateMetadata(); err != nil {
		t.Fatalf("CreateMetadata: %v", err)
	}
	metaPath := filepath.Join(sr.metadata.Spec.RunPath, "terminals", string(sr.id), "metadata.json")
	// Age the file so a rewrite inside the test would be visible even on
	// filesystems with coarse mtime granularity.
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(metaPath, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	beforeBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read metadata.json: %v", err)
	}

	sr.metadataMu.RLock()
	beforeAttachers := slices.Clone(sr.metadata.Status.Attachers)
	beforeLastAttached := sr.metadata.Status.LastAttachedAt
	sr.metadataMu.RUnlock()

	writeAll(t, pw, []byte("output"))
	waitBytesOut(t, sr, 6)
	for range 10 {
		mustActivity(t, sr)
	}

	sr.metadataMu.RLock()
	afterAttachers := slices.Clone(sr.metadata.Status.Attachers)
	afterLastAttached := sr.metadata.Status.LastAttachedAt
	sr.metadataMu.RUnlock()
	if !slices.Equal(afterAttachers, beforeAttachers) {
		t.Errorf("Attachers changed: %v -> %v", beforeAttachers, afterAttachers)
	}
	if !afterLastAttached.Equal(beforeLastAttached) {
		t.Errorf("LastAttachedAt changed: %v -> %v", beforeLastAttached, afterLastAttached)
	}

	st, err := os.Stat(metaPath)
	if err != nil {
		t.Fatalf("stat metadata.json: %v", err)
	}
	if !st.ModTime().Equal(old) {
		t.Errorf("metadata.json mtime changed: %v -> %v", old, st.ModTime())
	}
	afterBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read metadata.json: %v", err)
	}
	if string(afterBytes) != string(beforeBytes) {
		t.Error("metadata.json content changed across Activity calls")
	}
}

// The PTY reader mutates the counters while Activity reads them; under
// -race (make test-race) this must not report.
func TestActivity_ConcurrentWithReader(t *testing.T) {
	sr, pw := newActivityExec(t)

	const writes = 200
	chunk := []byte("race-chunk\n")
	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				if _, err := sr.Activity(&api.ActivityArgs{}); err != nil {
					t.Errorf("Activity: %v", err)
					return
				}
			}
		}
	}()

	for range writes {
		writeAll(t, pw, chunk)
	}
	waitBytesOut(t, sr, uint64(writes*len(chunk)))
	close(stop)
	wg.Wait()
}
