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

// escPhase is the parser phase of an escState.
type escPhase uint8

const (
	escGround       escPhase = iota // between sequences; overlay bytes may be injected
	escEscape                       // saw ESC
	escIntermediate                 // ESC followed by 0x20-0x2F intermediates
	escCSI                          // ESC [ ... until a final byte
	escOSC                          // ESC ] ... until BEL or ST
	escOSCEsc                       // saw ESC inside an OSC (expecting '\' for ST)
	escString                       // DCS / SOS / PM / APC ... until ST
	escStringEsc                    // saw ESC inside a DCS/SOS/PM/APC string
	escUTF8                         // inside a multi-byte UTF-8 rune
)

// C0 controls and bytes with special meaning to the tracker.
const (
	byteBEL = 0x07
	byteCAN = 0x18 // cancel: aborts any sequence in progress
	byteSUB = 0x1a // substitute: aborts any sequence in progress
	byteESC = 0x1b
	byteDEL = 0x7f

	byteHighMin = 0x80 // first non-ASCII byte (UTF-8 lead/continuation or 8-bit)
)

// escCSIParamsCap bounds the CSI parameter bytes kept for private-mode
// decoding. Longer parameter strings are still parsed for their final byte;
// only the alt-screen bookkeeping ignores them.
const escCSIParamsCap = 32

// escState is a byte-level tracker of where a VT byte stream sits relative to
// escape-sequence boundaries. It answers one question for the overlay
// compositor: "if I inject bytes now, could they land inside a sequence the
// stream started?" (AtGround). It recognizes ESC sequences, CSI, OSC (BEL or
// ST terminated), DCS/SOS/PM/APC strings (ST terminated) and partial UTF-8
// runes, and honors CAN/SUB aborts the way a VT terminal does.
//
// It also tracks one piece of terminal state the restore paint needs: whether
// the stream left the terminal on the alternate screen (CSI ?47/?1047/?1049
// h/l, reset by RIS). Everything else is the screen model's job.
//
// The zero value is at ground on the normal screen. Not safe for concurrent
// use; the owning subscriberWriter serializes access under its mutex.
type escState struct {
	phase    escPhase
	utf8Need int // remaining continuation bytes while phase == escUTF8

	csiParams [escCSIParamsCap]byte
	csiLen    int
	csiLong   bool // parameter bytes overflowed csiParams

	altScreen bool
}

// AtGround reports whether the stream is between sequences and runes.
func (e *escState) AtGround() bool { return e.phase == escGround }

// feed advances the tracker over p.
func (e *escState) feed(p []byte) {
	for _, b := range p {
		e.feedByte(b)
	}
}

// feedUntilGround advances the tracker byte by byte and returns the index
// just past the byte that brought it back to ground, or 0 if it already was.
// It returns -1 when p ends without reaching ground (all of p was consumed).
func (e *escState) feedUntilGround(p []byte) int {
	if e.AtGround() {
		return 0
	}
	for i, b := range p {
		e.feedByte(b)
		if e.AtGround() {
			return i + 1
		}
	}
	return -1
}

// reset returns the tracker to ground, as CAN does on the terminal side. The
// alt-screen flag is terminal state, not parser state, so it is kept.
func (e *escState) reset() {
	e.phase = escGround
	e.utf8Need = 0
}

//nolint:gocognit,gocyclo,cyclop,funlen // a flat VT state table reads best as one switch
func (e *escState) feedByte(b byte) {
	// CAN and SUB abort any sequence or partial rune and return to ground.
	if b == byteCAN || b == byteSUB {
		e.reset()
		return
	}

	switch e.phase {
	case escGround:
		e.ground(b)

	case escUTF8:
		if b >= 0x80 && b <= 0xbf {
			e.utf8Need--
			if e.utf8Need == 0 {
				e.phase = escGround
			}
			return
		}
		// A non-continuation byte ends the (malformed) rune; the terminal
		// shows a replacement glyph and processes b afresh.
		e.reset()
		e.ground(b)

	case escEscape:
		e.escape(b)

	case escIntermediate:
		switch {
		case b == byteESC:
			e.phase = escEscape
		case b >= 0x20 && b <= 0x2f:
			// more intermediates
		case b >= 0x30 && b <= 0x7e:
			e.phase = escGround
		}
		// C0 controls execute without leaving the sequence.

	case escCSI:
		switch {
		case b == byteESC:
			e.phase = escEscape
		case b >= 0x30 && b <= 0x3f:
			if e.csiLen < len(e.csiParams) {
				e.csiParams[e.csiLen] = b
				e.csiLen++
			} else {
				e.csiLong = true
			}
		case b >= 0x40 && b <= 0x7e:
			e.csiFinal(b)
			e.phase = escGround
		}
		// Intermediates (0x20-0x2f), C0 controls and DEL stay in CSI.

	case escOSC:
		switch b {
		case byteBEL:
			e.phase = escGround
		case byteESC:
			e.phase = escOSCEsc
		}

	case escOSCEsc:
		if b == '\\' {
			e.phase = escGround
			return
		}
		// ESC not followed by '\' aborts the OSC and starts a new escape.
		e.phase = escEscape
		e.escape(b)

	case escString:
		if b == byteESC {
			e.phase = escStringEsc
		}

	case escStringEsc:
		if b == '\\' {
			e.phase = escGround
			return
		}
		e.phase = escEscape
		e.escape(b)
	}
}

// ground handles a byte at ground state.
func (e *escState) ground(b byte) {
	switch {
	case b == byteESC:
		e.phase = escEscape
	case b >= 0xc2 && b <= 0xdf:
		e.phase, e.utf8Need = escUTF8, 1
	case b >= 0xe0 && b <= 0xef:
		e.phase, e.utf8Need = escUTF8, 2
	case b >= 0xf0 && b <= 0xf4:
		e.phase, e.utf8Need = escUTF8, 3
	}
	// Printable ASCII, C0 controls and stray/invalid UTF-8 bytes leave the
	// stream at ground.
}

// escape handles the byte following ESC.
func (e *escState) escape(b byte) {
	switch {
	case b == byteESC:
		e.phase = escEscape
	case b == '[':
		e.phase = escCSI
		e.csiLen = 0
		e.csiLong = false
	case b == ']':
		e.phase = escOSC
	case b == 'P', b == 'X', b == '^', b == '_': // DCS, SOS, PM, APC
		e.phase = escString
	case b == 'c': // RIS: full reset back to the normal screen
		e.altScreen = false
		e.phase = escGround
	case b >= 0x20 && b <= 0x2f:
		e.phase = escIntermediate
	case b >= 0x30 && b <= 0x7e:
		e.phase = escGround
	case b == byteDEL:
		// ignored
	default:
		// C0 controls execute and stay in the escape; other bytes (UTF-8
		// leads, 8-bit) are invalid here and fall back to ground.
		if b >= byteHighMin {
			e.phase = escGround
			e.ground(b)
		}
	}
}

// csiFinal applies the side effects the tracker cares about once a CSI
// sequence completes: DEC private set/reset of the alternate screen.
func (e *escState) csiFinal(final byte) {
	if (final != 'h' && final != 'l') || e.csiLong || e.csiLen == 0 || e.csiParams[0] != '?' {
		return
	}
	set := final == 'h'
	n, have := 0, false
	apply := func() {
		if have && (n == 47 || n == 1047 || n == 1049) {
			e.altScreen = set
		}
		n, have = 0, false
	}
	for _, c := range e.csiParams[1:e.csiLen] {
		switch {
		case c >= '0' && c <= '9':
			n = n*10 + int(c-'0') //nolint:mnd // decimal accumulation
			have = true
		case c == ';':
			apply()
		default:
			have = false
		}
	}
	apply()
}
