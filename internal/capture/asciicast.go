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

// asciicast v2 support for the capture format selector (see
// pkg/api.CaptureFormat). The on-disk shape is the asciicast v2 stream:
// https://docs.asciinema.org/manual/asciicast/v2/ — a JSON header object on
// the first line, then one event line `[t, code, data]` per record, where t is
// the absolute offset in seconds from session start.
//
// The capture writer emits exactly one output ("o") event per Write call, so a
// record line is never bisected by segment rotation (which lands on write
// boundaries). To keep every segment self-describing — and so a reassembled
// capture stays decodable after retention prunes segment 0 — the writer
// re-emits the header at the top of each segment. DecodeAsciicast therefore
// tolerates a header line anywhere in the stream, not just at the very top.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
)

// AsciicastVersion is the asciicast format version this package reads/writes.
const AsciicastVersion = 2

// eventOutput is the asciicast event code for terminal output. Other codes
// (input "i", resize "r", marker "m") are not emitted by the writer and are
// skipped by the decoder.
const eventOutput = "o"

// maxEventLine bounds a single scanned record line. The capture writer emits
// one event per PTY read (currently 8 KiB), and a JSON-escaped payload at most
// quadruples that, so 1 MiB is comfortably above any real line while still
// guarding the decoder against an unbounded allocation on a corrupt segment.
const maxEventLine = 1 << 20

// scanBufInit is the initial (pre-grow) decoder scan buffer; bufio.Scanner
// grows it up to maxEventLine as needed.
const scanBufInit = 64 * 1024

// Header is the asciicast v2 header — the first line of the stream.
type Header struct {
	Version   int   `json:"version"`
	Width     int   `json:"width"`
	Height    int   `json:"height"`
	Timestamp int64 `json:"timestamp,omitempty"`
}

// EncodeHeader marshals an asciicast v2 header line (newline-terminated).
// width/height record the terminal's initial geometry; timestamp is the unix
// epoch (seconds) of session start, used by players to label the recording.
func EncodeHeader(width, height int, timestamp int64) ([]byte, error) {
	b, err := json.Marshal(Header{
		Version:   AsciicastVersion,
		Width:     width,
		Height:    height,
		Timestamp: timestamp,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal asciicast header: %w", err)
	}
	return append(b, '\n'), nil
}

// EncodeOutputEvent marshals one asciicast v2 output record line:
// `[<elapsed-seconds>, "o", <data>]\n`. data is wrapped in a JSON string, so
// arbitrary bytes are escaped (and non-UTF-8 bytes coerced by the encoder —
// raw is the byte-exact format; asciicast trades that for replay/portability).
func EncodeOutputEvent(elapsed float64, data []byte) ([]byte, error) {
	b, err := json.Marshal([]any{elapsed, eventOutput, string(data)})
	if err != nil {
		return nil, fmt.Errorf("marshal asciicast event: %w", err)
	}
	return append(b, '\n'), nil
}

// IsAsciicast reports whether data begins with an asciicast v2 header line —
// the format sniff readers use to decide between decode and raw passthrough.
// It inspects only the first line, so it is cheap on a large reassembled
// transcript.
func IsAsciicast(data []byte) bool {
	line := data
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		line = data[:i]
	}
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return false
	}
	var h Header
	if err := json.Unmarshal(line, &h); err != nil {
		return false
	}
	return h.Version == AsciicastVersion
}

// DecodeAsciicast extracts the concatenated terminal-output bytes from an
// asciicast v2 stream: it skips header lines (one per segment in a reassembled
// multi-segment capture) and decodes each `[t, "o", data]` record to its data
// payload, dropping non-"o" events. A malformed line is skipped rather than
// failing the whole replay, so a torn tail (e.g. a half-written live segment)
// never denies an attach — it just stops contributing bytes.
func DecodeAsciicast(data []byte) []byte {
	var out []byte
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, scanBufInit), maxEventLine)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] != '[' {
			continue // header object, blank line, or non-record content
		}
		var rec []json.RawMessage
		if err := json.Unmarshal(line, &rec); err != nil || len(rec) < 3 {
			continue
		}
		var code string
		if err := json.Unmarshal(rec[1], &code); err != nil || code != eventOutput {
			continue
		}
		var payload string
		if err := json.Unmarshal(rec[2], &payload); err != nil {
			continue
		}
		out = append(out, payload...)
	}
	return out
}

// Replay reassembles the full transcript at canonical and returns the bytes a
// reader should blast to a client PTY: raw captures pass through byte-for-byte;
// asciicast captures are decoded to their output bytes. It is the single
// sniff-and-decode entry point shared by `--full-capture` attach and `sb read`
// so both readers agree on format handling.
func Replay(canonical string) ([]byte, error) {
	data, err := ReadAll(canonical)
	if err != nil {
		return nil, err
	}
	if IsAsciicast(data) {
		return DecodeAsciicast(data), nil
	}
	return data, nil
}
