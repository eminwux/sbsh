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

package write

import (
	"bytes"
	"testing"
)

func TestParseCaret(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		in      string
		want    []byte
		wantErr bool
	}{
		{"empty", "", []byte{}, false},
		{"plain ascii", "hello", []byte("hello"), false},
		{"ctrl-c", "^C", []byte{0x03}, false},
		{"ctrl-d", "^D", []byte{0x04}, false},
		{"ctrl-z", "^Z", []byte{0x1A}, false},
		{"ctrl-m CR", "^M", []byte{0x0D}, false},
		{"ctrl-i TAB", "^I", []byte{0x09}, false},
		{"ctrl-null", "^@", []byte{0x00}, false},
		{"escape", "^[", []byte{0x1B}, false},
		{"group-sep", "^]", []byte{0x1D}, false},
		{"record-sep", "^^", []byte{0x1E}, false},
		{"unit-sep", "^_", []byte{0x1F}, false},
		{"del", "^?", []byte{0x7F}, false},
		{"lowercase ^c", "^c", []byte{0x03}, false},
		{"hex lowercase", `\x0a`, []byte{0x0A}, false},
		{"hex uppercase", `\xFF`, []byte{0xFF}, false},
		{"hex mixed case", `\xaB`, []byte{0xAB}, false},
		{"literal caret via hex", `\x5E`, []byte{'^'}, false},
		{"literal backslash", `\\`, []byte{'\\'}, false},
		{"mixed sequence", `echo hi^M`, append([]byte("echo hi"), 0x0D), false},
		{"hex then plain", `\x03rest`, append([]byte{0x03}, []byte("rest")...), false},

		// negative cases
		{"shell \\n not supported", `\n`, nil, true},
		{"shell \\t not supported", `\t`, nil, true},
		{"dangling caret", "^", nil, true},
		{"dangling backslash", `\`, nil, true},
		{"bad caret", "^!", nil, true},
		{"incomplete hex", `\x0`, nil, true},
		{"non-hex nibble", `\xGG`, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseCaret([]byte(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got none (out=%q)", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("parseCaret(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
