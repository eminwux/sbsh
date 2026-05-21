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
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/internal/capture"
)

// newTestCaptureWriter builds a captureWriter with explicit, small thresholds
// so rotation/retention is exercisable without writing megabytes. It bypasses
// the defaults-derived constructor knobs but reuses newCaptureWriter for the
// open + seq-seed path, then overrides the bounds.
func newTestCaptureWriter(
	t *testing.T,
	canonical string,
	maxBytes int64,
	retainSegments int,
	retainBytes int64,
) *captureWriter {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w, err := newCaptureWriter(canonical, logger, nil)
	if err != nil {
		t.Fatalf("newCaptureWriter: %v", err)
	}
	w.maxBytes = maxBytes
	w.retainSegments = retainSegments
	w.retainBytes = retainBytes
	return w
}

func mustWrite(t *testing.T, w io.Writer, b []byte) {
	t.Helper()
	n, err := w.Write(b)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != len(b) {
		t.Fatalf("short write: %d of %d", n, len(b))
	}
}

// Under the threshold, nothing rotates: the canonical holds everything and no
// closed siblings exist.
func TestCaptureWriter_NoRotation(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	w := newTestCaptureWriter(t, canonical, 1<<20, 8, 1<<30)

	mustWrite(t, w, []byte("small payload"))
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	segs, err := capture.ClosedSegments(canonical)
	if err != nil {
		t.Fatalf("ClosedSegments: %v", err)
	}
	if len(segs) != 0 {
		t.Fatalf("expected no rotation, got %d closed segments", len(segs))
	}
	data, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatalf("read canonical: %v", err)
	}
	if !bytes.Equal(data, []byte("small payload")) {
		t.Fatalf("canonical = %q, want %q", data, "small payload")
	}
}

// Crossing the threshold once produces exactly one closed segment plus a fresh
// live segment, and `cat`-ing the canonical shows only the live tail.
func TestCaptureWriter_SingleRotation(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	w := newTestCaptureWriter(t, canonical, 8, 8, 1<<30)

	mustWrite(t, w, []byte("12345678")) // hits threshold -> rotates after write
	mustWrite(t, w, []byte("tail"))     // lands in the fresh live segment
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	segs, err := capture.ClosedSegments(canonical)
	if err != nil {
		t.Fatalf("ClosedSegments: %v", err)
	}
	if len(segs) != 1 {
		t.Fatalf("expected 1 closed segment, got %d", len(segs))
	}
	closed, err := os.ReadFile(segs[0].Path)
	if err != nil {
		t.Fatalf("read closed: %v", err)
	}
	if !bytes.Equal(closed, []byte("12345678")) {
		t.Fatalf("closed segment = %q, want %q", closed, "12345678")
	}
	// cat of the canonical shows the live tail only.
	live, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatalf("read canonical: %v", err)
	}
	if !bytes.Equal(live, []byte("tail")) {
		t.Fatalf("live tail = %q, want %q", live, "tail")
	}
}

// Many rotations with a tight retention cap: the oldest closed segments are
// pruned so the closed-segment count stays bounded, yet the reassembled stream
// remains gap/dupe-free over whatever survives.
func TestCaptureWriter_MultipleRotationsAndPrune(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	// 4-byte segments, keep at most 2 closed segments, generous byte cap.
	w := newTestCaptureWriter(t, canonical, 4, 2, 1<<30)

	// 6 writes of 4 bytes: each crosses the threshold and rotates, producing
	// 6 closed segments before retention, capped to the newest 2.
	chunks := [][]byte{
		[]byte("AAAA"), []byte("BBBB"), []byte("CCCC"),
		[]byte("DDDD"), []byte("EEEE"), []byte("FFFF"),
	}
	for _, c := range chunks {
		mustWrite(t, w, c)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	segs, err := capture.ClosedSegments(canonical)
	if err != nil {
		t.Fatalf("ClosedSegments: %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("expected retention to cap closed segments at 2, got %d", len(segs))
	}
	// The two surviving closed segments must be the newest two written
	// before the final (empty) live segment: "EEEE" and "FFFF".
	got0, _ := os.ReadFile(segs[0].Path)
	got1, _ := os.ReadFile(segs[1].Path)
	if !bytes.Equal(got0, []byte("EEEE")) || !bytes.Equal(got1, []byte("FFFF")) {
		t.Fatalf("surviving segments = %q,%q, want EEEE,FFFF", got0, got1)
	}
}

// Byte-based retention prunes oldest until total closed bytes fit the cap,
// independent of the count cap.
func TestCaptureWriter_BytePrune(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	// 4-byte segments, no count cap, keep at most 8 closed bytes (2 segments).
	w := newTestCaptureWriter(t, canonical, 4, 0, 8)

	for _, c := range [][]byte{[]byte("AAAA"), []byte("BBBB"), []byte("CCCC"), []byte("DDDD")} {
		mustWrite(t, w, c)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	segs, err := capture.ClosedSegments(canonical)
	if err != nil {
		t.Fatalf("ClosedSegments: %v", err)
	}
	var total int64
	for _, s := range segs {
		total += s.Size
	}
	if total > 8 {
		t.Fatalf("byte retention exceeded: total closed bytes = %d, cap 8", total)
	}
}

// A reader spanning the segment boundary sees the full stream in order with no
// gaps or duplicates — the core reassembly contract.
func TestCaptureWriter_ReaderSpansBoundary(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	// Small threshold, retention large enough that nothing is pruned, so the
	// reassembled stream must equal every byte written.
	w := newTestCaptureWriter(t, canonical, 8, 100, 1<<30)

	var want bytes.Buffer
	chunks := []string{
		"first chunk ", "second chunk ", "third chunk ",
		"fourth chunk ", "fifth chunk",
	}
	for _, c := range chunks {
		mustWrite(t, w, []byte(c))
		want.WriteString(c)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Multiple rotations must have occurred.
	segs, err := capture.ClosedSegments(canonical)
	if err != nil {
		t.Fatalf("ClosedSegments: %v", err)
	}
	if len(segs) < 2 {
		t.Fatalf("expected multiple rotations, got %d closed segments", len(segs))
	}

	got, err := capture.ReadAll(canonical)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Fatalf("reassembled stream mismatch:\n got %q\nwant %q", got, want.Bytes())
	}
}

// Sequence numbering resumes across a writer restart on the same canonical
// path, so a later run does not collide with or reorder earlier segments.
func TestCaptureWriter_SeqResumesAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")

	w1 := newTestCaptureWriter(t, canonical, 4, 100, 1<<30)
	mustWrite(t, w1, []byte("AAAA")) // rotates -> capture.000000
	mustWrite(t, w1, []byte("BBBB")) // rotates -> capture.000001
	if err := w1.Close(); err != nil {
		t.Fatalf("close w1: %v", err)
	}

	// Restart: the live segment from w1 is empty (last write rotated), so the
	// new writer must seed nextSeq past the existing closed segments.
	w2 := newTestCaptureWriter(t, canonical, 4, 100, 1<<30)
	mustWrite(t, w2, []byte("CCCC")) // rotates -> capture.000002
	if err := w2.Close(); err != nil {
		t.Fatalf("close w2: %v", err)
	}

	got, err := capture.ReadAll(canonical)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if want := []byte("AAAABBBBCCCC"); !bytes.Equal(got, want) {
		t.Fatalf("after restart ReadAll = %q, want %q", got, want)
	}
}
