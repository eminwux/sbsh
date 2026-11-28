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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	pcapture "github.com/eminwux/sbsh/pkg/capture"
)

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

// A header line plus a couple of output events round-trips: pcapture.IsAsciicast sniffs
// true, pcapture.DecodeAsciicast yields the concatenated payloads, and the header line
// itself is valid asciicast v2 JSON.
func TestAsciicast_HeaderAndEvents(t *testing.T) {
	hdr, err := EncodeHeader(80, 24, 1700000000)
	if err != nil {
		t.Fatalf("EncodeHeader: %v", err)
	}
	if !bytes.HasSuffix(hdr, []byte("\n")) {
		t.Fatalf("header not newline-terminated: %q", hdr)
	}
	var h pcapture.Header
	if uerr := json.Unmarshal(bytes.TrimSpace(hdr), &h); uerr != nil {
		t.Fatalf("header not valid JSON: %v", uerr)
	}
	if h.Version != pcapture.AsciicastVersion || h.Width != 80 || h.Height != 24 || h.Timestamp != 1700000000 {
		t.Fatalf("header fields = %+v", h)
	}

	var stream []byte
	stream = append(stream, hdr...)
	for _, payload := range [][]byte{[]byte("hello "), []byte("world\r\n")} {
		ev, eerr := EncodeOutputEvent(0.5, payload)
		if eerr != nil {
			t.Fatalf("EncodeOutputEvent: %v", eerr)
		}
		stream = append(stream, ev...)
	}

	if !pcapture.IsAsciicast(stream) {
		t.Fatalf("pcapture.IsAsciicast(asciicast stream) = false")
	}
	if got, want := pcapture.DecodeAsciicast(stream), []byte("hello world\r\n"); !bytes.Equal(got, want) {
		t.Fatalf("pcapture.DecodeAsciicast = %q, want %q", got, want)
	}
}

// EncodeOutputEvent escapes arbitrary bytes through a JSON string and the
// decoder restores valid-UTF-8 payloads exactly (control bytes, quotes,
// backslashes). This is the round-trip the writer/reader pair relies on.
func TestAsciicast_PayloadEscaping(t *testing.T) {
	payload := []byte("a\tb\"c\\d\x1b[0m\n")
	ev, err := EncodeOutputEvent(1.25, payload)
	if err != nil {
		t.Fatalf("EncodeOutputEvent: %v", err)
	}
	// The encoded line must itself be valid JSON.
	var rec []json.RawMessage
	if uerr := json.Unmarshal(bytes.TrimSpace(ev), &rec); uerr != nil {
		t.Fatalf("event not valid JSON: %v", uerr)
	}
	if got := pcapture.DecodeAsciicast(ev); !bytes.Equal(got, payload) {
		t.Fatalf("pcapture.DecodeAsciicast = %q, want %q", got, payload)
	}
}

// Multiple header lines (one per segment in a reassembled multi-segment
// capture) and non-output event codes are skipped; only "o" payloads survive.
func TestAsciicast_MultiHeaderAndForeignEvents(t *testing.T) {
	hdr, _ := EncodeHeader(80, 24, 1)
	out1, _ := EncodeOutputEvent(0.1, []byte("AAA"))
	out2, _ := EncodeOutputEvent(0.2, []byte("BBB"))

	var stream []byte
	stream = append(stream, hdr...)                                       // segment 0 header
	stream = append(stream, out1...)                                      //
	stream = append(stream, []byte(`[0.15,"i","ignored input"]`+"\n")...) // foreign event
	stream = append(stream, hdr...)                                       // segment 1 header (reassembled)
	stream = append(stream, out2...)                                      //
	stream = append(stream, []byte("\n")...)                              // blank line tolerated

	if got, want := pcapture.DecodeAsciicast(stream), []byte("AAABBB"); !bytes.Equal(got, want) {
		t.Fatalf("pcapture.DecodeAsciicast = %q, want %q", got, want)
	}
}

// A torn final event line (e.g. a half-flushed live segment) is skipped rather
// than failing the decode, so a replay never denies an attach over a partial
// tail.
func TestAsciicast_TornTailSkipped(t *testing.T) {
	hdr, _ := EncodeHeader(80, 24, 1)
	good, _ := EncodeOutputEvent(0.1, []byte("ok"))
	var stream []byte
	stream = append(stream, hdr...)
	stream = append(stream, good...)
	stream = append(stream, []byte(`[0.2,"o","truncat`)...) // no closing/newline

	if got, want := pcapture.DecodeAsciicast(stream), []byte("ok"); !bytes.Equal(got, want) {
		t.Fatalf("pcapture.DecodeAsciicast = %q, want %q", got, want)
	}
}

// pcapture.Replay over on-disk segments: an asciicast capture rotated into a closed
// segment + live segment reassembles and decodes to the full output, and a raw
// capture passes through unchanged. Mirrors how the readers call pcapture.Replay.
func TestReplay_AsciicastAndRaw(t *testing.T) {
	t.Run("asciicast across rotation", func(t *testing.T) {
		dir := t.TempDir()
		canonical := filepath.Join(dir, "capture")
		hdr, _ := EncodeHeader(80, 24, 1)
		ev0, _ := EncodeOutputEvent(0.1, []byte("seg0-"))
		ev1, _ := EncodeOutputEvent(0.2, []byte("live"))
		// Closed segment 0: its own header + an event (self-describing).
		writeFile(t, pcapture.SegmentPath(canonical, 0), append(append([]byte{}, hdr...), ev0...))
		// Live segment: header + an event.
		writeFile(t, canonical, append(append([]byte{}, hdr...), ev1...))

		got, err := pcapture.Replay(canonical)
		if err != nil {
			t.Fatalf("pcapture.Replay: %v", err)
		}
		if want := []byte("seg0-live"); !bytes.Equal(got, want) {
			t.Fatalf("pcapture.Replay = %q, want %q", got, want)
		}
	})

	t.Run("raw passthrough", func(t *testing.T) {
		dir := t.TempDir()
		canonical := filepath.Join(dir, "capture")
		writeFile(t, pcapture.SegmentPath(canonical, 0), []byte("closed-"))
		writeFile(t, canonical, []byte("live"))
		got, err := pcapture.Replay(canonical)
		if err != nil {
			t.Fatalf("pcapture.Replay: %v", err)
		}
		if want := []byte("closed-live"); !bytes.Equal(got, want) {
			t.Fatalf("pcapture.Replay = %q, want %q", got, want)
		}
	})
}

// After segment 0 is pruned by retention, the reassembled asciicast stream is
// still valid: the surviving oldest segment carries its own header, so
// pcapture.IsAsciicast stays true and decoding still works. This is the load-bearing
// header-survival criterion.
func TestReplay_HeaderSurvivesPruning(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "capture")
	hdr, _ := EncodeHeader(80, 24, 1)
	ev1, _ := EncodeOutputEvent(0.2, []byte("kept1-"))
	evLive, _ := EncodeOutputEvent(0.3, []byte("live"))
	// Segment 0 has been pruned; segment 1 (now oldest) keeps its own header.
	writeFile(t, pcapture.SegmentPath(canonical, 1), append(append([]byte{}, hdr...), ev1...))
	writeFile(t, canonical, append(append([]byte{}, hdr...), evLive...))

	data, err := pcapture.ReadAll(canonical)
	if err != nil {
		t.Fatalf("pcapture.ReadAll: %v", err)
	}
	if !pcapture.IsAsciicast(data) {
		t.Fatalf("reassembled stream not sniffed as asciicast after segment 0 pruned")
	}
	got, err := pcapture.Replay(canonical)
	if err != nil {
		t.Fatalf("pcapture.Replay: %v", err)
	}
	if want := []byte("kept1-live"); !bytes.Equal(got, want) {
		t.Fatalf("pcapture.Replay = %q, want %q", got, want)
	}
}

// A resize event is a three-element `[t, "r", "<cols>x<rows>"]` line that the
// decoder skips (it carries no output bytes).
func TestAsciicast_ResizeEvent(t *testing.T) {
	ev, err := EncodeResizeEvent(1.5, 120, 40)
	if err != nil {
		t.Fatalf("EncodeResizeEvent: %v", err)
	}
	if got, want := string(ev), "[1.5,\"r\",\"120x40\"]\n"; got != want {
		t.Fatalf("EncodeResizeEvent = %q, want %q", got, want)
	}
	hdr, _ := EncodeHeader(80, 24, 0)
	out, _ := EncodeOutputEvent(2, []byte("hi"))
	stream := append(append(hdr, ev...), out...)
	if got := pcapture.DecodeAsciicast(stream); string(got) != "hi" {
		t.Fatalf("decoded = %q, want %q", got, "hi")
	}
}
