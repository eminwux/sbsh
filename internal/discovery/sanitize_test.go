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

package discovery

import "testing"

func TestSanitizeField(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"clean ascii is untouched", "alpha-beta_1", "alpha-beta_1"},
		{"unicode letters pass through", "café-Ω-名前", "café-Ω-名前"},
		{"space is preserved", "two words", "two words"},
		{"ESC is escaped", "evil\x1b[31mRED\x1b[0m", `evil\x1b[31mRED\x1b[0m`},
		{"OSC title-set is defanged", "x\x1b]0;pwned\x07y", `x\x1b]0;pwned\x07y`},
		{"clear-screen CSI is defanged", "a\x1b[2Jb", `a\x1b[2Jb`},
		{"NUL is escaped", "a\x00b", `a\x00b`},
		{"DEL is escaped", "a\x7fb", `a\x7fb`},
		{"tab is escaped so the table cannot be split", "a\tb", `a\x09b`},
		{"newline is escaped so rows cannot be forged", "a\nb", `a\x0ab`},
		{"C1 CSI introducer is escaped", "am", `a\x9bm`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeField(tt.in); got != tt.want {
				t.Fatalf("sanitizeField(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestSanitizeFieldNeutralizesControlBytes is the security invariant: no output
// of sanitizeField ever carries a raw control byte the terminal could act on.
func TestSanitizeFieldNeutralizesControlBytes(t *testing.T) {
	in := "name\x1b[31m\x1b]0;t\x07\x00\x7f"
	got := sanitizeField(in)
	for _, r := range got {
		if isControlRune(r) {
			t.Fatalf("sanitizeField left a raw control rune %#x in %q", r, got)
		}
	}
}
