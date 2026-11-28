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
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/pkg/api"
	pcapture "github.com/eminwux/sbsh/pkg/capture"
)

// castLine is one parsed asciicast line: a header or an event.
type castLine struct {
	header *pcapture.Header
	time   float64
	code   string
	data   string
}

// parseCastLines parses every line of an asciicast stream and fails the test
// unless each one is a header object or a three-element event array.
func parseCastLines(t *testing.T, data []byte) []castLine {
	t.Helper()
	if len(data) > 0 && data[len(data)-1] != '\n' {
		t.Fatalf("stream does not end with a newline: %q", data)
	}
	var out []castLine
	for i, line := range bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n")) {
		switch {
		case len(line) > 0 && line[0] == '{':
			var h pcapture.Header
			if err := json.Unmarshal(line, &h); err != nil || h.Version != pcapture.AsciicastVersion {
				t.Fatalf("line %d: not a header: %q (%v)", i, line, err)
			}
			out = append(out, castLine{header: &h})
		case len(line) > 0 && line[0] == '[':
			var rec []json.RawMessage
			if err := json.Unmarshal(line, &rec); err != nil || len(rec) != 3 {
				t.Fatalf("line %d: not a 3-element event: %q (%v)", i, line, err)
			}
			var cl castLine
			if json.Unmarshal(rec[0], &cl.time) != nil ||
				json.Unmarshal(rec[1], &cl.code) != nil ||
				json.Unmarshal(rec[2], &cl.data) != nil {
				t.Fatalf("line %d: malformed event fields: %q", i, line)
			}
			out = append(out, cl)
		default:
			t.Fatalf("line %d: neither header nor event: %q", i, line)
		}
	}
	return out
}

// assertRunsMonotonic fails unless t never decreases within each
// header-delimited run.
func assertRunsMonotonic(t *testing.T, lines []castLine) {
	t.Helper()
	last := -1.0
	for i, l := range lines {
		if l.header != nil {
			last = -1
			continue
		}
		if l.time < last {
			t.Fatalf("line %d: t went backwards within a run (%v after %v)", i, l.time, last)
		}
		last = l.time
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func events(lines []castLine, code string) []castLine {
	var out []castLine
	for _, l := range lines {
		if l.header == nil && l.code == code {
			out = append(out, l)
		}
	}
	return out
}

// A resize to a new size appends one "r" event; a resize to the current size
// appends nothing.
func TestCaptureWriter_AsciicastResizeEvent(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "capture")
	w := newTestAsciicastWriter(t, canonical, 1<<20, 8, 1<<30)

	mustWrite(t, w, []byte("before"))
	if err := w.Resize(120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if err := w.Resize(120, 40); err != nil { // same size: no event
		t.Fatalf("Resize same: %v", err)
	}
	if err := w.Resize(0, 40); err != nil { // non-positive: ignored
		t.Fatalf("Resize zero: %v", err)
	}
	mustWrite(t, w, []byte("after"))
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	lines := parseCastLines(t, readFile(t, canonical))
	rs := events(lines, "r")
	if len(rs) != 1 || rs[0].data != "120x40" {
		t.Fatalf("resize events = %+v, want one \"120x40\"", rs)
	}
	// Order: header, o before, r, o after.
	if len(lines) != 4 || lines[2].code != "r" {
		t.Fatalf("unexpected line order: %+v", lines)
	}
	if got := pcapture.DecodeAsciicast(readFile(t, canonical)); string(got) != "beforeafter" {
		t.Fatalf("decoded = %q, want %q", got, "beforeafter")
	}
}

// A raw capture is byte-for-byte unchanged by a resize.
func TestCaptureWriter_RawResizeUnchanged(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	resized := filepath.Join(dir, "resized")

	a := newTestCaptureWriter(t, plain, 1<<20, 8, 1<<30)
	mustWrite(t, a, []byte("abc"))
	mustWrite(t, a, []byte("def"))
	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	b := newTestCaptureWriter(t, resized, 1<<20, 8, 1<<30)
	mustWrite(t, b, []byte("abc"))
	if err := b.Resize(120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	mustWrite(t, b, []byte("def"))
	if err := b.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if got, want := readFile(t, resized), readFile(t, plain); !bytes.Equal(got, want) {
		t.Fatalf("raw capture changed by resize: %q, want %q", got, want)
	}
}

// A segment opened after a resize carries the size in effect in its header,
// so the recording stays correct once retention prunes the segment holding
// the "r" event.
func TestCaptureWriter_AsciicastHeaderAfterResizeSurvivesPrune(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "capture")
	// Every record rotates; byte retention is generous enough to keep all.
	w := newTestAsciicastWriter(t, canonical, 1, 16, 1<<20)

	if err := w.Resize(132, 50); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	mustWrite(t, w, []byte("X"))
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	segs, err := pcapture.ClosedSegments(canonical)
	if err != nil {
		t.Fatalf("ClosedSegments: %v", err)
	}
	if len(segs) < 2 {
		t.Fatalf("expected >=2 closed segments, got %d", len(segs))
	}
	// Segment 0 holds the launch header and the "r" event; prune it.
	if rmErr := os.Remove(segs[0].Path); rmErr != nil {
		t.Fatalf("prune: %v", rmErr)
	}
	data, err := pcapture.ReadAll(canonical)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	lines := parseCastLines(t, data)
	if len(events(lines, "r")) != 0 {
		t.Fatalf("expected the r event to be pruned: %q", data)
	}
	if lines[0].header == nil || lines[0].header.Width != 132 || lines[0].header.Height != 50 {
		t.Fatalf("oldest surviving header = %+v, want 132x50", lines[0].header)
	}
	if got := pcapture.DecodeAsciicast(data); string(got) != "X" {
		t.Fatalf("decoded = %q, want %q", got, "X")
	}
}

// Reopening a non-empty asciicast capture (a second run on a reused capture
// path) appends a header with the new start time before the first event, so t
// never decreases within each header-delimited run.
func TestCaptureWriter_AsciicastReopenWritesRunHeader(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "capture")

	w1 := newTestAsciicastWriter(t, canonical, 1<<20, 8, 1<<30)
	mustWrite(t, w1, []byte("A"))
	mustWrite(t, w1, []byte("B"))
	if err := w1.Close(); err != nil {
		t.Fatalf("close 1: %v", err)
	}

	w2 := newTestAsciicastWriter(t, canonical, 1<<20, 8, 1<<30)
	mustWrite(t, w2, []byte("C"))
	if err := w2.Close(); err != nil {
		t.Fatalf("close 2: %v", err)
	}

	lines := parseCastLines(t, readFile(t, canonical))
	// header, o A, o B, header, o C
	if len(lines) != 5 || lines[0].header == nil || lines[3].header == nil {
		t.Fatalf("unexpected run layout: %+v", lines)
	}
	if lines[3].header.Timestamp != w2.start.Unix() {
		t.Fatalf("run header timestamp = %d, want %d", lines[3].header.Timestamp, w2.start.Unix())
	}
	assertRunsMonotonic(t, lines)
	if got := pcapture.DecodeAsciicast(readFile(t, canonical)); string(got) != "ABC" {
		t.Fatalf("decoded = %q, want %q", got, "ABC")
	}
}

// A torn tail left by a run that died mid-line does not swallow the next
// run's header: a newline is written first, so the header stays its own line.
func TestCaptureWriter_AsciicastTornTailRunHeader(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "capture")

	w1 := newTestAsciicastWriter(t, canonical, 1<<20, 8, 1<<30)
	mustWrite(t, w1, []byte("A"))
	if err := w1.Close(); err != nil {
		t.Fatalf("close 1: %v", err)
	}
	f, err := os.OpenFile(canonical, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, werr := f.WriteString(`[9.5, "o", "torn`); werr != nil {
		t.Fatalf("append torn tail: %v", werr)
	}
	_ = f.Close()

	w2 := newTestAsciicastWriter(t, canonical, 1<<20, 8, 1<<30)
	mustWrite(t, w2, []byte("B"))
	if cerr := w2.Close(); cerr != nil {
		t.Fatalf("close 2: %v", cerr)
	}

	data := readFile(t, canonical)
	parts := bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	if len(parts) != 5 {
		t.Fatalf("lines = %d, want 5 (header, o, torn, header, o): %q", len(parts), data)
	}
	if string(parts[2]) != `[9.5, "o", "torn` {
		t.Fatalf("torn line = %q", parts[2])
	}
	// Everything but the torn line is a valid header/event, and the new run
	// starts with its own header.
	rest := bytes.Join([][]byte{parts[0], parts[1], parts[3], parts[4]}, []byte("\n"))
	lines := parseCastLines(t, append(rest, '\n'))
	if lines[2].header == nil {
		t.Fatalf("new run does not start with a header: %q", parts[3])
	}
	assertRunsMonotonic(t, lines)
	if got := pcapture.DecodeAsciicast(data); string(got) != "AB" {
		t.Fatalf("decoded = %q, want %q", got, "AB")
	}
}

// A multi-byte character split across two writes decodes to the original
// bytes with no U+FFFD, whichever way it is split.
func TestCaptureWriter_AsciicastUTF8Split(t *testing.T) {
	box := []byte("─") // E2 94 80
	for _, split := range []int{1, 2} {
		canonical := filepath.Join(t.TempDir(), "capture")
		w := newTestAsciicastWriter(t, canonical, 1<<20, 8, 1<<30)
		first := append([]byte("a"), box[:split]...)
		second := append(append([]byte(nil), box[split:]...), 'b')
		mustWrite(t, w, first)
		mustWrite(t, w, second)
		if err := w.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		data := readFile(t, canonical)
		got := pcapture.DecodeAsciicast(data)
		if want := []byte("a─b"); !bytes.Equal(got, want) {
			t.Fatalf("split %d: decoded = %q, want %q", split, got, want)
		}
		if bytes.Contains(got, []byte("�")) {
			t.Fatalf("split %d: decoded contains U+FFFD: %q", split, got)
		}
		parseCastLines(t, data)
	}
}

// A write consisting only of a partial character produces no event until the
// character completes; a sequence still incomplete at Close is written out.
func TestCaptureWriter_AsciicastUTF8HeldAndFlushedOnClose(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "capture")
	w := newTestAsciicastWriter(t, canonical, 1<<20, 8, 1<<30)

	mustWrite(t, w, []byte{0xE2}) // lead byte only: held, no event yet
	lines := parseCastLines(t, readFile(t, canonical))
	if n := len(events(lines, "o")); n != 0 {
		t.Fatalf("events after a lone lead byte = %d, want 0", n)
	}
	mustWrite(t, w, []byte{0x94}) // still incomplete
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	lines = parseCastLines(t, readFile(t, canonical))
	outs := events(lines, "o")
	if len(outs) != 1 {
		t.Fatalf("events after Close = %d, want 1 (the flushed partial)", len(outs))
	}
	// The partial is invalid UTF-8 on its own, so it is encoded as today.
	if outs[0].data == "" {
		t.Fatalf("flushed event is empty")
	}
}

// Bytes invalid for another reason (a stray continuation byte) are not held
// back: they are written in the same Write, as before.
func TestCaptureWriter_AsciicastInvalidUTF8NotHeld(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "capture")
	w := newTestAsciicastWriter(t, canonical, 1<<20, 8, 1<<30)
	mustWrite(t, w, []byte{'x', 0x94})
	lines := parseCastLines(t, readFile(t, canonical))
	if n := len(events(lines, "o")); n != 1 {
		t.Fatalf("events = %d, want 1", n)
	}
	if len(w.carry) != 0 {
		t.Fatalf("carry = %x, want empty", w.carry)
	}
	_ = w.Close()
}

func TestIncompleteUTF8Tail(t *testing.T) {
	cases := []struct {
		in   []byte
		want int
	}{
		{nil, 0},
		{[]byte("abc"), 3},
		{[]byte("a─"), 4},
		{[]byte{'a', 0xE2}, 1},
		{[]byte{'a', 0xE2, 0x94}, 1},
		{[]byte{'a', 0xF0, 0x9F, 0x98}, 1},       // 3 of 4 bytes
		{[]byte{0xF0, 0x9F, 0x98, 0x80}, 4},      // complete 4-byte rune
		{[]byte{'a', 0x94}, 2},                   // stray continuation: invalid, not held
		{[]byte{'a', 0x94, 0x94, 0x94, 0x94}, 5}, // no rune start in the last 3 bytes
	}
	for _, c := range cases {
		if got := incompleteUTF8Tail(c.in); got != c.want {
			t.Errorf("incompleteUTF8Tail(%x) = %d, want %d", c.in, got, c.want)
		}
	}
}

// With resizes, several runs and rotation, every line of the reassembled
// capture is a header or a three-element event, t never decreases within a
// run, and Replay (behind `sb read` and --full-capture attach) returns the
// concatenated output across the rotation boundaries.
func TestCaptureWriter_AsciicastRunsResizesRotation(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "capture")
	const maxBytes = 96

	w1 := newTestAsciicastWriter(t, canonical, maxBytes, 64, 1<<30)
	mustWrite(t, w1, []byte("one "))
	if err := w1.Resize(100, 30); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	mustWrite(t, w1, []byte("two "))
	mustWrite(t, w1, []byte("thr\xe2")) // split "─"
	mustWrite(t, w1, []byte("\x94\x80 "))
	if err := w1.Close(); err != nil {
		t.Fatalf("close 1: %v", err)
	}

	w2 := newTestAsciicastWriter(t, canonical, maxBytes, 64, 1<<30)
	mustWrite(t, w2, []byte("four "))
	if err := w2.Resize(90, 20); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	mustWrite(t, w2, []byte("five"))
	if err := w2.Close(); err != nil {
		t.Fatalf("close 2: %v", err)
	}

	segs, err := pcapture.ClosedSegments(canonical)
	if err != nil {
		t.Fatalf("ClosedSegments: %v", err)
	}
	if len(segs) == 0 {
		t.Fatalf("expected rotation")
	}
	data, err := pcapture.ReadAll(canonical)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	lines := parseCastLines(t, data)
	assertRunsMonotonic(t, lines)
	if n := len(events(lines, "r")); n != 2 {
		t.Fatalf("resize events = %d, want 2", n)
	}
	got, err := pcapture.Replay(canonical)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if want := "one two thr─ four five"; string(got) != want {
		t.Fatalf("Replay = %q, want %q", got, want)
	}
}

// Exec.Resize records the resize in an asciicast capture.
func TestExecResizeRecordsAsciicastEvent(t *testing.T) {
	spec := newLiveSpec(t, "/bin/cat")
	spec.CaptureFormat = api.CaptureFormatAsciicast

	sr := NewTerminalRunnerExec(context.Background(), newDiscardLogger(), spec).(*Exec)
	if err := sr.CreateMetadata(); err != nil {
		t.Fatalf("CreateMetadata: %v", err)
	}
	evCh := make(chan Event, 32)
	if err := sr.StartTerminal(evCh); err != nil {
		t.Fatalf("StartTerminal: %v", err)
	}
	sr.Resize(api.ResizeArgs{Cols: 120, Rows: 40})
	if err := sr.Close(nil); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := pcapture.ReadAll(spec.CaptureFile)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	rs := events(parseCastLines(t, data), "r")
	if len(rs) != 1 || rs[0].data != "120x40" {
		t.Fatalf("resize events = %+v, want one \"120x40\"", rs)
	}
}
