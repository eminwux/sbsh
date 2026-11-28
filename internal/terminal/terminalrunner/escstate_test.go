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

import (
	"testing"

	"github.com/hinshun/vt10x"
)

func TestEscStateGround(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		ground bool
	}{
		{"empty", "", true},
		{"plain", "hello\r\n", true},
		{"lone ESC", "\x1b", false},
		{"ESC final", "\x1b7", true},
		{"ESC intermediate pending", "\x1b(", false},
		{"ESC intermediate final", "\x1b(B", true},
		{"CSI pending params", "\x1b[3", false},
		{"CSI pending intro", "\x1b[", false},
		{"CSI complete", "\x1b[31m", true},
		{"CSI private complete", "\x1b[?25l", true},
		{"CSI with C0 inside", "\x1b[3\r1m", true},
		{"CSI aborted by ESC", "\x1b[3\x1b", false},
		{"OSC pending", "\x1b]0;title", false},
		{"OSC BEL", "\x1b]0;title\x07", true},
		{"OSC ST", "\x1b]0;title\x1b\\", true},
		{"OSC ST split", "\x1b]0;title\x1b", false},
		{"DCS pending", "\x1bPq#0;2;0;0;0", false},
		{"DCS BEL does not end", "\x1bPdata\x07", false},
		{"DCS ST", "\x1bPdata\x1b\\", true},
		{"APC ST", "\x1b_data\x1b\\", true},
		{"PM ST", "\x1b^data\x1b\\", true},
		{"SOS ST", "\x1bXdata\x1b\\", true},
		{"utf8 2-byte partial", "\xc3", false},
		{"utf8 2-byte", "\xc3\xa9", true},
		{"utf8 3-byte partial", "\xe2\x94", false},
		{"utf8 3-byte", "\xe2\x94\x80", true},
		{"utf8 4-byte partial", "\xf0\x9f\x98", false},
		{"utf8 4-byte", "\xf0\x9f\x98\x80", true},
		{"utf8 broken by ascii", "\xe2a", true},
		{"utf8 broken by ESC", "\xe2\x1b", false},
		{"CAN aborts CSI", "\x1b[3\x18", true},
		{"SUB aborts OSC", "\x1b]0;t\x1a", true},
		{"CAN aborts utf8", "\xe2\x18", true},
		{"stray continuation", "\x80", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var e escState
			e.feed([]byte(tc.in))
			if got := e.AtGround(); got != tc.ground {
				t.Errorf("AtGround after %q = %v, want %v", tc.in, got, tc.ground)
			}
		})
	}
}

// Splitting a stream at every possible point must not change the final
// state: the tracker is a pure byte-level machine.
func TestEscStateChunkInvariant(t *testing.T) {
	stream := "a\x1b[1;31mb\x1b]0;t\x07c\xe2\x94\x80\x1bPx\x1b\\d\x1b[?1049h"
	var whole escState
	whole.feed([]byte(stream))
	for i := range len(stream) + 1 {
		var split escState
		split.feed([]byte(stream[:i]))
		split.feed([]byte(stream[i:]))
		if split != whole {
			t.Fatalf("split at %d diverged: %+v vs %+v", i, split, whole)
		}
	}
}

func TestEscStateFeedUntilGround(t *testing.T) {
	var e escState
	if got := e.feedUntilGround([]byte("abc")); got != 0 {
		t.Errorf("already at ground: got %d, want 0", got)
	}
	e.feed([]byte("\x1b[3"))
	if got := e.feedUntilGround([]byte("1mXY")); got != 2 {
		t.Errorf("CSI tail: got %d, want 2", got)
	}
	e.feed([]byte("\x1b]0;"))
	if got := e.feedUntilGround([]byte("still")); got != -1 {
		t.Errorf("unterminated: got %d, want -1", got)
	}
}

func TestEscStateAltScreen(t *testing.T) {
	cases := []struct {
		in  string
		alt bool
	}{
		{"\x1b[?1049h", true},
		{"\x1b[?1049h\x1b[?1049l", false},
		{"\x1b[?47h", true},
		{"\x1b[?1047h", true},
		{"\x1b[?25;1049h", true},
		{"\x1b[?1049h\x1bc", false},
		{"\x1b[1049h", false}, // not private
		{"\x1b[?10490h", false},
	}
	for _, tc := range cases {
		var e escState
		e.feed([]byte(tc.in))
		if e.altScreen != tc.alt {
			t.Errorf("altScreen after %q = %v, want %v", tc.in, e.altScreen, tc.alt)
		}
	}
}

// vt10x keeps its attribute and cursor-state bits unexported; the mirrors in
// terminal_screen.go must match them. A vt10x upgrade that renumbers them
// fails here instead of silently corrupting redraw styles.
func TestVT10xAttrBitsPinned(t *testing.T) {
	cases := []struct {
		sgr  string
		want int16
	}{
		{"\x1b[7m", vtAttrReverse},
		{"\x1b[4m", vtAttrUnderline},
		{"\x1b[1m", vtAttrBold},
		{"\x1b[3m", vtAttrItalic},
		{"\x1b[5m", vtAttrBlink},
	}
	for _, tc := range cases {
		vt := vt10x.New(vt10x.WithSize(10, 2))
		_, _ = vt.Write([]byte(tc.sgr + "X"))
		vt.Lock()
		got := vt.Cell(0, 0).Mode & vtStyleMask
		vt.Unlock()
		if got != tc.want {
			t.Errorf("%q: cell mode = %#x, want %#x", tc.sgr, got, tc.want)
		}
	}

	vt := vt10x.New(vt10x.WithSize(4, 2))
	_, _ = vt.Write([]byte("abcd"))
	vt.Lock()
	cur := vt.Cursor()
	vt.Unlock()
	if cur.State&vtCursorWrapNext == 0 || cur.X != 3 {
		t.Errorf("after filling a row: cursor state %#x x=%d, want wrap-next set at x=3", cur.State, cur.X)
	}
}
