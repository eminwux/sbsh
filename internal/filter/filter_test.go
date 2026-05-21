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

package filter

import (
	"bytes"
	"testing"
)

func TestNewClientEscapeFilter_DefaultsEscToCtrlRBracket(t *testing.T) {
	f := NewClientEscapeFilter(0, false, nil)
	if f.Esc != 0x1d {
		t.Fatalf("Esc = %#x, want 0x1d (default Ctrl+])", f.Esc)
	}
	if f.PasteAware {
		t.Errorf("PasteAware = true, want false")
	}

	custom := NewClientEscapeFilter('q', true, func() {})
	if custom.Esc != 'q' {
		t.Errorf("Esc = %#x, want 'q'", custom.Esc)
	}
	if !custom.PasteAware {
		t.Errorf("PasteAware = false, want true")
	}
	if custom.OnDetach == nil {
		t.Errorf("OnDetach = nil, want non-nil")
	}
}

func TestProcess_EmptyInput(t *testing.T) {
	f := NewClientEscapeFilter(0, false, nil)
	out, nwrite, ncons, err := f.Process(nil, 0)
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if out != nil || nwrite != 0 || ncons != 0 {
		t.Fatalf("Process(empty) = (%v, %d, %d); want (nil, 0, 0)", out, nwrite, ncons)
	}
}

func TestProcess_NoEscapePassesThrough(t *testing.T) {
	f := NewClientEscapeFilter(0, false, nil)
	buf := []byte("hello world")
	out, nwrite, ncons, err := f.Process(buf, len(buf))
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if out != nil {
		t.Errorf("out = %v, want nil (write buf[:ncons] in place)", out)
	}
	if nwrite != len(buf) || ncons != len(buf) {
		t.Errorf("Process = (nwrite=%d, ncons=%d); want both %d", nwrite, ncons, len(buf))
	}
}

func TestProcess_PrefixBeforeEscapeWrittenFirst(t *testing.T) {
	f := NewClientEscapeFilter(0, false, nil)
	buf := []byte("abc\x1d")
	out, nwrite, ncons, err := f.Process(buf, len(buf))
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if out != nil {
		t.Errorf("out = %v, want nil", out)
	}
	if nwrite != 3 || ncons != 3 {
		t.Errorf("Process = (nwrite=%d, ncons=%d); want (3, 3) — prefix before Esc", nwrite, ncons)
	}
}

func TestProcess_SingleEscapeForwardedAndArmed(t *testing.T) {
	f := NewClientEscapeFilter(0, false, nil)
	buf := []byte{0x1d}
	out, nwrite, ncons, err := f.Process(buf, len(buf))
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if !bytes.Equal(out, []byte{0x1d}) || nwrite != 1 || ncons != 1 {
		t.Fatalf("Process = (out=%v, nwrite=%d, ncons=%d); want (0x1d, 1, 1)", out, nwrite, ncons)
	}
	if !f.sawEsc {
		t.Errorf("sawEsc = false after a single Esc; want true (armed for adjacency)")
	}
}

func TestProcess_AdjacentEscapeWithinChunkTriggersDetach(t *testing.T) {
	detached := false
	f := NewClientEscapeFilter(0, false, func() { detached = true })

	// First Esc forwarded, arms adjacency.
	buf := []byte{0x1d, 0x1d}
	out, nwrite, ncons, err := f.Process(buf, len(buf))
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if !bytes.Equal(out, []byte{0x1d}) || nwrite != 1 || ncons != 1 {
		t.Fatalf("first Esc = (out=%v, nwrite=%d, ncons=%d); want (0x1d, 1, 1)", out, nwrite, ncons)
	}

	// Second Esc (now buf[0]) is swallowed, BEL emitted, detach fires.
	out, nwrite, ncons, err = f.Process(buf[1:], 1)
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if !bytes.Equal(out, []byte{0x07}) || nwrite != 1 || ncons != 1 {
		t.Fatalf("second Esc = (out=%v, nwrite=%d, ncons=%d); want (BEL 0x07, 1, 1)", out, nwrite, ncons)
	}
	if !detached {
		t.Errorf("OnDetach not invoked on adjacent ^]^]")
	}
	if f.sawEsc {
		t.Errorf("sawEsc = true after detach; want false (cleared)")
	}
}

func TestProcess_AdjacentEscapeWithNilOnDetach(t *testing.T) {
	f := NewClientEscapeFilter(0, false, nil)
	f.Process([]byte{0x1d}, 1) // arm
	out, nwrite, ncons, err := f.Process([]byte{0x1d}, 1)
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if !bytes.Equal(out, []byte{0x07}) || nwrite != 1 || ncons != 1 {
		t.Fatalf("second Esc with nil OnDetach = (out=%v, nwrite=%d, ncons=%d); want (BEL, 1, 1)", out, nwrite, ncons)
	}
}

func TestProcess_ArmedButNextByteNotEscapeClearsPending(t *testing.T) {
	f := NewClientEscapeFilter(0, false, func() { t.Fatal("OnDetach should not fire") })
	f.Process([]byte{0x1d}, 1) // arm
	if !f.sawEsc {
		t.Fatal("expected armed after single Esc")
	}
	// Next chunk does not start with Esc → pending cleared, bytes pass through.
	out, nwrite, ncons, err := f.Process([]byte("xyz"), 3)
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if out != nil || nwrite != 3 || ncons != 3 {
		t.Fatalf("Process = (out=%v, nwrite=%d, ncons=%d); want (nil, 3, 3)", out, nwrite, ncons)
	}
	if f.sawEsc {
		t.Errorf("sawEsc = true; want false (cleared by non-Esc next byte)")
	}
}

func TestProcess_PasteModeWritesThroughUntilEndMarker(t *testing.T) {
	f := NewClientEscapeFilter(0, true, func() { t.Fatal("OnDetach should not fire inside paste") })

	// Paste start before any Esc → enter paste mode, write through marker.
	start := []byte{0x1b, '[', '2', '0', '0', '~'}
	out, nwrite, ncons, err := f.Process(start, len(start))
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if out != nil || nwrite != len(start) || ncons != len(start) {
		t.Fatalf("paste start = (out=%v, nwrite=%d, ncons=%d); want (nil, %d, %d)", out, nwrite, ncons, len(start), len(start))
	}
	if !f.pasteMode {
		t.Fatal("pasteMode = false after start marker; want true")
	}

	// Inside paste, an escape byte must NOT trigger detach — passes through.
	body := []byte{0x1d, 'a', 'b'}
	out, nwrite, ncons, err = f.Process(body, len(body))
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if out != nil || nwrite != len(body) || ncons != len(body) {
		t.Fatalf("paste body = (out=%v, nwrite=%d, ncons=%d); want full passthrough", out, nwrite, ncons)
	}

	// Paste end marker exits paste mode.
	end := []byte{0x1b, '[', '2', '0', '1', '~'}
	out, nwrite, ncons, err = f.Process(end, len(end))
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if out != nil || nwrite != len(end) || ncons != len(end) {
		t.Fatalf("paste end = (out=%v, nwrite=%d, ncons=%d); want full passthrough", out, nwrite, ncons)
	}
	if f.pasteMode {
		t.Errorf("pasteMode = true after end marker; want false")
	}
}

func TestProcess_PasteModeNoEndMarkerWritesAll(t *testing.T) {
	f := NewClientEscapeFilter(0, true, nil)
	f.pasteMode = true
	buf := []byte("no end marker here")
	out, nwrite, ncons, err := f.Process(buf, len(buf))
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if out != nil || nwrite != len(buf) || ncons != len(buf) {
		t.Fatalf("Process = (out=%v, nwrite=%d, ncons=%d); want full passthrough", out, nwrite, ncons)
	}
	if !f.pasteMode {
		t.Errorf("pasteMode = false; want still true (no end marker seen)")
	}
}

func TestProcess_EscapeBeforePasteStartTakesEscapePath(t *testing.T) {
	// PasteAware on, but an Esc appears before the paste-start marker.
	// The start marker must NOT win — the Esc path is taken.
	f := NewClientEscapeFilter(0, true, nil)
	buf := append([]byte{0x1d}, 0x1b, '[', '2', '0', '0', '~')
	out, nwrite, ncons, err := f.Process(buf, len(buf))
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	// buf[0] is Esc and we are not armed → forward the single Esc, arm.
	if !bytes.Equal(out, []byte{0x1d}) || nwrite != 1 || ncons != 1 {
		t.Fatalf("Process = (out=%v, nwrite=%d, ncons=%d); want single Esc forwarded", out, nwrite, ncons)
	}
	if f.pasteMode {
		t.Errorf("pasteMode = true; want false (Esc precedes paste start)")
	}
}

func TestIndexOf(t *testing.T) {
	cases := []struct {
		name string
		s    []byte
		pat  []byte
		want int
	}{
		{"empty pattern returns 0", []byte("abc"), nil, 0},
		{"found", []byte("hello"), []byte("ll"), 2},
		{"not found", []byte("hello"), []byte("zz"), -1},
		{"at start", []byte("abc"), []byte("a"), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := indexOf(tc.s, tc.pat); got != tc.want {
				t.Errorf("indexOf(%q, %q) = %d, want %d", tc.s, tc.pat, got, tc.want)
			}
		})
	}
}
