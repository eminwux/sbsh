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
	"fmt"
	"strings"

	"github.com/eminwux/sbsh/pkg/api"
	"github.com/hinshun/vt10x"
)

const sgrReset = "\x1b[0m"

// Escape sequences used to synthesize an attach repaint (see repaint).
const (
	escEnterAltScreen = "\x1b[?1049h" // switch to the alternate screen buffer
	escClearScreen    = "\x1b[2J"     // erase the entire screen
	escCursorHome     = "\x1b[H"      // move cursor to row 1, col 1
	escCursorTo       = "\x1b[%d;%dH" // move cursor to row, col (1-based)
	escShowCursor     = "\x1b[?25h"   // make the cursor visible
	escHideCursor     = "\x1b[?25l"   // hide the cursor
	escEraseToEOL     = "\x1b[K"      // erase from cursor to end of line
	escCursorUpN      = "\x1b[%dA"    // move cursor up n rows (CUU)
	escCursorRightN   = "\x1b[%dC"    // move cursor right n cols (CUF)
)

// vt10x color encoding boundaries. The 16 ANSI colors occupy [0,16):
// [0,8) are the basic colors and [8,16) their bright variants. [16,256)
// is the xterm 256-color palette. 24-bit truecolor is packed as the
// 0xRRGGBB value (below 1<<24). The sentinel default colors
// (DefaultFG/DefaultBG/DefaultCursor) sit at or above 1<<24.
const (
	ansiBasicEnd   = 8
	ansiBrightEnd  = 16
	palette256End  = 256
	defaultColorAt = 1 << 24

	// SGR parameter bases for the 16 ANSI colors (foreground and background).
	sgrFGBasic  = 30
	sgrFGBright = 90
	sgrBGBasic  = 40
	sgrBGBright = 100

	rgbByteMask   = 0xff
	rgbShiftRed   = 16
	rgbShiftGreen = 8
)

// screenModel decodes the PTY byte stream into a live screen grid (cells,
// cursor, colors, alt-screen state). It is registered as an *additional*
// io.Writer sink on the DynamicMultiWriter — fed "warm" from the same
// bytes the capture file receives — so a Screenshot stays O(1) and never
// re-reads the capture file. The capture writer is untouched: the model
// is a second reader of the stream, not a replacement.
//
// vt10x.Terminal is internally synchronized on a single mutex: Write
// (called from the single PTY-reader goroutine via the multiwriter) and
// the snapshot render path (called from a Screenshot RPC goroutine) share
// it, so concurrent feed-and-read is safe.
type screenModel struct {
	vt vt10x.Terminal

	// leftover holds a trailing partial UTF-8 rune that vt10x.Write could
	// not consume at a chunk boundary. The multiwriter calls Write
	// serially from the lone PTY reader, so no lock guards this field.
	leftover []byte
}

// newScreenModel creates a screen model sized to the VT100 default the PTY
// is started at (see startPty); Resize keeps it in step thereafter.
func newScreenModel() *screenModel {
	return &screenModel{vt: vt10x.New(vt10x.WithSize(vt100Cols, vt100Rows))}
}

// Write feeds raw PTY output into the parser. It satisfies io.Writer so
// the model can join the DynamicMultiWriter fan-out. It always reports the
// full payload as written and never returns an error: a parser hiccup must
// not drop the model from the fan-out (which would silently stop tracking
// the screen) — the capture file is the source of truth, the model is
// best-effort.
func (m *screenModel) Write(p []byte) (int, error) {
	buf := p
	if len(m.leftover) > 0 {
		combined := make([]byte, 0, len(m.leftover)+len(p))
		combined = append(combined, m.leftover...)
		combined = append(combined, p...)
		buf = combined
		m.leftover = nil
	}
	// vt10x.Write returns the count of fully-decoded bytes; a trailing
	// incomplete rune (split across this and the next chunk) is left
	// unconsumed. Carry it so the next Write completes the rune instead
	// of corrupting it.
	n, _ := m.vt.Write(buf)
	if n < len(buf) {
		m.leftover = append([]byte(nil), buf[n:]...)
	}
	return len(p), nil
}

// resize keeps the screen grid in step with the PTY winsize so wrapping
// and cursor bounds match what an attached client sees.
func (m *screenModel) resize(cols, rows int) {
	if cols > 0 && rows > 0 {
		m.vt.Resize(cols, rows)
	}
}

// snapshot renders the current grid into an api.ScreenshotResult under a
// single state lock so cells, cursor, and size are a mutually consistent
// frame (no Write can interleave mid-render).
func (m *screenModel) snapshot() *api.ScreenshotResult {
	m.vt.Lock()
	defer m.vt.Unlock()

	cols, rows := m.vt.Size()
	cur := m.vt.Cursor()
	res := &api.ScreenshotResult{
		Cols:          cols,
		Rows:          rows,
		CursorX:       cur.X,
		CursorY:       cur.Y,
		CursorVisible: m.vt.CursorVisible(),
		AltScreen:     m.vt.Mode()&vt10x.ModeAltScreen != 0,
	}
	res.Text, res.ANSI = renderGrid(m.vt, cols, rows)
	return res
}

// repaint synthesizes a byte sequence that, written to a freshly attached
// client's raw-mode terminal, reproduces the current screen. Unlike the raw
// capture replay it is bounded to the viewport regardless of how long the
// session has run, which is the whole point of repaint-on-attach.
//
// By default the paint never destroys the client's existing terminal
// content: rows are framed relatively from the cursor's current line
// ("\r\n" between rows, since in raw mode a bare "\n" is a line feed that
// does not return the carriage) so prior content scrolls up intact, and the
// final cursor lands via relative moves (LF/CUU for rows, CR+CUF for the
// column — relative because the client's absolute cursor row is unknown to
// the server). Each painted row ends with an erase-to-EOL so stale client
// content cannot bleed through shorter rows. An empty screen model paints
// no rows at all.
//
// clearScreen opts back into the legacy clear-and-repaint: erase the whole
// screen, home, then absolute per-row positioning (CSI row;1H). Alt-screen
// sessions always take the absolute path inside the alt buffer — entering
// the alt screen never destroys the client's normal-buffer content, and
// TUIs need absolute coordinates to render correctly.
//
// It is taken under the vt10x state lock so cells, cursor, and mode form
// one consistent frame.
func (m *screenModel) repaint(clearScreen bool) []byte {
	m.vt.Lock()
	defer m.vt.Unlock()

	cols, rows := m.vt.Size()
	cur := m.vt.Cursor()
	altScreen := m.vt.Mode()&vt10x.ModeAltScreen != 0
	cursorVisible := m.vt.CursorVisible()

	textLines := make([]string, rows)
	ansiLines := make([]string, rows)
	for y := range rows {
		textLines[y], ansiLines[y] = renderLine(m.vt, cols, y)
	}
	// Trailing blank rows need no paint. Stop at the last non-empty row
	// (plain text is the canonical emptiness signal, mirroring renderGrid).
	end := len(textLines)
	for end > 0 && textLines[end-1] == "" {
		end--
	}

	var b strings.Builder
	if altScreen || clearScreen {
		writeAbsolutePaint(&b, ansiLines[:end], cur, altScreen)
	} else {
		writeRelativePaint(&b, ansiLines[:end], cur)
	}
	if cursorVisible {
		b.WriteString(escShowCursor)
	} else {
		b.WriteString(escHideCursor)
	}
	return []byte(b.String())
}

// writeAbsolutePaint emits the legacy clear-and-repaint: erase the whole
// screen, home, one absolutely positioned write per row, absolute final
// cursor. Used for alt-screen sessions (entering the alt buffer first) and
// for the --clear-screen opt-in on the normal buffer.
func writeAbsolutePaint(b *strings.Builder, lines []string, cur vt10x.Cursor, altScreen bool) {
	if altScreen {
		b.WriteString(escEnterAltScreen)
	}
	b.WriteString(escClearScreen)
	b.WriteString(escCursorHome)
	for y, line := range lines {
		fmt.Fprintf(b, escCursorTo, y+1, 1)
		b.WriteString(line)
	}
	// vt10x cursor coordinates are 0-based; CSI row;colH is 1-based.
	fmt.Fprintf(b, escCursorTo, cur.Y+1, cur.X+1)
}

// writeRelativePaint emits the default non-destructive paint: rows framed
// from the client cursor's current line with "\r\n", each erased to EOL so
// stale client content cannot bleed through, then the final cursor placed
// with relative moves only.
func writeRelativePaint(b *strings.Builder, lines []string, cur vt10x.Cursor) {
	for y, line := range lines {
		if y == 0 {
			b.WriteString("\r")
		} else {
			b.WriteString("\r\n")
		}
		b.WriteString(line)
		b.WriteString(escEraseToEOL)
	}
	// The physical line now under the cursor corresponds to the last
	// painted screen row (or the starting line when nothing painted).
	// Walk to the cursor's row relatively: LF scrolls at the bottom
	// margin where CUD would not, CUU for upward moves.
	lastRow := 0
	if len(lines) > 0 {
		lastRow = len(lines) - 1
	}
	if d := cur.Y - lastRow; d > 0 {
		b.WriteString(strings.Repeat("\n", d))
	} else if d < 0 {
		fmt.Fprintf(b, escCursorUpN, -d)
	}
	b.WriteString("\r")
	if cur.X > 0 {
		fmt.Fprintf(b, escCursorRightN, cur.X)
	}
}

// renderGrid walks the grid row by row and produces both the plain-text
// and the SGR-colored renderings. The caller must hold the vt10x state
// lock (see snapshot); renderGrid uses the lock-free Cell accessor.
// Trailing blank lines are trimmed so the output reads like a screen, not
// an 80x24 block of padding.
func renderGrid(v vt10x.View, cols, rows int) (string, string) {
	textLines := make([]string, rows)
	ansiLines := make([]string, rows)
	for y := range rows {
		textLines[y], ansiLines[y] = renderLine(v, cols, y)
	}

	// Drop trailing blank lines (plain text is the canonical "is this row
	// empty" signal; the ANSI line for the same row is dropped in lockstep).
	end := len(textLines)
	for end > 0 && textLines[end-1] == "" {
		end--
	}

	text := strings.Join(textLines[:end], "\n")
	ansi := strings.Join(ansiLines[:end], "\n")
	if text != "" {
		text += "\n"
	}
	if ansi != "" {
		ansi += "\n"
	}
	return text, ansi
}

// renderLine renders a single grid row, returning its plain text and its
// SGR-colored form. Trailing blank columns are dropped.
func renderLine(v vt10x.View, cols, y int) (string, string) {
	last := -1
	for x := range cols {
		if ch := v.Cell(x, y).Char; ch != ' ' && ch != 0 {
			last = x
		}
	}

	var text, ansi strings.Builder
	curFG, curBG := vt10x.DefaultFG, vt10x.DefaultBG
	colored := false
	for x := 0; x <= last; x++ {
		g := v.Cell(x, y)
		ch := g.Char
		if ch == 0 {
			ch = ' '
		}
		text.WriteRune(ch)

		if g.FG != curFG || g.BG != curBG {
			fmt.Fprintf(&ansi, "\x1b[%s;%sm", ansiFG(g.FG), ansiBG(g.BG))
			curFG, curBG = g.FG, g.BG
			if g.FG != vt10x.DefaultFG || g.BG != vt10x.DefaultBG {
				colored = true
			}
		}
		ansi.WriteRune(ch)
	}
	if colored {
		ansi.WriteString(sgrReset)
	}
	return text.String(), ansi.String()
}

// ansiFG maps a vt10x foreground color to its SGR parameter.
func ansiFG(c vt10x.Color) string {
	return ansiColor(c, sgrFGBasic, sgrFGBright, "38", "39")
}

// ansiBG mirrors ansiFG for background colors.
func ansiBG(c vt10x.Color) string {
	return ansiColor(c, sgrBGBasic, sgrBGBright, "48", "49")
}

// ansiColor renders one color as an SGR parameter list. basicBase/brightBase
// are the SGR bases for the 16 ANSI colors (30/90 fg, 40/100 bg); extPrefix
// is "38"/"48" for the 256-color and truecolor forms; defaultParam is the
// "reset to default" parameter (39 fg, 49 bg).
func ansiColor(c vt10x.Color, basicBase, brightBase vt10x.Color, extPrefix, defaultParam string) string {
	switch {
	case c >= defaultColorAt: // DefaultFG / DefaultBG / DefaultCursor
		return defaultParam
	case c < ansiBasicEnd:
		return fmt.Sprintf("%d", basicBase+c)
	case c < ansiBrightEnd:
		return fmt.Sprintf("%d", brightBase+(c-ansiBasicEnd))
	case c < palette256End:
		return fmt.Sprintf("%s;5;%d", extPrefix, c)
	default:
		return fmt.Sprintf("%s;2;%d;%d;%d", extPrefix,
			(c>>rgbShiftRed)&rgbByteMask, (c>>rgbShiftGreen)&rgbByteMask, c&rgbByteMask)
	}
}
