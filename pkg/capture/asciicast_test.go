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
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/pkg/api"
)

// Raw bytes (no header) are not sniffed as asciicast; Sniff reports them
// as raw.
func TestAsciicast_RawNotSniffed(t *testing.T) {
	raw := []byte("$ ls -la\r\ntotal 0\r\n")
	if IsAsciicast(raw) {
		t.Fatalf("IsAsciicast(raw bytes) = true")
	}
	// A line that merely starts with '{' but is not a v2 header is still raw.
	if IsAsciicast([]byte("{not json\n")) {
		t.Fatalf("IsAsciicast(non-header brace) = true")
	}
	if IsAsciicast([]byte(`{"version":1}` + "\n")) {
		t.Fatalf("IsAsciicast(v1 header) = true")
	}
}

func TestSniff(t *testing.T) {
	if got := Sniff([]byte(`{"version":2,"width":80,"height":24}` + "\n[0.1,\"o\",\"x\"]\n")); got != api.CaptureFormatAsciicast {
		t.Fatalf("Sniff(asciicast) = %q", got)
	}
	for _, in := range [][]byte{nil, []byte("$ ls\r\n"), []byte(`{"version":1}`)} {
		if got := Sniff(in); got != api.CaptureFormatRaw {
			t.Fatalf("Sniff(%q) = %q, want raw", in, got)
		}
	}
}

// The decoder returns headers and events in order, skips blank, malformed and
// over-long lines, and stops before an unterminated final line; Offset is the
// byte offset just past the last complete line.
func TestDecoder_RecordsAndOffset(t *testing.T) {
	complete := `{"version":2,"width":80,"height":24,"timestamp":100}` + "\n" +
		`[0.5,"o","hi"]` + "\n" +
		"\n" +
		`not json` + "\n" +
		`[0.6,"r","100x30"]` + "\n" +
		"[" + strings.Repeat("x", maxEventLine) + "]\n" +
		`{"version":2,"width":100,"height":30,"timestamp":200}` + "\n" +
		`[0.1,"o","there"]` + "\n"
	torn := `[0.2,"o","tor`
	d := NewDecoder(strings.NewReader(complete + torn))

	var got []Record
	for {
		rec, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		got = append(got, rec)
	}
	if len(got) != 5 {
		t.Fatalf("records = %d, want 5: %+v", len(got), got)
	}
	if h := got[0].Header; h == nil || h.Width != 80 || h.Timestamp != 100 {
		t.Fatalf("record 0 = %+v, want 80-col header", got[0])
	}
	if e := got[1].Event; e == nil || e.Time != 0.5 || e.Code != EventOutput || e.Data != "hi" {
		t.Fatalf("record 1 = %+v", got[1].Event)
	}
	if e := got[2].Event; e == nil || e.Code != EventResize || e.Data != "100x30" {
		t.Fatalf("record 2 = %+v", got[2].Event)
	}
	if h := got[3].Header; h == nil || h.Width != 100 || h.Timestamp != 200 {
		t.Fatalf("record 3 = %+v", got[3])
	}
	if e := got[4].Event; e == nil || e.Data != "there" {
		t.Fatalf("record 4 = %+v", got[4].Event)
	}
	if d.Offset() != int64(len(complete)) {
		t.Fatalf("Offset = %d, want %d", d.Offset(), len(complete))
	}
	if !bytes.Equal(DecodeAsciicast([]byte(complete+torn)), []byte("hithere")) {
		t.Fatalf("DecodeAsciicast mismatch")
	}
}
