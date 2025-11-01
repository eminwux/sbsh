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

// Package filter provides a byte-stream filter that detects an adjacent
// Ctrl+] Ctrl+] (0x1D 0x1D) sequence to trigger a detach action, while
// forwarding a single Ctrl+] immediately to the PTY. It optionally ignores
// escape detection inside terminal Bracketed Paste Mode.
package filter

import "bytes"

// SupervisorEscapeFilter implements adjacent-^]^] detach detection on stdin->PTY streams.
// Policy:
//   - Single ^] (0x1D) is written through immediately.
//   - If the *very next byte* is ^] again (adjacent, possibly across reads),
//     the second is swallowed and OnDetach is invoked.
//   - No timers (pure adjacency, zero added latency).
//   - If PasteAware is true, escape detection is disabled between
//     ESC[200~ (paste start) and ESC[201~ (paste end).
type SupervisorEscapeFilter struct {
	// Esc is the escape byte; default is 0x1d (Ctrl+]).
	Esc byte

	// PasteAware toggles Bracketed Paste handling (ESC[200~ ... ESC[201~).
	// When true, escape detection is disabled while pasteMode is active.
	PasteAware bool

	// OnDetach is called when an adjacent ^]^] is detected. It should be fast
	// and non-blocking; if you need slow work, signal a goroutine.
	OnDetach func()

	// --- internal state ---
	sawEsc    bool // true if a ^] was just forwarded and we're watching the next byte
	pasteMode bool // true while inside ESC[200~ ... ESC[201~]
}

// NewSupervisorEscapeFilter creates an EscapeFilter with sensible defaults.
// If esc == 0, it defaults to Ctrl+] (0x1d).
func NewSupervisorEscapeFilter(esc byte, pasteAware bool, onDetach func()) *SupervisorEscapeFilter {
	if esc == 0 {
		esc = 0x1d // Ctrl+]
	}
	return &SupervisorEscapeFilter{
		Esc:        esc,
		PasteAware: pasteAware,
		OnDetach:   onDetach,
	}
}

// Process inspects buf[:n]. It returns:
//   - out:   bytes to write (may be a subslice of buf; nil means "write buf[:ncons]")
//   - nwrite: number of bytes to write (len(out) if out != nil; otherwise ncons)
//   - ncons:  number of input bytes consumed from buf[:n]
//
// The caller must:
//   - advance the input by ncons,
//   - and write exactly nwrite bytes (either 'out' or buf[:nwrite]) before calling again.
//
// When an adjacent escape pair is detected and OnDetach is invoked, the filter
// writes a BEL (0x07) to the terminal slave to reset readline/terminal state
// because one Ctrl+] (0x1D) has already been forwarded.
//
// This design allows the caller to loop until the whole chunk is consumed without
// extra allocations and with precise control over "consume but don't write" cases.
func (e *SupervisorEscapeFilter) Process(buf []byte, n int) ([]byte, int, int, error) {
	if n == 0 {
		return nil, 0, 0, nil
	}

	// Defaults
	esc := e.Esc
	if esc == 0 {
		esc = 0x1d // Ctrl+]
	}

	pasteStart := []byte{0x1b, '[', '2', '0', '0', '~'}
	pasteEnd := []byte{0x1b, '[', '2', '0', '1', '~'}

	if e.PasteAware {
		if e.pasteMode {
			// While in paste mode, write everything through until (and including) the end marker.
			if j := indexOf(buf[:n], pasteEnd); j >= 0 {
				// Write through the end marker and exit paste mode.
				end := j + len(pasteEnd)
				e.pasteMode = false
				e.sawEsc = false // cancel any pending adjacency
				return nil, end, end, nil
			}
			// No end marker in this chunk: write all.
			return nil, n, n, nil
		}

		// Not in paste mode: if a start marker appears *before* any Esc,
		// write up to and including the marker, then enter pasteMode.
		jStart := indexOf(buf[:n], pasteStart)
		iEsc := bytes.IndexByte(buf[:n], esc)

		if jStart >= 0 && (iEsc < 0 || jStart <= iEsc) {
			end := jStart + len(pasteStart)
			e.pasteMode = true
			e.sawEsc = false // cancel pending adjacency just in case
			return nil, end, end, nil
		}
	}

	// ---- Adjacent ^]^] handling ----

	// If we were waiting for a second ^] but the next byte is not ^], clear the pending.
	// (This also covers the case of "no ^] in this chunk".)
	if e.sawEsc && buf[0] != esc {
		e.sawEsc = false
	}

	// Fast path: no Esc in this chunk -> write all, consume all.
	i := bytes.IndexByte(buf[:n], esc)
	if i == -1 {
		return nil, n, n, nil
	}

	// If there are bytes before the first Esc, write that prefix first.
	if i > 0 {
		return nil, i, i, nil
	}

	// buf[0] == Esc here.
	if e.sawEsc {
		// Adjacent second Esc → swallow it, send BEL to reset readline, and trigger detach.
		e.sawEsc = false

		// Immediately return 0x07 (Ctrl+G / BEL) to the terminal slave to abort readline mode.
		bel := []byte{0x07}

		// The filter’s Process return values control what gets written next.
		// We’ll return BEL as the output so the caller writes it before detaching.
		if e.OnDetach != nil {
			e.OnDetach()
		}

		// Write one BEL byte (0x07) to reset readline, consume 1 (^]).
		return bel, 1, 1, nil
	}

	// First Esc → pass it through immediately; arm pending adjacency.
	e.sawEsc = true
	return buf[:1], 1, 1, nil
}

// indexOf finds pat in s and returns its index, or -1.
func indexOf(s, pat []byte) int {
	if len(pat) == 0 {
		return 0
	}
	return bytes.Index(s, pat)
}
