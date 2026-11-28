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
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/eminwux/sbsh/pkg/api"
)

// AsciicastVersion is the asciicast format version sbsh writes and reads.
const AsciicastVersion = 2

// Asciicast event codes. sbsh writes EventOutput and EventResize; a decoder
// returns any code it finds, and DecodeAsciicast keeps only EventOutput.
const (
	// EventOutput is terminal output; Data is the output text.
	EventOutput = "o"
	// EventResize is a terminal resize; Data is "<cols>x<rows>".
	EventResize = "r"
)

// maxEventLine bounds a single record line. The writer emits one event per
// PTY read (currently 8 KiB), and a JSON-escaped payload at most quadruples
// that, so 1 MiB is comfortably above any real line while still guarding the
// decoder against an unbounded allocation on a corrupt segment. A longer line
// is skipped.
const maxEventLine = 1 << 20

// Header is an asciicast v2 header line. Width and Height are the terminal
// size in effect when the line was written; Timestamp is the unix time
// (seconds) the run started.
type Header struct {
	Version   int   `json:"version"`
	Width     int   `json:"width"`
	Height    int   `json:"height"`
	Timestamp int64 `json:"timestamp,omitempty"`
}

// Event is one asciicast v2 event line `[time, code, data]`. Time is seconds
// since the start of the run, i.e. since the Timestamp of the nearest
// preceding Header.
type Event struct {
	Time float64
	Code string
	Data string
}

// Record is one decoded line: exactly one of Header and Event is set.
type Record struct {
	Header *Header
	Event  *Event
}

// Decoder reads asciicast records from a stream one whole line at a time.
// Blank and malformed lines are skipped. A final line without a terminating
// newline (a torn tail, or a line still being written) is not returned and
// not counted by Offset, so a caller can resume from Offset once it is
// complete.
type Decoder struct {
	r   *bufio.Reader
	off int64
	buf []byte
}

// NewDecoder returns a Decoder reading from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

// Next returns the next header or event. It returns io.EOF at the end of the
// stream, including when only an unterminated final line remains.
func (d *Decoder) Next() (Record, error) {
	for {
		line, err := d.readLine()
		if err != nil {
			return Record{}, err
		}
		if rec, ok := parseLine(line); ok {
			return rec, nil
		}
	}
}

// Offset returns the number of bytes consumed through the end of the last
// complete line read, i.e. the offset of the first byte not yet decoded.
func (d *Decoder) Offset() int64 {
	return d.off
}

// readLine returns the next newline-terminated line. A line over maxEventLine
// is returned as nil (and skipped by the caller) but still consumed.
func (d *Decoder) readLine() ([]byte, error) {
	d.buf = d.buf[:0]
	var n int64
	tooLong := false
	for {
		chunk, err := d.r.ReadSlice('\n')
		n += int64(len(chunk))
		if !tooLong {
			if len(d.buf)+len(chunk) > maxEventLine {
				tooLong = true
				d.buf = d.buf[:0]
			} else {
				d.buf = append(d.buf, chunk...)
			}
		}
		switch {
		case err == nil:
			d.off += n
			if tooLong {
				return nil, nil
			}
			return d.buf, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		default:
			// io.EOF with a partial line, or a read error: the partial
			// line is not consumed.
			return nil, err
		}
	}
}

// parseLine decodes one line into a header or event record.
func parseLine(line []byte) (Record, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return Record{}, false
	}
	switch line[0] {
	case '{':
		var h Header
		if err := json.Unmarshal(line, &h); err != nil || h.Version != AsciicastVersion {
			return Record{}, false
		}
		return Record{Header: &h}, true
	case '[':
		var rec []json.RawMessage
		if err := json.Unmarshal(line, &rec); err != nil || len(rec) < 3 {
			return Record{}, false
		}
		var ev Event
		if json.Unmarshal(rec[0], &ev.Time) != nil ||
			json.Unmarshal(rec[1], &ev.Code) != nil ||
			json.Unmarshal(rec[2], &ev.Data) != nil {
			return Record{}, false
		}
		return Record{Event: &ev}, true
	}
	return Record{}, false
}

// Sniff reports the capture format of a stream from its first line:
// api.CaptureFormatAsciicast when it is an asciicast v2 header, otherwise
// api.CaptureFormatRaw. data may be the first line or any prefix of the
// stream that contains it.
func Sniff(data []byte) string {
	if IsAsciicast(data) {
		return api.CaptureFormatAsciicast
	}
	return api.CaptureFormatRaw
}

// IsAsciicast reports whether data begins with an asciicast v2 header line.
// It inspects only the first line, so it is cheap on a large reassembled
// capture.
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

// DecodeAsciicast returns the concatenated output bytes of an asciicast
// stream: the Data of every EventOutput event, in order. Headers (one per
// segment and per run), other event codes, malformed lines and an
// unterminated final line are skipped, so a torn tail never fails a replay.
func DecodeAsciicast(data []byte) []byte {
	var out []byte
	d := NewDecoder(bytes.NewReader(data))
	for {
		rec, err := d.Next()
		if err != nil {
			return out
		}
		if rec.Event != nil && rec.Event.Code == EventOutput {
			out = append(out, rec.Event.Data...)
		}
	}
}

// Replay reassembles the capture at canonical and returns its output bytes:
// a raw capture byte-for-byte, an asciicast capture decoded with
// DecodeAsciicast. The format is sniffed from the first line of the
// reassembled stream. `sb read` and attach --full-capture both use it.
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
