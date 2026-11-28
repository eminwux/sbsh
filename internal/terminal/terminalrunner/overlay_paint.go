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

	"github.com/hinshun/vt10x"
)

// Escape sequences used only by the overlay paints.
const (
	escSyncBegin = "\x1b[?2026h" // synchronized output: hold rendering
	escSyncEnd   = "\x1b[?2026l" // synchronized output: render now
	// Buffer switches for the restore. ?1047 switches buffers only; ?1049
	// would also save (h) or restore (l) the cursor like DECSC/DECRC, which
	// overwrites or reads the workload's saved-cursor slot.
	escUseAltBuffer    = "\x1b[?1047h"
	escUseNormalBuffer = "\x1b[?1047l"
)

// restorePaint returns the bytes that bring a client terminal whose screen
// was overwritten (by takeover frames) back to exactly the model's state:
// the buffer (normal/alt), every cell with its colors and text style
// (styled trailing blanks included), the cursor position (including a pending wrap), the current pen, and the
// cursor visibility. It uses absolute positioning only and never touches the
// saved-cursor slot (no DECSC/DECRC), which belongs to the workload. The
// whole paint is wrapped in synchronized output.
//
// clientAlt is whether the client terminal is currently on the alternate
// screen (tracked by the client's escState); the buffer is switched only
// when it disagrees with the model. alt reports whether the model (and so
// the painted buffer) is the alternate screen.
func (m *screenModel) restorePaint(clientAlt bool) ([]byte, bool) {
	m.vt.Lock()
	defer m.vt.Unlock()

	cols, rows := m.vt.Size()
	altScreen := m.vt.Mode()&vt10x.ModeAltScreen != 0

	var b strings.Builder
	b.WriteString(escSyncBegin)
	switch {
	case altScreen && !clientAlt:
		b.WriteString(escUseAltBuffer)
	case !altScreen && clientAlt:
		b.WriteString(escUseNormalBuffer)
	}
	b.WriteString(sgrReset)
	b.WriteString(escClearScreen)
	for y := range rows {
		// Paint up to the last cell that differs from a cleared one. Unlike
		// renderLine, this keeps trailing blanks that carry a pen (a
		// reverse-video title bar, a colored background), which the client
		// showed before the takeover and the clear above wiped.
		if last := m.lastPaintedCellLocked(cols, y); last >= 0 {
			m.writeSpanLocked(&b, y, 0, last+1)
		}
	}
	m.writeCursorStateLocked(&b, cols, nil)
	b.WriteString(escSyncEnd)
	return []byte(b.String()), altScreen
}

// lastPaintedCellLocked returns the index of the last cell in row y whose
// rune is not blank or whose pen (colors or text style) is not the default,
// or -1 when the whole row looks like a cleared one. The caller holds the vt
// lock.
func (m *screenModel) lastPaintedCellLocked(cols, y int) int {
	for x := cols - 1; x >= 0; x-- {
		g := m.vt.Cell(x, y)
		if g.Char != ' ' && g.Char != 0 {
			return x
		}
		if fg, bg, mode := cellPen(g); fg != vt10x.DefaultFG || bg != vt10x.DefaultBG || mode != 0 {
			return x
		}
	}
	return -1
}

// writeSpanLocked paints cells [x0,x1) of row y exactly as the model holds
// them — every cell, styled blanks included — starting from an absolute
// cursor move and ending with the pen reset. The caller holds the vt lock.
func (m *screenModel) writeSpanLocked(b *strings.Builder, y, x0, x1 int) {
	fmt.Fprintf(b, escCursorTo, y+1, x0+1)
	first := true
	var curFG, curBG vt10x.Color
	var curMode int16
	for x := x0; x < x1; x++ {
		g := m.vt.Cell(x, y)
		fg, bg, mode := cellPen(g)
		if first || fg != curFG || bg != curBG || mode != curMode {
			b.WriteString(sgrPen(mode, fg, bg))
			curFG, curBG, curMode, first = fg, bg, mode, false
		}
		ch := g.Char
		if ch == 0 {
			ch = ' '
		}
		b.WriteRune(ch)
	}
	b.WriteString(sgrReset)
}

// writeCursorStateLocked re-establishes the model's cursor position, pen and
// cursor visibility on the client after overlay bytes moved them. A pending
// wrap (cursor parked past the last column after writing it) cannot be
// expressed with a cursor move — CUP clears it — so it is recreated by
// re-writing the last cell of the row, which leaves the terminal in exactly
// that state.
//
// keep, when non-nil, is a toast box just drawn on the client: if the cell
// the wrap recreation would rewrite lies inside it, the wrap is not
// recreated (that would replace the box's border with the model's cell) and
// the cursor is only moved there. The client's pending wrap is lost until the
// box erase, which always runs before the next live byte reaches the client
// and recreates it. The caller holds the vt lock.
func (m *screenModel) writeCursorStateLocked(b *strings.Builder, cols int, keep *toastBox) {
	cur := m.vt.Cursor()
	if cur.State&vtCursorWrapNext != 0 && cols > 0 && !keep.contains(cols-1, cur.Y) {
		m.writeSpanLocked(b, cur.Y, cols-1, cols)
	} else {
		fmt.Fprintf(b, escCursorTo, cur.Y+1, cur.X+1)
	}
	b.WriteString(sgrPen(cur.Attr.Mode&vtStyleMask, cur.Attr.FG, cur.Attr.BG))
	if m.vt.CursorVisible() {
		b.WriteString(escShowCursor)
	} else {
		b.WriteString(escHideCursor)
	}
}

// size returns the model's grid size.
func (m *screenModel) size() (int, int) {
	m.vt.Lock()
	defer m.vt.Unlock()
	return m.vt.Size()
}

// toastBox is the on-screen rectangle of a laid-out toast (0-based cells).
type toastBox struct {
	left, top, width, height int
}

// contains reports whether cell (x, y) lies inside the box. A nil box
// contains nothing.
func (bx *toastBox) contains(x, y int) bool {
	return bx != nil && x >= bx.left && x < bx.left+bx.width && y >= bx.top && y < bx.top+bx.height
}

// Toast box geometry: one border cell on each side plus one padding column
// on each side of the text.
const (
	toastBorderRows = 2
	toastChromeCols = 4
	toastMinCols    = toastChromeCols + 1
	toastMinRows    = toastBorderRows + 1
)

// layoutToast places a toast's box in its corner of a cols x rows screen,
// truncating lines that do not fit. ok is false when the screen is too small
// to hold even a one-column, one-line box.
func layoutToast(t *Toast, cols, rows int) (toastBox, []string, bool) {
	if cols < toastMinCols || rows < toastMinRows || len(t.Lines) == 0 {
		return toastBox{}, nil, false
	}
	lines := t.Lines
	if maxLines := rows - toastBorderRows; len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	textWidth := 0
	for _, l := range lines {
		textWidth = max(textWidth, len([]rune(l)))
	}
	textWidth = max(1, min(textWidth, cols-toastChromeCols))
	out := make([]string, len(lines))
	for i, l := range lines {
		r := []rune(l)
		if len(r) > textWidth {
			r = r[:textWidth]
		}
		out[i] = string(r) + strings.Repeat(" ", textWidth-len(r))
	}

	box := toastBox{width: textWidth + toastChromeCols, height: len(out) + toastBorderRows}
	switch t.Position {
	case ToastTopRight:
		box.left = cols - box.width
	case ToastBottomLeft:
		box.top = rows - box.height
	case ToastBottomRight:
		box.left, box.top = cols-box.width, rows-box.height
	case ToastTopLeft:
	}
	return box, out, true
}

// toastSGR is the pen a toast box is drawn with.
func toastSGR(s ToastStyle) string {
	switch s {
	case ToastWarning:
		return "\x1b[0;1;30;43m"
	case ToastError:
		return "\x1b[0;1;97;41m"
	case ToastInfo:
		fallthrough
	default:
		return "\x1b[0;1;97;44m"
	}
}

// toastFrame lays t out against the model's current size and returns the
// bytes that draw it over the client's screen and the bytes that erase it
// again by repainting the covered cells from the model as it is right now.
// Both end by restoring the model's cursor, pen and visibility and are
// wrapped in synchronized output; a pending wrap whose cell the box covers
// is restored by the erase only (see writeCursorStateLocked). ok is false
// when the toast does not fit.
func (m *screenModel) toastFrame(t *Toast) ([]byte, []byte, bool) {
	m.vt.Lock()
	defer m.vt.Unlock()

	cols, rows := m.vt.Size()
	box, lines, ok := layoutToast(t, cols, rows)
	if !ok {
		return nil, nil, false
	}

	var draw strings.Builder
	draw.WriteString(escSyncBegin)
	draw.WriteString(toastSGR(t.Style))
	inner := box.width - toastBorderRows
	for r := range box.height {
		fmt.Fprintf(&draw, escCursorTo, box.top+r+1, box.left+1)
		switch r {
		case 0:
			draw.WriteString("┌" + strings.Repeat("─", inner) + "┐")
		case box.height - 1:
			draw.WriteString("└" + strings.Repeat("─", inner) + "┘")
		default:
			draw.WriteString("│ " + lines[r-1] + " │")
		}
	}
	draw.WriteString(sgrReset)
	m.writeCursorStateLocked(&draw, cols, &box)
	draw.WriteString(escSyncEnd)

	var erase strings.Builder
	erase.WriteString(escSyncBegin)
	for r := range box.height {
		m.writeSpanLocked(&erase, box.top+r, box.left, box.left+box.width)
	}
	m.writeCursorStateLocked(&erase, cols, nil)
	erase.WriteString(escSyncEnd)

	return []byte(draw.String()), []byte(erase.String()), true
}
