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
	"fmt"
)

// parseCaret expands stty-style caret notation and hex escapes into
// raw bytes.
//
// Supported escapes:
//   - `^@`..`^_`  → 0x00..0x1F (ASCII control characters)
//   - `^?`        → 0x7F (DEL)
//   - `\\xHH`     → single byte HH (two hex digits, case-insensitive)
//   - `\\\\`      → literal backslash (so the hex form remains escapable)
//
// Intentionally unsupported:
//   - Shell-style `\\n`, `\\t`, etc. They are ambiguous with `^M`/`^I`
//     and mask the fact that a real newline is 0x0A. Callers that want
//     a literal newline should write `^M`, `\\x0A`, or use --raw.
//
// Anything not matching one of the above escapes passes through as the
// original UTF-8 bytes of the input.
func parseCaret(in []byte) ([]byte, error) {
	out := make([]byte, 0, len(in))
	for i := 0; i < len(in); i++ {
		b := in[i]
		switch {
		case b == '^':
			if i+1 >= len(in) {
				return nil, fmt.Errorf("dangling '^' at offset %d", i)
			}
			next := in[i+1]
			decoded, ok := caretByte(next)
			if !ok {
				return nil, fmt.Errorf("unsupported caret sequence '^%c' at offset %d", next, i)
			}
			out = append(out, decoded)
			i++
		case b == '\\':
			if i+1 >= len(in) {
				return nil, fmt.Errorf("dangling '\\' at offset %d", i)
			}
			switch in[i+1] {
			case '\\':
				out = append(out, '\\')
				i++
			case 'x', 'X':
				if i+3 >= len(in) {
					return nil, fmt.Errorf("incomplete \\x escape at offset %d", i)
				}
				hi, ok1 := hexNibble(in[i+2])
				lo, ok2 := hexNibble(in[i+3])
				if !ok1 || !ok2 {
					return nil, fmt.Errorf("invalid \\x escape at offset %d", i)
				}
				out = append(out, hi<<4|lo)
				i += 3
			default:
				return nil, fmt.Errorf(
					"unsupported backslash escape '\\%c' at offset %d; use \\xHH or --raw",
					in[i+1], i,
				)
			}
		default:
			out = append(out, b)
		}
	}
	return out, nil
}

// caretByte maps the character after '^' to its control-byte equivalent
// per stty convention. Returns false when the sequence is unsupported.
func caretByte(c byte) (byte, bool) {
	switch {
	case c == '?':
		return 0x7F, true
	case c >= '@' && c <= '_':
		return c - '@', true
	case c >= 'a' && c <= 'z':
		// Lowercase form: ^a..^z == ^A..^Z.
		return c - 'a' + 1, true
	default:
		return 0, false
	}
}

func hexNibble(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	default:
		return 0, false
	}
}
