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

import "strings"

// sanitizeField neutralizes terminal control/escape bytes in an
// externally-sourced string before it is written to the operator's terminal in
// a human-readable listing. Names and commands originate from attacker-
// controllable inputs (--name, profile metadata.name, any writer of
// metadata.json under the run path), so a value carrying raw C0/C1 control
// characters or ESC/CSI/OSC sequences would otherwise be rendered verbatim and
// could spoof rows, hide entries, move the cursor, set the title bar, or clear
// the screen of whoever runs `sb get`.
//
// Each control rune — C0 (0x00–0x1F), DEL (0x7F), and C1 (0x80–0x9F, which
// includes the single-byte CSI/OSC introducers a UTF-8 terminal still
// interprets) — is replaced with a printable \xNN escape. Escaping ESC (0x1B)
// alone defangs every CSI/OSC sequence, since the trailing bytes are inert
// printable characters once the introducer is gone. Tab and newline are escaped
// too: they are the tabwriter column/row delimiters, so a raw one embedded in a
// field would corrupt the table layout. Raw bytes that are not valid UTF-8 are
// decoded by the range loop to the printable replacement rune (U+FFFD) and pass
// through harmlessly.
//
// Machine-readable output (-o json / -o yaml) must NOT pass through this — those
// renderings are not terminal-interpreted and callers rely on the exact original
// bytes — so sanitization is applied only at the human-table print sites.
func sanitizeField(s string) string {
	if !needsSanitize(s) {
		return s
	}
	const hex = "0123456789abcdef"
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isControlRune(r) {
			//nolint:mnd // 0x00..0x9F all fit in two hex digits
			b.WriteString(`\x`)
			b.WriteByte(hex[byte(r)>>4])
			b.WriteByte(hex[byte(r)&0x0f])
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// needsSanitize reports whether s contains any control rune sanitizeField would
// rewrite. The common case is a clean name, so this fast path lets sanitizeField
// return the input unchanged without allocating a builder.
func needsSanitize(s string) bool {
	for _, r := range s {
		if isControlRune(r) {
			return true
		}
	}
	return false
}

// isControlRune reports whether r is a C0 control (0x00–0x1F), DEL (0x7F), or a
// C1 control (0x80–0x9F) — the runes a terminal may interpret as commands.
func isControlRune(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}
