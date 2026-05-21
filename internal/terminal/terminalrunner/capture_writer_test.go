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
	w, err := newCaptureWriter(canonical, logger, nil, captureFormatOpts{})
	if err != nil {
		t.Fatalf("newCaptureWriter: %v", err)
	}
	w.maxBytes = maxBytes
	w.retainSegments = retainSegments
	w.retainBytes = retainBytes
	return w
}

// gzSize returns the on-disk gzip size CompressSegment produces for payload.
// Closed segments are compressed on rotation, so byte-retention caps are set
// relative to the compressed size rather than the raw payload length.
func gzSize(t *testing.T, payload []byte) int64 {
	t.Helper()
	raw := filepath.Join(t.TempDir(), "sample.000000")
	if err := os.WriteFile(raw, payload, 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	gzPath, err := capture.CompressSegment(raw)
	if err != nil {
		t.Fatalf("compress sample: %v", err)
	}
	info, err := os.Stat(gzPath)
	if err != nil {
		t.Fatalf("stat sample gz: %v", err)
	}
	return info.Size()
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
	// The closed segment is gzip-compressed; the canonical (live) stays raw.
	if !segs[0].Compressed {
		t.Fatalf("closed segment should be compressed: %+v", segs[0])
	}
	if filepath.Ext(segs[0].Path) != capture.GzSuffix {
		t.Fatalf("closed segment path = %q, want a %q suffix", segs[0].Path, capture.GzSuffix)
	}
	// Reassembly transparently decompresses the closed segment, then the live
	// tail: the full transcript reads back as everything written.
	full, err := capture.ReadAll(canonical)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(full, []byte("12345678tail")) {
		t.Fatalf("reassembled = %q, want %q", full, "12345678tail")
	}
	// cat of the canonical shows the live tail only — never compressed.
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
	// The two surviving closed segments must be the newest two written before
	// the final (empty) live segment: "EEEE" then "FFFF". They are gzip-
	// compressed, so verify content through the decompressing reassembler.
	if !segs[0].Compressed || !segs[1].Compressed {
		t.Fatalf("surviving closed segments should be compressed: %+v", segs)
	}
	got, err := capture.ReadAll(canonical)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, []byte("EEEEFFFF")) {
		t.Fatalf("reassembled survivors = %q, want %q", got, "EEEEFFFF")
	}
}

// Byte-based retention prunes oldest until total closed bytes fit the cap,
// independent of the count cap.
func TestCaptureWriter_BytePrune(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	// Identical 8-byte payloads each compress to a fixed size g once closed;
	// a byte cap of 2*g (no count cap) must retain exactly the newest two.
	payload := []byte("PAYLOAD8")
	g := gzSize(t, payload)
	w := newTestCaptureWriter(t, canonical, int64(len(payload)), 0, 2*g)

	for range 4 {
		mustWrite(t, w, payload) // each write hits the threshold and rotates
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	segs, err := capture.ClosedSegments(canonical)
	if err != nil {
		t.Fatalf("ClosedSegments: %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("byte retention should keep newest 2 (cap=2*%d), got %d", g, len(segs))
	}
	var total int64
	for _, s := range segs {
		total += s.Size
	}
	if total > 2*g {
		t.Fatalf("byte retention exceeded: total closed bytes = %d, cap %d", total, 2*g)
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

// newTestAsciicastWriter mirrors newTestCaptureWriter but selects the
// asciicast format, so rotation/header-survival is exercisable with small
// thresholds.
func newTestAsciicastWriter(
	t *testing.T,
	canonical string,
	maxBytes int64,
	retainSegments int,
	retainBytes int64,
) *captureWriter {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w, err := newCaptureWriter(canonical, logger, nil, captureFormatOpts{
		Asciicast: true,
		Cols:      80,
		Rows:      24,
	})
	if err != nil {
		t.Fatalf("newCaptureWriter: %v", err)
	}
	w.maxBytes = maxBytes
	w.retainSegments = retainSegments
	w.retainBytes = retainBytes
	return w
}

// In asciicast mode the live file is valid asciicast v2 (header line + one
// timed "o" record per Write) and decodes back to exactly the bytes written.
func TestCaptureWriter_AsciicastLiveFileValid(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	w := newTestAsciicastWriter(t, canonical, 1<<20, 8, 1<<30)

	mustWrite(t, w, []byte("hello "))
	mustWrite(t, w, []byte("world\r\n"))
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	data, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatalf("read canonical: %v", err)
	}
	if !capture.IsAsciicast(data) {
		t.Fatalf("live file not valid asciicast v2: %q", data)
	}
	// One header line + one record per write = 3 lines.
	if got := bytes.Count(data, []byte("\n")); got != 3 {
		t.Fatalf("line count = %d, want 3 (header + 2 records)", got)
	}
	if got, want := capture.DecodeAsciicast(data), []byte("hello world\r\n"); !bytes.Equal(got, want) {
		t.Fatalf("decoded live file = %q, want %q", got, want)
	}
}

// Asciicast rotation writes a header at the top of every segment, so the
// reassembled transcript decodes correctly across a rotation boundary and
// stays valid even after the oldest segment (segment 0) is pruned.
func TestCaptureWriter_AsciicastRotationHeaderPerSegment(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	// Tiny threshold so each write rotates, producing several closed segments.
	w := newTestAsciicastWriter(t, canonical, 1, 8, 1<<30)

	for _, p := range [][]byte{[]byte("AAA"), []byte("BBB"), []byte("CCC")} {
		mustWrite(t, w, p)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	segs, err := capture.ClosedSegments(canonical)
	if err != nil {
		t.Fatalf("ClosedSegments: %v", err)
	}
	if len(segs) == 0 {
		t.Fatalf("expected rotation to produce closed segments")
	}

	// Full reassembly decodes to all payloads in order across rotations.
	full, err := capture.Replay(canonical)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if want := []byte("AAABBBCCC"); !bytes.Equal(full, want) {
		t.Fatalf("reassembled+decoded = %q, want %q", full, want)
	}

	// Header survives pruning: drop the oldest closed segment (segment 0) and
	// the reassembled stream is still sniffed as asciicast and still decodes.
	if rmErr := os.Remove(segs[0].Path); rmErr != nil {
		t.Fatalf("prune segment 0: %v", rmErr)
	}
	reassembled, err := capture.ReadAll(canonical)
	if err != nil {
		t.Fatalf("ReadAll after prune: %v", err)
	}
	if !capture.IsAsciicast(reassembled) {
		t.Fatalf("stream not asciicast after pruning segment 0: %q", reassembled)
	}
	decoded := capture.DecodeAsciicast(reassembled)
	// The first payload ("AAA") was in segment 0 and is gone; the remainder
	// survives and still decodes.
	if !bytes.Contains(decoded, []byte("CCC")) {
		t.Fatalf("post-prune decode lost the live tail: %q", decoded)
	}
}
