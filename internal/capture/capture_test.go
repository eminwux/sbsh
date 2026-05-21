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
	// Live segment — must not appear among closed segments.
	writeFile(t, canonical, []byte("live"))
	// Unrelated siblings that must be ignored: non-digit suffix and a
	// future compressed segment.
	writeFile(t, canonical+".notes", []byte("x"))
	writeFile(t, SegmentPath(canonical, 1)+".gz", []byte("x"))

	segs, err := ClosedSegments(canonical)
	if err != nil {
		t.Fatalf("ClosedSegments: %v", err)
	}
	if len(segs) != 3 {
		t.Fatalf("got %d closed segments, want 3: %+v", len(segs), segs)
	}
	wantSeq := []uint64{0, 2, 10}
	for i, s := range segs {
		if s.Seq != wantSeq[i] {
			t.Fatalf("segment %d seq = %d, want %d", i, s.Seq, wantSeq[i])
		}
	}
	if segs[2].Size != int64(len("ten!!")) {
		t.Fatalf("seq 10 size = %d, want %d", segs[2].Size, len("ten!!"))
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
