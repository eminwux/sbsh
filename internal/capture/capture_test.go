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

package capture

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func TestSegmentPath(t *testing.T) {
	got := SegmentPath("/run/t/capture", 7)
	want := "/run/t/capture.000007"
	if got != want {
		t.Fatalf("SegmentPath = %q, want %q", got, want)
	}
}

func TestClosedSegments_OrderAndFilter(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")

	// Closed segments out of lexical/creation order; ReadDir order is not
	// guaranteed, so the sort must impose ascending sequence.
	writeFile(t, SegmentPath(canonical, 2), []byte("two"))
	writeFile(t, SegmentPath(canonical, 0), []byte("zero"))
	writeFile(t, SegmentPath(canonical, 10), []byte("ten!!"))
	// A compressed closed segment must be spliced in (seq 1, ".gz" form).
	writeFile(t, SegmentPath(canonical, 1)+GzSuffix, []byte("x"))
	// Live segment — must not appear among closed segments.
	writeFile(t, canonical, []byte("live"))
	// Unrelated siblings that must be ignored: non-digit suffix and an
	// in-flight compression temp file.
	writeFile(t, canonical+".notes", []byte("x"))
	writeFile(t, SegmentPath(canonical, 5)+GzSuffix+".tmp", []byte("x"))

	segs, err := ClosedSegments(canonical)
	if err != nil {
		t.Fatalf("ClosedSegments: %v", err)
	}
	if len(segs) != 4 {
		t.Fatalf("got %d closed segments, want 4: %+v", len(segs), segs)
	}
	wantSeq := []uint64{0, 1, 2, 10}
	for i, s := range segs {
		if s.Seq != wantSeq[i] {
			t.Fatalf("segment %d seq = %d, want %d", i, s.Seq, wantSeq[i])
		}
	}
	if !segs[1].Compressed {
		t.Fatalf("seq 1 should be flagged compressed: %+v", segs[1])
	}
	if segs[0].Compressed {
		t.Fatalf("seq 0 (raw) should not be flagged compressed: %+v", segs[0])
	}
	if segs[3].Size != int64(len("ten!!")) {
		t.Fatalf("seq 10 size = %d, want %d", segs[3].Size, len("ten!!"))
	}
}

func TestClosedSegments_RawWinsOverGzForSameSeq(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")

	// Both raw and ".gz" exist for seq 3 — the transient window during
	// rotation's compress step. They must collapse to a single segment,
	// preferring the raw copy.
	writeFile(t, SegmentPath(canonical, 3), []byte("raw-bytes"))
	writeFile(t, SegmentPath(canonical, 3)+GzSuffix, []byte("gz-bytes"))

	segs, err := ClosedSegments(canonical)
	if err != nil {
		t.Fatalf("ClosedSegments: %v", err)
	}
	if len(segs) != 1 {
		t.Fatalf("got %d segments, want 1 (collapsed): %+v", len(segs), segs)
	}
	if segs[0].Compressed {
		t.Fatalf("collapsed segment should be the raw copy: %+v", segs[0])
	}
	if segs[0].Path != SegmentPath(canonical, 3) {
		t.Fatalf("collapsed path = %q, want raw %q", segs[0].Path, SegmentPath(canonical, 3))
	}
}

func TestClosedSegments_MissingDir(t *testing.T) {
	segs, err := ClosedSegments(filepath.Join(t.TempDir(), "nope", "capture"))
	if err != nil {
		t.Fatalf("ClosedSegments on missing dir: %v", err)
	}
	if len(segs) != 0 {
		t.Fatalf("want no segments, got %d", len(segs))
	}
}

func TestNextSeq(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")

	seq, err := NextSeq(canonical)
	if err != nil {
		t.Fatalf("NextSeq empty: %v", err)
	}
	if seq != 0 {
		t.Fatalf("NextSeq with no segments = %d, want 0", seq)
	}

	writeFile(t, SegmentPath(canonical, 4), []byte("x"))
	seq, err = NextSeq(canonical)
	if err != nil {
		t.Fatalf("NextSeq: %v", err)
	}
	if seq != 5 {
		t.Fatalf("NextSeq after seq 4 = %d, want 5", seq)
	}
}

func TestOrderedPaths_LiveLast(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	writeFile(t, SegmentPath(canonical, 0), []byte("a"))
	writeFile(t, SegmentPath(canonical, 1), []byte("b"))
	writeFile(t, canonical, []byte("c"))

	paths, err := OrderedPaths(canonical)
	if err != nil {
		t.Fatalf("OrderedPaths: %v", err)
	}
	want := []string{
		SegmentPath(canonical, 0),
		SegmentPath(canonical, 1),
		canonical,
	}
	if len(paths) != len(want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("path %d = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestOrderedPaths_NoLive(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	writeFile(t, SegmentPath(canonical, 0), []byte("a"))

	paths, err := OrderedPaths(canonical)
	if err != nil {
		t.Fatalf("OrderedPaths: %v", err)
	}
	if len(paths) != 1 || paths[0] != SegmentPath(canonical, 0) {
		t.Fatalf("got %v, want only the closed segment", paths)
	}
}

func TestReadAll_ReassemblesInOrder(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	writeFile(t, SegmentPath(canonical, 0), []byte("AAAA"))
	writeFile(t, SegmentPath(canonical, 1), []byte("BBBB"))
	writeFile(t, canonical, []byte("CCCC"))

	data, err := ReadAll(canonical)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if want := []byte("AAAABBBBCCCC"); !bytes.Equal(data, want) {
		t.Fatalf("ReadAll = %q, want %q", data, want)
	}
}

func TestReadAll_NeverWritten(t *testing.T) {
	data, err := ReadAll(filepath.Join(t.TempDir(), "capture"))
	if err != nil {
		t.Fatalf("ReadAll on absent capture: %v", err)
	}
	if data != nil {
		t.Fatalf("want nil, got %q", data)
	}
}

func TestDump_ReassemblesInOrder(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	writeFile(t, SegmentPath(canonical, 0), []byte("hello "))
	writeFile(t, SegmentPath(canonical, 1), []byte("rotated "))
	writeFile(t, canonical, []byte("world"))

	var buf bytes.Buffer
	if err := Dump(canonical, &buf); err != nil {
		t.Fatalf("Dump: %v", err)
	}
	if want := "hello rotated world"; buf.String() != want {
		t.Fatalf("Dump = %q, want %q", buf.String(), want)
	}
}

func TestDump_NeverWritten(t *testing.T) {
	var buf bytes.Buffer
	if err := Dump(filepath.Join(t.TempDir(), "capture"), &buf); err != nil {
		t.Fatalf("Dump on absent capture: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("want no output, got %q", buf.String())
	}
}

func TestCompressSegment_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	raw := SegmentPath(canonical, 0)
	// Repetitive payload so gzip actually shrinks it — also exercises a
	// payload larger than one gzip block.
	original := bytes.Repeat([]byte("the quick brown fox jumped over\n"), 4096)
	writeFile(t, raw, original)

	gzPath, err := CompressSegment(raw)
	if err != nil {
		t.Fatalf("CompressSegment: %v", err)
	}
	if want := raw + GzSuffix; gzPath != want {
		t.Fatalf("gzPath = %q, want %q", gzPath, want)
	}
	// Raw is removed; only the compressed copy remains.
	if _, serr := os.Stat(raw); !os.IsNotExist(serr) {
		t.Fatalf("raw segment still present after compress: %v", serr)
	}
	info, err := os.Stat(gzPath)
	if err != nil {
		t.Fatalf("stat %q: %v", gzPath, err)
	}
	if info.Size() >= int64(len(original)) {
		t.Fatalf("compressed size %d not smaller than original %d", info.Size(), len(original))
	}
	// No in-flight temp file is left behind.
	if _, serr := os.Stat(gzPath + ".tmp"); !os.IsNotExist(serr) {
		t.Fatalf("temp file lingered: %v", serr)
	}

	// Round-trip: the compressed segment reads back as the original bytes.
	got, err := ReadAll(canonical)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("round-trip mismatch: got %d bytes, want %d", len(got), len(original))
	}
}

// TestReadSegment_ResolvesGzSiblingOnRawENOENT is the regression for issue
// #316: a segment listed by OrderedPaths as raw can be swapped to ".gz" by a
// concurrent CompressSegment in the window before the reader opens it. The old
// readSegment returned os.IsNotExist on the vanished raw path and ReadAll
// silently dropped the still-retained segment. readSegment must instead
// recover the ".gz" sibling and read it back as the original bytes.
func TestReadSegment_ResolvesGzSiblingOnRawENOENT(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	raw := SegmentPath(canonical, 0)
	original := bytes.Repeat([]byte("retained-history\n"), 512)
	writeFile(t, raw, original)
	if _, err := CompressSegment(raw); err != nil {
		t.Fatalf("CompressSegment: %v", err)
	}
	// Raw is gone; only raw+".gz" remains — exactly the post-race on-disk state
	// for the stale raw path OrderedPaths handed out.
	if _, serr := os.Stat(raw); !os.IsNotExist(serr) {
		t.Fatalf("raw segment unexpectedly present: %v", serr)
	}

	got, err := readSegment(raw)
	if err != nil {
		t.Fatalf("readSegment on raced raw path: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("recovered %d bytes, want %d", len(got), len(original))
	}
}

// TestDumpOne_ResolvesGzSiblingOnRawENOENT is the Dump-path twin of the above:
// dumpOne must also re-resolve the ".gz" sibling rather than skipping a
// retained segment that was compressed out from under it.
func TestDumpOne_ResolvesGzSiblingOnRawENOENT(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	raw := SegmentPath(canonical, 0)
	original := bytes.Repeat([]byte("retained-history\n"), 512)
	writeFile(t, raw, original)
	if _, err := CompressSegment(raw); err != nil {
		t.Fatalf("CompressSegment: %v", err)
	}

	var buf bytes.Buffer
	if err := dumpOne(raw, &buf); err != nil {
		t.Fatalf("dumpOne on raced raw path: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), original) {
		t.Fatalf("dumped %d bytes, want %d", buf.Len(), len(original))
	}
}

// TestReadSegment_ResolvesRawSiblingOnGzENOENT covers the inverse direction the
// fix is symmetric over: a ".gz" path that no longer exists falls back to the
// raw sibling (strip GzSuffix) before giving up.
func TestReadSegment_ResolvesRawSiblingOnGzENOENT(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	raw := SegmentPath(canonical, 0)
	original := []byte("raw-only-bytes")
	writeFile(t, raw, original)

	got, err := readSegment(raw + GzSuffix)
	if err != nil {
		t.Fatalf("readSegment on absent .gz with raw sibling: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("recovered %q, want %q", got, original)
	}
}

// TestReadSegment_BothGoneReturnsIsNotExist locks in the skip path: when
// neither the raw nor the ".gz" representation exists the segment is genuinely
// pruned/rotated, so readSegment must surface os.IsNotExist for ReadAll to skip
// it — the sibling retry must not mask a real prune as some other error.
func TestReadSegment_BothGoneReturnsIsNotExist(t *testing.T) {
	dir := t.TempDir()
	raw := SegmentPath(filepath.Join(dir, "capture"), 0)

	_, err := readSegment(raw)
	if !os.IsNotExist(err) {
		t.Fatalf("readSegment on fully-pruned segment: err = %v, want os.IsNotExist", err)
	}
}

func TestReadAll_SpansGzClosedAndRawLive(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")

	// Two closed segments compressed to ".gz", then a raw live segment —
	// the steady state after rotations have compressed older segments.
	writeFile(t, SegmentPath(canonical, 0), []byte("AAAA"))
	if _, err := CompressSegment(SegmentPath(canonical, 0)); err != nil {
		t.Fatalf("compress seq 0: %v", err)
	}
	writeFile(t, SegmentPath(canonical, 1), []byte("BBBB"))
	if _, err := CompressSegment(SegmentPath(canonical, 1)); err != nil {
		t.Fatalf("compress seq 1: %v", err)
	}
	writeFile(t, canonical, []byte("CCCC"))

	data, err := ReadAll(canonical)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if want := []byte("AAAABBBBCCCC"); !bytes.Equal(data, want) {
		t.Fatalf("ReadAll = %q, want %q", data, want)
	}

	// Dump must agree with ReadAll across the same mixed layout.
	var buf bytes.Buffer
	if derr := Dump(canonical, &buf); derr != nil {
		t.Fatalf("Dump: %v", derr)
	}
	if buf.String() != "AAAABBBBCCCC" {
		t.Fatalf("Dump = %q, want %q", buf.String(), "AAAABBBBCCCC")
	}
}
