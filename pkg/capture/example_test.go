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

package capture_test

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/pkg/capture"
)

// These tests use only the exported API, the way a module outside sbsh sees
// it.

const (
	hdr80  = `{"version":2,"width":80,"height":24,"timestamp":1700000000}` + "\n"
	hdr100 = `{"version":2,"width":100,"height":30,"timestamp":1700000000}` + "\n"
)

func mustWrite(t testing.TB, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustGzip(t testing.TB, path, data string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(data)); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

// buildCapture lays out a capture directory with one raw closed segment, one
// gzip closed segment and a live segment, as a rotating writer leaves it.
func buildCapture(t testing.TB) string {
	t.Helper()
	canonical := filepath.Join(t.TempDir(), "capture.cast")
	mustWrite(t, capture.SegmentPath(canonical, 0), hdr80+`[0.1,"o","one "]`+"\n"+`[0.2,"r","100x30"]`+"\n")
	mustGzip(t, capture.SegmentPath(canonical, 1)+capture.GzSuffix, hdr100+`[0.3,"o","two "]`+"\n")
	mustWrite(t, canonical, hdr100+`[0.4,"o","three"]`+"\n")
	return canonical
}

// readSegments opens every listed segment decompressed and concatenates them.
func readSegments(t *testing.T, segs []capture.Segment) []byte {
	t.Helper()
	var all []byte
	for _, s := range segs {
		r, err := capture.Open(s)
		if err != nil {
			t.Fatalf("Open(%d): %v", s.Seq, err)
		}
		b, rerr := io.ReadAll(r)
		_ = r.Close()
		if rerr != nil {
			t.Fatalf("read %d: %v", s.Seq, rerr)
		}
		all = append(all, b...)
	}
	return all
}

func TestExternal_List(t *testing.T) {
	canonical := buildCapture(t)

	segs, err := capture.Segments(canonical)
	if err != nil {
		t.Fatalf("Segments: %v", err)
	}
	if len(segs) != 3 {
		t.Fatalf("segments = %+v, want 3", segs)
	}
	if segs[0].Seq != 0 || segs[0].Compressed || segs[0].Live {
		t.Fatalf("segment 0 = %+v", segs[0])
	}
	if segs[1].Seq != 1 || !segs[1].Compressed || segs[1].Path != capture.SegmentPath(canonical, 1)+capture.GzSuffix {
		t.Fatalf("segment 1 = %+v", segs[1])
	}
	if !segs[2].Live || segs[2].Path != canonical || segs[2].Seq != 2 {
		t.Fatalf("live segment = %+v, want Seq 2 (NextSeq)", segs[2])
	}
	if next, _ := capture.NextSeq(canonical); next != segs[2].Seq {
		t.Fatalf("NextSeq = %d, live Seq = %d", next, segs[2].Seq)
	}
	for _, s := range segs {
		info, serr := os.Stat(s.Path)
		if serr != nil || info.Size() != s.Size {
			t.Fatalf("segment %d Size = %d, on disk %v (%v)", s.Seq, s.Size, info, serr)
		}
	}
}

func TestExternal_OpenAndDecode(t *testing.T) {
	canonical := buildCapture(t)
	segs, err := capture.Segments(canonical)
	if err != nil {
		t.Fatalf("Segments: %v", err)
	}

	// Stored form: the gzip segment is served as stored.
	f, compressed, err := capture.OpenStored(segs[1])
	if err != nil || !compressed {
		t.Fatalf("OpenStored(gz) = %v, %v", compressed, err)
	}
	stored, _ := io.ReadAll(f)
	_ = f.Close()
	if len(stored) < 2 || stored[0] != 0x1f || stored[1] != 0x8b {
		t.Fatalf("OpenStored did not return gzip bytes")
	}

	// Decompressed form, decoded.
	all := readSegments(t, segs)
	if capture.Sniff(all) != "asciicast" {
		t.Fatalf("Sniff = %q", capture.Sniff(all))
	}
	if got := string(capture.DecodeAsciicast(all)); got != "one two three" {
		t.Fatalf("DecodeAsciicast = %q", got)
	}
	if got, _ := capture.Replay(canonical); string(got) != "one two three" {
		t.Fatalf("Replay = %q", got)
	}
}

// A segment renamed between listing and open (raw compressed to .gz, or the
// reverse) is still opened, under the name it now has.
func TestExternal_RenameBetweenListAndOpen(t *testing.T) {
	canonical := buildCapture(t)
	segs, err := capture.Segments(canonical)
	if err != nil {
		t.Fatalf("Segments: %v", err)
	}

	// raw -> .gz for segment 0.
	raw0, _ := os.ReadFile(segs[0].Path)
	mustGzip(t, segs[0].Path+capture.GzSuffix, string(raw0))
	if rerr := os.Remove(segs[0].Path); rerr != nil {
		t.Fatal(rerr)
	}
	// .gz -> raw for segment 1 (the transient state where raw wins).
	if rerr := os.Remove(segs[1].Path); rerr != nil {
		t.Fatal(rerr)
	}
	mustWrite(t, capture.SegmentPath(canonical, 1), hdr100+`[0.3,"o","two "]`+"\n")

	f, compressed, err := capture.OpenStored(segs[0])
	if err != nil || !compressed {
		t.Fatalf("OpenStored(seg0) = %v, %v; want compressed", compressed, err)
	}
	_ = f.Close()
	f, compressed, err = capture.OpenStored(segs[1])
	if err != nil || compressed {
		t.Fatalf("OpenStored(seg1) = %v, %v; want raw", compressed, err)
	}
	_ = f.Close()

	all := readSegments(t, segs)
	if got := string(capture.DecodeAsciicast(all)); got != "one two three" {
		t.Fatalf("decoded after renames = %q", got)
	}
}

// On a torn tail the decoder stops before the partial line, and Offset is
// where a reader resumes once the line is complete.
func TestExternal_DecoderOffsetTornTail(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "capture.cast")
	complete := hdr80 + `[0.1,"o","a"]` + "\n"
	mustWrite(t, canonical, complete+`[0.2,"o","b`)

	segs, err := capture.Segments(canonical)
	if err != nil || len(segs) != 1 {
		t.Fatalf("Segments = %+v, %v", segs, err)
	}
	r, err := capture.Open(segs[0])
	if err != nil {
		t.Fatal(err)
	}
	d := capture.NewDecoder(r)
	n := 0
	for {
		_, nerr := d.Next()
		if errors.Is(nerr, io.EOF) {
			break
		}
		if nerr != nil {
			t.Fatal(nerr)
		}
		n++
	}
	_ = r.Close()
	if n != 2 || d.Offset() != int64(len(complete)) {
		t.Fatalf("records = %d, Offset = %d; want 2, %d", n, d.Offset(), len(complete))
	}

	// The line completes; resume from Offset.
	fa, _ := os.OpenFile(canonical, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = fa.WriteString(`"]` + "\n")
	_ = fa.Close()
	f, _, err := capture.OpenStored(segs[0])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, serr := f.Seek(d.Offset(), io.SeekStart); serr != nil {
		t.Fatal(serr)
	}
	rec, err := capture.NewDecoder(f).Next()
	if err != nil || rec.Event == nil || rec.Event.Data != "b" {
		t.Fatalf("resumed record = %+v, %v", rec, err)
	}
}

// Example lists a capture's segments and decodes its output.
func Example() {
	dir, _ := os.MkdirTemp("", "capture-example")
	defer func() { _ = os.RemoveAll(dir) }()
	canonical := filepath.Join(dir, "capture.cast")
	_ = os.WriteFile(capture.SegmentPath(canonical, 0), []byte(hdr80+`[0.1,"o","hello "]`+"\n"), 0o600)
	_ = os.WriteFile(canonical, []byte(hdr80+`[0.2,"o","world"]`+"\n"), 0o600)

	segs, _ := capture.Segments(canonical)
	for _, s := range segs {
		fmt.Println(filepath.Base(s.Path), s.Seq, s.Live)
	}
	out, _ := capture.Replay(canonical)
	fmt.Println(string(out))
	// Output:
	// capture.cast.000000 0 false
	// capture.cast 1 true
	// hello world
}
