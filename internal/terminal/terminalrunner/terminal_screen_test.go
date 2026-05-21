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
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/eminwux/sbsh/pkg/api"
)

// feed writes each chunk to the model in order, mirroring how the
// multiwriter hands the parser successive PTY read buffers.
func feed(m *screenModel, chunks ...string) {
	for _, c := range chunks {
		_, _ = m.Write([]byte(c))
	}
}

func TestScreenshotPlainOutput(t *testing.T) {
	m := newScreenModel()
	feed(m, "hello world")

	snap := m.snapshot()
	if snap.Cols != vt100Cols || snap.Rows != vt100Rows {
		t.Fatalf("unexpected grid size: got %dx%d want %dx%d", snap.Cols, snap.Rows, vt100Cols, vt100Rows)
	}
	if !strings.Contains(snap.Text, "hello world") {
		t.Errorf("plain text should contain the written line; got %q", snap.Text)
	}
	// Trailing blank rows are trimmed: a single line of output is one line.
	if got := strings.Count(snap.Text, "\n"); got != 1 {
		t.Errorf("expected one rendered line, got %d newlines in %q", got, snap.Text)
	}
}

func TestScreenshotCursorPositioning(t *testing.T) {
	m := newScreenModel()
	// CUP (1-based) to row 5, col 10 -> 0-based cursor (x=9, y=4).
	feed(m, "\x1b[5;10H")

	snap := m.snapshot()
	if snap.CursorX != 9 || snap.CursorY != 4 {
		t.Errorf("cursor position: got (%d,%d) want (9,4)", snap.CursorX, snap.CursorY)
	}
	if !snap.CursorVisible {
		t.Errorf("cursor should be visible by default")
	}

	// Hiding the cursor (DECTCEM reset) must be reflected in the snapshot.
	feed(m, "\x1b[?25l")
	if m.snapshot().CursorVisible {
		t.Errorf("cursor should be hidden after \\x1b[?25l")
	}
}

func TestScreenshotColorSGR(t *testing.T) {
	m := newScreenModel()
	// Red foreground, then reset.
	feed(m, "\x1b[31mRED\x1b[0mplain")

	snap := m.snapshot()
	if !strings.Contains(snap.Text, "REDplain") {
		t.Errorf("plain text drops SGR but keeps glyphs; got %q", snap.Text)
	}
	// The ANSI rendering must carry the decoded foreground color (SGR 31),
	// proving the model tracks color state rather than echoing raw bytes.
	if !strings.Contains(snap.ANSI, "\x1b[31;49m") {
		t.Errorf("ANSI rendering should contain red foreground SGR; got %q", snap.ANSI)
	}
	// "plain" reverts to default colors, so a reset must appear before it.
	if !strings.Contains(snap.ANSI, sgrReset) {
		t.Errorf("ANSI rendering should reset back to default; got %q", snap.ANSI)
	}
}

func TestScreenshot256AndBackgroundColor(t *testing.T) {
	m := newScreenModel()
	// 256-color foreground (208) on a blue background (SGR 44).
	feed(m, "\x1b[38;5;208m\x1b[44mX")

	ansi := m.snapshot().ANSI
	if !strings.Contains(ansi, "38;5;208") {
		t.Errorf("ANSI should carry 256-color foreground; got %q", ansi)
	}
	if !strings.Contains(ansi, "44") {
		t.Errorf("ANSI should carry blue background; got %q", ansi)
	}
}

func TestScreenshotAltScreen(t *testing.T) {
	m := newScreenModel()
	// Draw on the primary screen first.
	feed(m, "primary content")
	if m.snapshot().AltScreen {
		t.Fatalf("should start on the primary screen")
	}

	// Enter the alternate screen (DECSET 1049) the way vim/htop do, then
	// draw a full-screen UI on it.
	feed(m, "\x1b[?1049h", "\x1b[2J\x1b[H", "ALT SCREEN APP")
	alt := m.snapshot()
	if !alt.AltScreen {
		t.Errorf("AltScreen should be true after \\x1b[?1049h")
	}
	if !strings.Contains(alt.Text, "ALT SCREEN APP") {
		t.Errorf("screenshot should reflect the alt-screen grid; got %q", alt.Text)
	}
	if strings.Contains(alt.Text, "primary content") {
		t.Errorf("alt screen must not show primary content; got %q", alt.Text)
	}

	// Leave the alternate screen; the primary content is restored.
	feed(m, "\x1b[?1049l")
	restored := m.snapshot()
	if restored.AltScreen {
		t.Errorf("AltScreen should be false after \\x1b[?1049l")
	}
	if !strings.Contains(restored.Text, "primary content") {
		t.Errorf("primary content should be restored on exit; got %q", restored.Text)
	}
	if strings.Contains(restored.Text, "ALT SCREEN APP") {
		t.Errorf("alt-screen content must not leak back to primary; got %q", restored.Text)
	}
}

// A multibyte rune split across two PTY read buffers must still decode to a
// single glyph, not a corrupted pair. The model carries the trailing
// partial rune between Writes.
func TestScreenshotSplitMultibyteRune(t *testing.T) {
	m := newScreenModel()
	// "café" — the 'é' (U+00E9) is 0xC3 0xA9; split it across two writes.
	feed(m, "caf\xc3", "\xa9!")

	text := m.snapshot().Text
	if !strings.Contains(text, "café!") {
		t.Errorf("split multibyte rune should decode intact; got %q", text)
	}
}

// The screen model is fed warm as an additional sink on the multiwriter;
// adding it must not disturb the capture writer, which still receives the
// exact same bytes.
func TestScreenModelWarmFeedDoesNotDisturbCapture(t *testing.T) {
	var capture bytes.Buffer
	mw := NewDynamicMultiWriter(nil, &capture)
	m := newScreenModel()
	mw.Add(m)

	raw := "\x1b[32mok\x1b[0m\r\n"
	if _, err := mw.Write([]byte(raw)); err != nil {
		t.Fatalf("multiwriter write: %v", err)
	}

	// Capture file gets the raw bytes verbatim (unchanged by the parser).
	if capture.String() != raw {
		t.Errorf("capture writer should receive raw bytes verbatim; got %q want %q", capture.String(), raw)
	}
	// The model decoded the same stream into a screen.
	if !strings.Contains(m.snapshot().Text, "ok") {
		t.Errorf("screen model should have decoded the warm-fed bytes; got %q", m.snapshot().Text)
	}
}

// The warm feed (Write from the PTY reader goroutine) and Screenshot reads
// (snapshot from an RPC goroutine) run concurrently. Run under -race to
// prove vt10x's internal lock makes that safe.
func TestScreenModelConcurrentWriteAndSnapshot(t *testing.T) {
	m := newScreenModel()
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 500 {
			_, _ = m.Write([]byte("\x1b[31mx\x1b[0m"))
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 500 {
			_ = m.snapshot()
		}
	}()

	wg.Wait()

	// The grid survived the concurrent feed intact (dimensions unchanged).
	if snap := m.snapshot(); snap.Cols != vt100Cols || snap.Rows != vt100Rows {
		t.Errorf("grid corrupted under concurrent access: got %dx%d", snap.Cols, snap.Rows)
	}
}

// An empty screen (no output yet, the "empty capture" case) repaints to a
// bare clear-screen + home, no alt-screen switch, cursor home and visible.
func TestRepaintEmptyScreen(t *testing.T) {
	m := newScreenModel()
	got := string(m.repaint())

	if strings.Contains(got, escEnterAltScreen) {
		t.Errorf("empty primary screen must not enter alt screen; got %q", got)
	}
	if !strings.Contains(got, escClearScreen) || !strings.Contains(got, escCursorHome) {
		t.Errorf("repaint must clear and home; got %q", got)
	}
	// Cursor lands at row 1, col 1 and stays visible.
	if !strings.HasSuffix(got, "\x1b[1;1H"+escShowCursor) {
		t.Errorf("empty repaint should end at home with a visible cursor; got %q", got)
	}
}

// A plain line of output repaints by positioning at row 1 and writing the
// glyphs — no newlines, which would stairstep in raw mode.
func TestRepaintPlainOutput(t *testing.T) {
	m := newScreenModel()
	feed(m, "hello world")
	got := string(m.repaint())

	if !strings.Contains(got, "\x1b[1;1Hhello world") {
		t.Errorf("repaint should position row 1 then write the line; got %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("repaint must not use newlines (raw mode); got %q", got)
	}
}

// Reattaching to an alt-screen app (vim/htop) must re-enter the alternate
// buffer and paint its current contents, not the primary screen's history.
func TestRepaintAltScreen(t *testing.T) {
	m := newScreenModel()
	feed(m, "primary content")
	feed(m, "\x1b[?1049h", "\x1b[2J\x1b[H", "ALT SCREEN APP")

	got := string(m.repaint())
	if !strings.HasPrefix(got, escEnterAltScreen) {
		t.Errorf("alt-screen repaint must switch to the alt buffer first; got %q", got)
	}
	if !strings.Contains(got, "ALT SCREEN APP") {
		t.Errorf("repaint should paint the alt-screen contents; got %q", got)
	}
	if strings.Contains(got, "primary content") {
		t.Errorf("alt-screen repaint must not leak primary content; got %q", got)
	}
}

// The cursor's position and hidden state survive the repaint.
func TestRepaintCursorPositionAndVisibility(t *testing.T) {
	m := newScreenModel()
	feed(m, "\x1b[5;10H", "\x1b[?25l") // CUP row 5 col 10, then hide cursor

	got := string(m.repaint())
	if !strings.Contains(got, "\x1b[5;10H") {
		t.Errorf("repaint should restore the cursor to row 5 col 10; got %q", got)
	}
	if !strings.HasSuffix(got, escHideCursor) {
		t.Errorf("repaint should end with the cursor hidden; got %q", got)
	}
}

// Repaint is bounded to the viewport regardless of session length: feeding
// far more lines than the grid has rows still paints at most one positioned
// row per grid row, never the whole scrollback history.
func TestRepaintBoundedToViewport(t *testing.T) {
	m := newScreenModel()
	for i := range 1000 {
		feed(m, "line\r\n")
		_ = i
	}
	got := string(m.repaint())

	if n := strings.Count(got, "\x1b[1;1H"); n != 1 {
		t.Errorf("repaint should home exactly once; got %d in %q", n, got)
	}
	// At most rows positioned writes (CSI <row>;1H) — one per grid row.
	positioned := strings.Count(got, ";1H")
	if positioned > vt100Rows+1 { // +1 for the final cursor-restore CUP
		t.Errorf("repaint painted %d positioned rows, exceeds viewport %d", positioned, vt100Rows)
	}
}

func newScreenExec() *Exec {
	ctx, cancel := context.WithCancel(context.Background())
	return &Exec{
		ctx:       ctx,
		ctxCancel: cancel,
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		id:        "screen-test",
		ptyPipes:  &ptyPipes{},
	}
}

func TestExecScreenshotNotRunning(t *testing.T) {
	sr := newScreenExec()
	if _, err := sr.Screenshot(&api.ScreenshotArgs{}); err == nil {
		t.Errorf("Screenshot before startPty should error; screen is nil")
	}
}

func TestExecScreenshotDelegatesToModel(t *testing.T) {
	sr := newScreenExec()
	screen := newScreenModel()
	feed(screen, "live grid")
	sr.ptyPipesMu.Lock()
	sr.ptyPipes.screen = screen
	sr.ptyPipesMu.Unlock()

	res, err := sr.Screenshot(&api.ScreenshotArgs{})
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	if !strings.Contains(res.Text, "live grid") {
		t.Errorf("Exec.Screenshot should return the model snapshot; got %q", res.Text)
	}
}

// resize must keep the screen grid in step with the PTY winsize so a
// Screenshot reports the dimensions the child program draws for. (Exec.Resize
// forwards to this after pty.Setsize.)
func TestScreenModelResize(t *testing.T) {
	m := newScreenModel()
	m.resize(100, 40)
	if snap := m.snapshot(); snap.Cols != 100 || snap.Rows != 40 {
		t.Errorf("screen grid should follow resize; got %dx%d want 100x40", snap.Cols, snap.Rows)
	}
	// A zero/negative dimension is ignored rather than collapsing the grid.
	m.resize(0, 0)
	if snap := m.snapshot(); snap.Cols != 100 || snap.Rows != 40 {
		t.Errorf("resize with zero dims should be a no-op; got %dx%d", snap.Cols, snap.Rows)
	}
}
