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
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
	"github.com/hinshun/vt10x"
)

// lockedBuffer is a concurrency-safe capture sink.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}

func (b *lockedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

// overlayRig wires an Exec's fan-out the way startPty does — capture sink
// first, then the screen model — without a PTY. Live output is injected
// with rig.out; clients are registered without a drain goroutine so a test
// reads their rings synchronously (drain) and nothing races the assertions.
type overlayRig struct {
	sr      *Exec
	mw      *DynamicMultiWriter
	screen  *screenModel
	capture *lockedBuffer
}

func newOverlayRig(t *testing.T) *overlayRig {
	t.Helper()
	sr := newWiredExec(t)
	capture := &lockedBuffer{}
	screen := newScreenModel()
	mw := sr.ptyPipes.multiOutW
	mw.Add(capture)
	mw.Add(screen)
	sr.ptyPipesMu.Lock()
	sr.ptyPipes.screen = screen
	sr.ptyPipesMu.Unlock()
	return &overlayRig{sr: sr, mw: mw, screen: screen, capture: capture}
}

func (r *overlayRig) out(t *testing.T, chunks ...string) {
	t.Helper()
	for _, c := range chunks {
		if _, err := r.mw.Write([]byte(c)); err != nil {
			t.Fatalf("fan-out write: %v", err)
		}
	}
}

// rigClient is one attached client: its ring is drained by hand into raw
// (every byte it received) and term (a VT emulator standing in for the
// client's terminal).
type rigClient struct {
	id    api.ID
	c     *ioClient
	w     *subscriberWriter
	raw   bytes.Buffer
	term  *screenModel
	input *lockedBuffer
}

func (r *overlayRig) attach(t *testing.T, id string, noOverlays bool) *rigClient {
	t.Helper()
	clientSide, serverSide := net.Pipe()
	t.Cleanup(func() {
		_ = clientSide.Close()
		_ = serverSide.Close()
	})
	cid := api.ID(id)
	c := &ioClient{id: &cid, conn: serverSide, noOverlays: noOverlays, clearScreen: true, attachedAt: time.Now()}
	initial := r.sr.initialAttachPaint(c)
	w := newSubscriberWriter(serverSide, defaultSubscriberBufferBytes, nil, r.sr.logger)
	w.enableBackpressure()
	w.seedReplay(initial)
	if !noOverlays {
		w.overlay = newClientOverlay(r.screen, r.mw.Quiesce)
		w.overlay.seed(initial)
	}
	input := &lockedBuffer{}
	c.outWriter = w
	c.gate = &inputGate{w: input}
	r.sr.addClient(c)
	r.mw.Add(w)
	rc := &rigClient{id: cid, c: c, w: w, term: newScreenModel(), input: input}
	rc.drain()
	return rc
}

// drain moves everything queued for the client into raw and term.
func (rc *rigClient) drain() []byte {
	rc.w.mu.Lock()
	b := append([]byte(nil), rc.w.buf.Bytes()...)
	rc.w.buf.Reset()
	rc.w.cond.Broadcast()
	rc.w.mu.Unlock()
	rc.raw.Write(b)
	_, _ = rc.term.Write(b)
	return b
}

func (rc *rigClient) queued() int {
	rc.w.mu.Lock()
	defer rc.w.mu.Unlock()
	return rc.w.buf.Len()
}

type cellPos struct{ x, y int }

// screenDiff compares two emulated screens cell by cell (rune, colors and
// text style) plus the cursor position, ignoring cells inside skip. It
// returns "" when they match.
func screenDiff(a, b *screenModel, skip *toastBox) string {
	a.vt.Lock()
	defer a.vt.Unlock()
	b.vt.Lock()
	defer b.vt.Unlock()
	ac, ar := a.vt.Size()
	bc, br := b.vt.Size()
	if ac != bc || ar != br {
		return fmt.Sprintf("size %dx%d vs %dx%d", ac, ar, bc, br)
	}
	inSkip := func(p cellPos) bool {
		return skip != nil && p.x >= skip.left && p.x < skip.left+skip.width &&
			p.y >= skip.top && p.y < skip.top+skip.height
	}
	norm := func(g vt10x.Glyph) vt10x.Glyph {
		if g.Char == 0 {
			g.Char = ' '
		}
		g.Mode &= vtStyleMask
		return g
	}
	for y := range ar {
		for x := range ac {
			if inSkip(cellPos{x, y}) {
				continue
			}
			ga, gb := norm(a.vt.Cell(x, y)), norm(b.vt.Cell(x, y))
			if ga != gb {
				return fmt.Sprintf("cell (%d,%d): %q fg=%d bg=%d mode=%#x vs %q fg=%d bg=%d mode=%#x",
					x, y, ga.Char, ga.FG, ga.BG, ga.Mode, gb.Char, gb.FG, gb.BG, gb.Mode)
			}
		}
	}
	ca, cb := a.vt.Cursor(), b.vt.Cursor()
	if ca.X != cb.X || ca.Y != cb.Y {
		return fmt.Sprintf("cursor (%d,%d) vs (%d,%d)", ca.X, ca.Y, cb.X, cb.Y)
	}
	if a.vt.CursorVisible() != b.vt.CursorVisible() {
		return fmt.Sprintf("cursor visible %v vs %v", a.vt.CursorVisible(), b.vt.CursorVisible())
	}
	if (a.vt.Mode()&vt10x.ModeAltScreen != 0) != (b.vt.Mode()&vt10x.ModeAltScreen != 0) {
		return "alt-screen mode differs"
	}
	return ""
}

// penDiff compares the current pens (SGR state for the next character).
func penDiff(a, b *screenModel) string {
	a.vt.Lock()
	defer a.vt.Unlock()
	b.vt.Lock()
	defer b.vt.Unlock()
	pa, pb := a.vt.Cursor().Attr, b.vt.Cursor().Attr
	if pa.FG != pb.FG || pa.BG != pb.BG || pa.Mode&vtStyleMask != pb.Mode&vtStyleMask {
		return fmt.Sprintf("pen fg=%d bg=%d mode=%#x vs fg=%d bg=%d mode=%#x",
			pa.FG, pa.BG, pa.Mode, pb.FG, pb.BG, pb.Mode)
	}
	return ""
}

// freshRedraw is the screen a client attaching now would get.
func (r *overlayRig) freshRedraw() *screenModel {
	m := newScreenModel()
	cols, rows := r.screen.size()
	m.resize(cols, rows)
	_, _ = m.Write(r.screen.repaint(true))
	return m
}

func (r *overlayRig) toastBoxFor(t *testing.T, toast Toast) *toastBox {
	t.Helper()
	cols, rows := r.screen.size()
	toast.Lines = sanitizeToastLines(toast.Lines)
	box, _, ok := layoutToast(&toast, cols, rows)
	if !ok {
		t.Fatal("toast does not fit")
	}
	return &box
}

const styledScreen = "\x1b[1mbold\x1b[0m \x1b[4munder\x1b[0m \x1b[7mreverse\x1b[0m " +
	"\x1b[31;42mcolor\x1b[0m \x1b[1;7;34mmixed\x1b[0m\r\nplain line\r\n"

// AC: without any overlay call, an overlay-capable client receives exactly
// the bytes it would without the feature — the seed paint, then the live
// stream verbatim, including chunks split mid-sequence.
func TestOverlayNoCallsByteIdentical(t *testing.T) {
	r := newOverlayRig(t)
	r.out(t, "before attach\r\n")
	with := r.attach(t, "with", false)
	without := r.attach(t, "without", true)
	if !bytes.Equal(with.raw.Bytes(), without.raw.Bytes()) {
		t.Fatalf("seed paints differ")
	}
	seed := len(with.raw.Bytes())

	live := []string{"\x1b[3", "1mred\x1b[0m", "\xe2\x94", "\x80 box\r\n", "\x1b]0;title", "\x07done"}
	r.out(t, live...)
	with.drain()
	without.drain()
	want := strings.Join(live, "")
	if got := string(with.raw.Bytes()[seed:]); got != want {
		t.Errorf("overlay-capable client got %q, want %q", got, want)
	}
	if got := string(without.raw.Bytes()[seed:]); got != want {
		t.Errorf("opted-out client got %q, want %q", got, want)
	}
}

// AC: an overlay never changes the capture, Screenshot or the workload's
// input. The same stream is run with and without a toast and a takeover
// interleaved; capture bytes and the screenshot must be identical.
func TestOverlayNeverTouchesCaptureScreenshotOrInput(t *testing.T) {
	stream := []string{styledScreen, "line 3\r\n", "\x1b[2;5H\x1b[32mgreen", "\x1b[10;1Hrow ten\r\n", "tail"}

	capA, shotA, inA := runStream(t, stream, nil)

	var tk *Takeover
	capB, shotB, inB := runStream(t, stream, map[int]func(r *overlayRig){
		1: func(r *overlayRig) { mustToast(t, r, Toast{Lines: []string{"hello"}}) },
		3: func(r *overlayRig) {
			tk = mustBegin(t, r, Target{All: true}, TakeoverOptions{})
			mustFrame(t, tk, "\x1b[2J\x1b[Hframe")
		},
		4: func(*overlayRig) { mustEnd(t, tk) },
	})

	if !bytes.Equal(capA, capB) {
		t.Errorf("capture differs with overlays:\n%q\nvs\n%q", capA, capB)
	}
	if !reflect.DeepEqual(shotA, shotB) {
		t.Errorf("screenshot differs with overlays:\n%+v\nvs\n%+v", shotA, shotB)
	}
	if len(inA) != 0 || len(inB) != 0 {
		t.Errorf("overlays wrote workload input: %q / %q", inA, inB)
	}
}

// runStream feeds stream to a fresh rig with one attached client, running
// before[i] ahead of chunk i, and returns the capture, the screenshot and
// the workload input.
func runStream(
	t *testing.T,
	stream []string,
	before map[int]func(r *overlayRig),
) ([]byte, *api.ScreenshotResult, []byte) {
	t.Helper()
	r := newOverlayRig(t)
	c := r.attach(t, "c", false)
	for i, chunk := range stream {
		if hook := before[i]; hook != nil {
			hook(r)
		}
		r.out(t, chunk)
	}
	c.drain()
	return r.capture.Bytes(), r.screen.snapshot(), c.input.Bytes()
}

func mustToast(t *testing.T, r *overlayRig, toast Toast) ToastID {
	t.Helper()
	id, err := r.sr.ShowToast(context.Background(), Target{All: true}, toast)
	if err != nil {
		t.Fatalf("ShowToast: %v", err)
	}
	return id
}

func mustBegin(t *testing.T, r *overlayRig, target Target, opts TakeoverOptions) *Takeover {
	t.Helper()
	tk, err := r.sr.BeginTakeover(context.Background(), target, opts)
	if err != nil {
		t.Fatalf("BeginTakeover: %v", err)
	}
	return tk
}

func mustFrame(t *testing.T, tk *Takeover, frame string) {
	t.Helper()
	if err := tk.Write([]byte(frame)); err != nil {
		t.Fatalf("Takeover.Write: %v", err)
	}
}

func mustEnd(t *testing.T, tk *Takeover) {
	t.Helper()
	if err := tk.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
}

// expectScreen fails the test when the client's screen (outside skip) or,
// with pen, its pen differs from want.
func expectScreen(t *testing.T, label string, got, want *screenModel, skip *toastBox, pen bool) {
	t.Helper()
	if d := screenDiff(got, want, skip); d != "" {
		t.Fatalf("%s: %s", label, d)
	}
	if !pen {
		return
	}
	if d := penDiff(got, want); d != "" {
		t.Fatalf("%s: %s", label, d)
	}
}

// AC: while a client is held the workload is never slowed. 10 MB written
// during a hold, with the held client never draining (a paused terminal),
// must not block the producer; a draining sibling and the capture receive
// every byte.
func TestTakeoverHoldNeverSlowsWorkload(t *testing.T) {
	r := newOverlayRig(t)
	held := r.attach(t, "held", false)

	// The sibling drains for real through its conn.
	sibClient, sibServer := net.Pipe()
	t.Cleanup(func() { _ = sibClient.Close(); _ = sibServer.Close() })
	sid := api.ID("sibling")
	sib := &ioClient{id: &sid, conn: sibServer, attachedAt: time.Now()}
	sw := newSubscriberWriter(sibServer, defaultSubscriberBufferBytes, nil, r.sr.logger)
	sw.enableBackpressure()
	sw.overlay = newClientOverlay(r.screen, r.mw.Quiesce)
	sib.outWriter = sw
	r.sr.addClient(sib)
	r.mw.Add(sw)
	go sw.Run()
	var sibGot int64
	sibDone := make(chan struct{})
	go func() {
		defer close(sibDone)
		n, _ := io.Copy(io.Discard, sibClient)
		sibGot = n
	}()

	tk, err := r.sr.BeginTakeover(context.Background(), Target{Only: []api.ID{"held"}}, TakeoverOptions{})
	if err != nil {
		t.Fatalf("BeginTakeover: %v", err)
	}
	before := held.queued()

	// The bound is per Write: a held client that buffered live output would
	// overflow its 1 MiB ring and park one Write for the full pacing budget
	// before being dropped. Total wall time is dominated by the screen
	// model and the sibling, so it is not the signal.
	const total = 10 << 20
	chunk := bytes.Repeat([]byte("0123456789abcdef\r\n"), 455) // ~8 KiB, ends at ground
	var slowest time.Duration
	written := 0
	for written < total {
		n := min(len(chunk), total-written)
		start := time.Now()
		if _, err = r.mw.Write(chunk[:n]); err != nil {
			t.Fatalf("write: %v", err)
		}
		slowest = max(slowest, time.Since(start))
		written += n
	}
	if slowest >= subscriberBackpressureStall {
		t.Errorf(
			"a Write during the hold took %v, at or beyond the pacing bound %v",
			slowest,
			subscriberBackpressureStall,
		)
	}
	if got := held.queued(); got != before {
		t.Errorf("held client's ring grew from %d to %d bytes: live output was buffered", before, got)
	}
	held.w.mu.Lock()
	lagged, closed := held.w.lagged, held.w.closed
	held.w.mu.Unlock()
	if lagged || closed {
		t.Errorf("held client was dropped (lagged=%v closed=%v)", lagged, closed)
	}
	if got := r.capture.Len(); got != total {
		t.Errorf("capture got %d bytes, want %d", got, total)
	}

	// The paused held client's ring is now filled with frames. End's restore
	// runs under Quiesce, so it must not wait for ring space (that would park
	// the whole fan-out for the pacing budget): the client is dropped as
	// lagged instead, and a workload Write during End is not slowed.
	held.w.mu.Lock()
	room := held.w.maxBytes - held.w.buf.Len()
	held.w.mu.Unlock()
	mustFrame(t, tk, strings.Repeat("x", room))
	if got := held.queued(); got != room+before {
		t.Fatalf("held ring holds %d bytes after the frames, want %d", got, room+before)
	}
	stop := make(chan struct{})
	writerDone := make(chan time.Duration)
	extra := 0
	go func() {
		var slow time.Duration
		for {
			select {
			case <-stop:
				writerDone <- slow
				return
			default:
			}
			start := time.Now()
			_, _ = r.mw.Write(chunk)
			slow = max(slow, time.Since(start))
			extra += len(chunk)
		}
	}()
	start := time.Now()
	err = tk.End()
	endTook := time.Since(start)
	close(stop)
	slowDuringEnd := <-writerDone
	if err != nil {
		t.Fatalf("End: %v", err)
	}
	if endTook >= subscriberBackpressureStall {
		t.Errorf(
			"End with a full held ring took %v, at or beyond the pacing bound %v",
			endTook,
			subscriberBackpressureStall,
		)
	}
	if slowDuringEnd >= subscriberBackpressureStall {
		t.Errorf(
			"a Write during End took %v, at or beyond the pacing bound %v",
			slowDuringEnd,
			subscriberBackpressureStall,
		)
	}
	held.w.mu.Lock()
	lagged = held.w.lagged
	held.w.mu.Unlock()
	if !lagged {
		t.Errorf("held client with a full ring was not dropped as lagged by the restore")
	}
	_ = sw.Close()
	<-sibDone
	if want := int64(total + extra); sibGot != want {
		t.Errorf("sibling got %d bytes, want %d", sibGot, want)
	}
}

// ShowToast never blocks on a client: a paused client whose ring is full is
// dropped as lagged instead of parking the fan-out under Quiesce.
func TestShowToastNeverBlocksOnFullRing(t *testing.T) {
	r := newOverlayRig(t)
	full := r.attach(t, "full", false)
	other := r.attach(t, "other", false)

	full.w.mu.Lock()
	_, _ = full.w.buf.WriteString(strings.Repeat("x", full.w.maxBytes-full.w.buf.Len()))
	full.w.mu.Unlock()

	start := time.Now()
	mustToast(t, r, Toast{Lines: []string{"hello"}})
	if took := time.Since(start); took >= subscriberBackpressureStall {
		t.Errorf(
			"ShowToast took %v with a full client ring, at or beyond the pacing bound %v",
			took,
			subscriberBackpressureStall,
		)
	}
	full.w.mu.Lock()
	lagged := full.w.lagged
	full.w.mu.Unlock()
	if !lagged {
		t.Errorf("client with a full ring was not dropped as lagged")
	}
	other.drain()
	if !bytes.Contains(other.raw.Bytes(), []byte("hello")) {
		t.Errorf("the other client did not get the toast")
	}
}

// AC: after End, the held client's screen equals a fresh attach redraw of
// the current model, styles included — whatever the frames did, and
// whatever the workload drew during the hold. attachedOut is sent live after
// the client attached and before the takeover; afterOut chunks are sent one
// by one after the resume and the client must track the model after each.
func TestTakeoverEndRestoresScreen(t *testing.T) {
	cases := []struct {
		name        string
		before      string
		attachedOut string
		frames      string
		duringOut   string
		afterOut    []string
		// lossyRedraw marks screens a fresh attach redraw cannot reproduce
		// (it trims styled trailing blanks, as on main); the held client is
		// then compared with the model only.
		lossyRedraw bool
	}{
		{
			name:      "normal screen",
			before:    styledScreen,
			frames:    "\x1b[2J\x1b[H\x1b[1;33;44m NOTICE \x1b[5;5H\x1b[?25l",
			duringOut: "\x1b[3;1H\x1b[4;35mstyled during hold\x1b[0m\x1b[1m",
		},
		{
			name:      "frames on alt screen",
			before:    styledScreen,
			frames:    "\x1b[?1049h\x1b[2J\x1b[7manimation\x1b[0m",
			duringOut: "more\r\n",
		},
		{
			name:      "workload enters alt screen during hold",
			before:    styledScreen,
			frames:    "\x1b[2Jframe",
			duringOut: "\x1b[?1049h\x1b[H\x1b[7mtui\x1b[0m\x1b[5;10H",
			// The workload quits the TUI: the client's normal buffer still
			// holds the frames and must be repainted from the model.
			afterOut: []string{"\x1b[?1049l", "back"},
		},
		{
			// The restore must not overwrite the workload's DECSC slot: the
			// workload saves the cursor on the normal screen, switches to
			// the alt screen without saving (?1047) during the hold, then
			// leaves it and restores the cursor after the resume.
			name:        "restore keeps the saved-cursor slot",
			before:      styledScreen,
			attachedOut: "\x1b[2;4H\x1b7\x1b[6;1H",
			frames:      "\x1b[2Jframe\x1b[10;10H",
			duringOut:   "\x1b[?1047h\x1b[Htui",
			afterOut:    []string{"\x1b[?1047l", "\x1b8", "x"},
		},
		{
			// Trailing blanks that carry a pen — a full-width reverse-video
			// title bar, a colored background — were on the client's screen
			// before the takeover and must be back after it.
			name:   "styled trailing blanks",
			before: styledScreen,
			attachedOut: "\x1b[6;1H\x1b[7m title" + strings.Repeat(" ", vt100Cols-len(" title")) + "\x1b[0m" +
				"\x1b[7;1H\x1b[44m" + strings.Repeat(" ", vt100Cols/2) + "\x1b[0m\x1b[9;1H",
			frames:      "\x1b[2Jframe",
			lossyRedraw: true,
		},
		{
			name:      "pending wrap",
			before:    styledScreen,
			frames:    "frame",
			duringOut: "\x1b[24;1H" + strings.Repeat("w", vt100Cols),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newOverlayRig(t)
			r.out(t, tc.before)
			c := r.attach(t, "c", false)
			if tc.attachedOut != "" {
				r.out(t, tc.attachedOut)
				c.drain()
				expectScreen(t, "client vs model before the takeover", c.term, r.screen, nil, true)
			}

			tk := mustBegin(t, r, Target{All: true}, TakeoverOptions{})
			mustFrame(t, tk, tc.frames)
			r.out(t, tc.duringOut)
			mustEnd(t, tk)
			c.drain()

			if !tc.lossyRedraw {
				expectScreen(t, "held client vs fresh redraw", c.term, r.freshRedraw(), nil, false)
			}
			expectScreen(t, "held client vs model", c.term, r.screen, nil, true)

			for i, chunk := range tc.afterOut {
				r.out(t, chunk)
				c.drain()
				expectScreen(t, fmt.Sprintf("after resume, chunk %d %q", i, chunk), c.term, r.screen, nil, true)
			}

			// Live output resumes and keeps both in step, pending wrap and
			// pen included.
			r.out(t, "after")
			c.drain()
			expectScreen(t, "after resume", c.term, r.screen, nil, true)
		})
	}
}

// AC: an escape code split across chunks is never interrupted; overlay
// bytes appear only at ground.
func TestOverlayWaitsForGround(t *testing.T) {
	r := newOverlayRig(t)
	c := r.attach(t, "c", false)
	r.out(t, "\x1b[3")
	c.drain()

	if _, err := r.sr.ShowToast(context.Background(), Target{All: true}, Toast{Lines: []string{"hi"}}); err != nil {
		t.Fatalf("ShowToast: %v", err)
	}
	if got := c.drain(); len(got) != 0 {
		t.Fatalf("overlay bytes injected mid-sequence: %q", got)
	}
	r.out(t, "1mred")
	got := string(c.drain())
	if !strings.HasPrefix(got, "1mred"+escSyncBegin) {
		t.Fatalf("toast should be drawn right after the sequence completed; got %q", got)
	}

	// Split UTF-8: the box must not land between the bytes of a rune.
	// Only the box erase (taken down at the previous ground boundary) may
	// precede the partial rune; nothing may follow it.
	r.out(t, "\xe2\x94")
	if got = string(c.drain()); !strings.HasSuffix(got, "\xe2\x94") {
		t.Fatalf("overlay bytes after a partial rune: %q", got)
	}
	r.out(t, "\x80")
	got = string(c.drain())
	if !strings.HasPrefix(got, "\x80"+escSyncBegin) {
		t.Fatalf("box should follow the completed rune; got %q", got)
	}
	var e escState
	e.feed(c.raw.Bytes())
	if !e.AtGround() {
		t.Errorf("client stream ends mid-sequence")
	}
}

// AC: the CAN fallback fires only after the 100 ms bound.
func TestOverlayCANAfterBound(t *testing.T) {
	r := newOverlayRig(t)
	c := r.attach(t, "c", false)
	r.out(t, "\x1b]0;never terminated")
	c.drain()

	start := time.Now()
	type result struct {
		tk  *Takeover
		err error
	}
	done := make(chan result, 1)
	go func() {
		tk, err := r.sr.BeginTakeover(context.Background(), Target{All: true}, TakeoverOptions{})
		done <- result{tk, err}
	}()

	time.Sleep(overlayGroundWait / 2)
	if got := c.drain(); bytes.Contains(got, cancelSeq) {
		t.Fatalf("CAN written before the bound: %q", got)
	}
	res := <-done
	if res.err != nil {
		t.Fatalf("BeginTakeover: %v", res.err)
	}
	if elapsed := time.Since(start); elapsed < overlayGroundWait {
		t.Errorf("hold engaged after %v, before the %v bound", elapsed, overlayGroundWait)
	}
	if got := c.drain(); !bytes.Equal(got, cancelSeq) {
		t.Errorf("want exactly CAN at the bound, got %q", got)
	}

	// The workload's OSC finally ends during the hold; End restores.
	r.out(t, "\x07visible")
	if err := res.tk.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	c.drain()
	if d := screenDiff(c.term, r.screen, nil); d != "" {
		t.Errorf("after End: %s", d)
	}
}

// A takeover ended while the workload is mid-sequence waits for ground,
// then after the bound resumes anyway without leaking the sequence's tail,
// and repaints once the tail arrived.
func TestTakeoverEndMidSequence(t *testing.T) {
	r := newOverlayRig(t)
	c := r.attach(t, "c", false)
	tk, err := r.sr.BeginTakeover(context.Background(), Target{All: true}, TakeoverOptions{})
	if err != nil {
		t.Fatalf("BeginTakeover: %v", err)
	}
	r.out(t, "\x1b[3")
	start := time.Now()
	if err = tk.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if elapsed := time.Since(start); elapsed < overlayGroundWait {
		t.Errorf("End resumed after %v, before the bound", elapsed)
	}
	r.out(t, "1mred\x1b[0m plain")
	c.drain()
	if strings.Contains(c.raw.String(), "1mred") {
		t.Errorf("the tail of a sequence the client never saw leaked: %q", c.raw.String())
	}
	if d := screenDiff(c.term, r.screen, nil); d != "" {
		t.Errorf("after tail: %s", d)
	}
	if d := penDiff(c.term, r.screen); d != "" {
		t.Errorf("after tail: %s", d)
	}
}

// AC: with a workload streaming (and scrolling) output, the client's screen
// outside the box matches the model after every chunk, its cursor matches
// the model's, and on dismissal and on TTL expiry the box area matches too.
func TestToastTracksLiveOutput(t *testing.T) {
	for _, pos := range []ToastPosition{ToastTopRight, ToastTopLeft, ToastBottomRight, ToastBottomLeft} {
		t.Run(fmt.Sprintf("position %d", pos), func(t *testing.T) {
			checkToastTracking(t, pos)
		})
	}
}

func checkToastTracking(t *testing.T, pos ToastPosition) {
	r := newOverlayRig(t)
	r.out(t, styledScreen)
	c := r.attach(t, "c", false)
	toast := Toast{Lines: []string{"another client attached", "\x1b[31mid\x07 42"}, Position: pos}
	box := r.toastBoxFor(t, toast)
	id := mustToast(t, r, toast)
	c.drain()
	expectScreen(t, "after show", c.term, r.screen, box, true)
	if !strings.Contains(c.raw.String(), "id 42") || strings.Contains(c.raw.String(), "\x1b[31mid") {
		t.Errorf("toast text should be drawn with control bytes stripped")
	}

	for i := range 60 {
		chunk := fmt.Sprintf("\x1b[3%dmline %d\x1b[0m with some text\r\n", i%8, i)
		if i%7 == 0 {
			chunk += "\x1b[1;4m" // leave a styled pen in effect
		}
		r.out(t, chunk)
		c.drain()
		expectScreen(t, fmt.Sprintf("after chunk %d", i), c.term, r.screen, box, true)
	}

	if err := r.sr.DismissToast(id); err != nil {
		t.Fatalf("DismissToast: %v", err)
	}
	c.drain()
	expectScreen(t, "after dismiss", c.term, r.screen, nil, true)
	if err := r.sr.DismissToast(id); !errors.Is(err, ErrToastNotFound) {
		t.Errorf("second dismiss: got %v, want ErrToastNotFound", err)
	}
}

// A bottom-right toast over a full-width last line that leaves the cursor in
// pending-wrap state keeps its right border: the draw does not recreate the
// wrap by rewriting the covered last cell. The erase recreates it, so the
// next live byte wraps on the client exactly as in the model, and a
// dismissal leaves the client in pending-wrap state too.
func TestToastPendingWrapKeepsBorder(t *testing.T) {
	r := newOverlayRig(t)
	cols, rows := r.screen.size()
	r.out(t, styledScreen, fmt.Sprintf("\x1b[%d;1H", rows), strings.Repeat("w", cols))
	if !wrapPending(r.screen) {
		t.Fatal("setup: model cursor is not in pending-wrap state")
	}
	c := r.attach(t, "c", false)
	toast := Toast{Lines: []string{"wrap"}, Position: ToastBottomRight}
	box := r.toastBoxFor(t, toast)

	checkBorder := func(label string) {
		t.Helper()
		want := []rune{'┐', '│', '┘'}
		for i, y := range []int{box.top, box.top + 1, box.top + box.height - 1} {
			c.term.vt.Lock()
			got := c.term.vt.Cell(cols-1, y).Char
			c.term.vt.Unlock()
			if got != want[i] {
				t.Errorf("%s: right border at (%d,%d) is %q, want %q", label, cols-1, y, got, want[i])
			}
		}
	}

	id := mustToast(t, r, toast)
	c.drain()
	checkBorder("after show")
	expectScreen(t, "after show", c.term, r.screen, box, true)

	if err := r.sr.DismissToast(id); err != nil {
		t.Fatalf("DismissToast: %v", err)
	}
	c.drain()
	expectScreen(t, "after dismiss", c.term, r.screen, nil, true)
	if !wrapPending(c.term) {
		t.Error("after dismiss: client lost the pending wrap")
	}

	mustToast(t, r, toast)
	c.drain()
	checkBorder("after re-show")
	r.out(t, "Z", "\r\nmore")
	c.drain()
	checkBorder("after live output")
	expectScreen(t, "after live output", c.term, r.screen, box, true)
}

func wrapPending(m *screenModel) bool {
	m.vt.Lock()
	defer m.vt.Unlock()
	return m.vt.Cursor().State&vtCursorWrapNext != 0
}

// Replacing sticky toasts (TTL <= 0) without dismissing the old IDs does not
// grow the registry: a toast replaced on every client that showed it is
// forgotten, and dismissing its ID reports ErrToastNotFound. A toast still
// shown on some client stays dismissable there.
func TestToastReplacedStickyPruned(t *testing.T) {
	r := newOverlayRig(t)
	r.out(t, styledScreen)
	c1 := r.attach(t, "c1", false)
	c2 := r.attach(t, "c2", false)

	registrySize := func() int {
		r.sr.overlays.mu.Lock()
		defer r.sr.overlays.mu.Unlock()
		return len(r.sr.overlays.toasts)
	}

	var first, last ToastID
	for i := range 100 {
		last = mustToast(t, r, Toast{Lines: []string{fmt.Sprintf("sticky %d", i)}})
		if i == 0 {
			first = last
		}
		if n := registrySize(); n != 1 {
			t.Fatalf("after %d replacements: registry holds %d toasts, want 1", i, n)
		}
	}
	if err := r.sr.DismissToast(first); !errors.Is(err, ErrToastNotFound) {
		t.Errorf("dismiss replaced toast: got %v, want ErrToastNotFound", err)
	}

	// Replace the last toast on c1 only: it stays registered for c2.
	only1, err := r.sr.ShowToast(context.Background(), Target{Only: []api.ID{c1.id}}, Toast{Lines: []string{"c1 only"}})
	if err != nil {
		t.Fatalf("ShowToast: %v", err)
	}
	if n := registrySize(); n != 2 {
		t.Fatalf("after a partial replace: registry holds %d toasts, want 2", n)
	}
	if err = r.sr.DismissToast(last); err != nil {
		t.Fatalf("dismiss toast still shown on c2: %v", err)
	}
	c2.drain()
	expectScreen(t, "c2 after dismiss", c2.term, r.screen, nil, true)
	c1.drain()
	expectScreen(t, "c1 keeps its toast", c1.term, r.screen,
		r.toastBoxFor(t, Toast{Lines: []string{"c1 only"}}), true)
	if err = r.sr.DismissToast(only1); err != nil {
		t.Fatalf("DismissToast: %v", err)
	}
	if n := registrySize(); n != 0 {
		t.Errorf("after dismissing everything: registry holds %d toasts, want 0", n)
	}

	// A replaced TTL toast is pruned too, and its timer stopped.
	mustToast(t, r, Toast{Lines: []string{"ttl"}, TTL: time.Hour})
	mustToast(t, r, Toast{Lines: []string{"sticky"}})
	if n := registrySize(); n != 1 {
		t.Errorf("after replacing a TTL toast: registry holds %d toasts, want 1", n)
	}
}

// AC: on TTL expiry the box area matches the model again.
func TestToastTTLExpiry(t *testing.T) {
	r := newOverlayRig(t)
	r.out(t, styledScreen)
	c := r.attach(t, "c", false)
	mustToast(t, r, Toast{Lines: []string{"bye"}, TTL: 20 * time.Millisecond})
	waitUntil(t, func() bool {
		c.drain()
		return screenDiff(c.term, r.screen, nil) == ""
	})
}

// A resize re-places the toast for the new size and keeps the screen in step.
func TestToastResize(t *testing.T) {
	r := newOverlayRig(t)
	r.out(t, styledScreen)
	c := r.attach(t, "c", false)
	toast := Toast{Lines: []string{"resize me"}, Position: ToastBottomRight}
	if _, err := r.sr.ShowToast(context.Background(), Target{All: true}, toast); err != nil {
		t.Fatalf("ShowToast: %v", err)
	}
	c.drain()

	// The client terminal resizes itself; the model follows the PTY.
	c.term.resize(100, 30)
	r.screen.resize(100, 30)
	r.sr.overlayResized()
	r.out(t, "x")
	c.drain()
	if d := screenDiff(c.term, r.screen, r.toastBoxFor(t, toast)); d != "" {
		t.Errorf("after resize: %s", d)
	}
}

// AC: during a takeover InputDrop keeps keystrokes from the workload and
// InputPass forwards them; End re-opens input.
func TestTakeoverInputGate(t *testing.T) {
	r := newOverlayRig(t)
	c := r.attach(t, "c", false)
	keys := func(s string) {
		if _, err := c.c.gate.Write([]byte(s)); err != nil {
			t.Fatalf("gate write: %v", err)
		}
	}

	keys("a")
	tk, err := r.sr.BeginTakeover(context.Background(), Target{All: true}, TakeoverOptions{})
	if err != nil {
		t.Fatalf("BeginTakeover: %v", err)
	}
	keys("b")
	if err = tk.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	keys("c")
	tk, err = r.sr.BeginTakeover(context.Background(), Target{All: true}, TakeoverOptions{Input: InputPass})
	if err != nil {
		t.Fatalf("BeginTakeover pass: %v", err)
	}
	keys("d")
	if err = tk.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if got := string(c.input.Bytes()); got != "acd" {
		t.Errorf("workload input = %q, want %q", got, "acd")
	}
}

// End to end over a real attach socket: dropped keystrokes never reach the
// workload's input pipe, and a Detach (what the client's ^]^] keystroke
// triggers) still detaches a held client.
func TestTakeoverInputDropAndDetachOverSocket(t *testing.T) {
	r := newOverlayRig(t)
	id := api.ID("sock")
	cliFD, err := r.sr.createClient(&id, attachOptions{clearScreen: true})
	if err != nil {
		t.Fatalf("createClient: %v", err)
	}
	cli := fdConn(t, cliFD)
	go func() { _, _ = io.Copy(io.Discard, cli) }()
	waitUntil(t, func() bool {
		c, ok := r.sr.getClient(id)
		if !ok {
			return false
		}
		r.sr.clientsMu.RLock()
		defer r.sr.clientsMu.RUnlock()
		return c.outWriter != nil && c.gate != nil
	})

	tk, err := r.sr.BeginTakeover(context.Background(), Target{All: true}, TakeoverOptions{})
	if err != nil {
		t.Fatalf("BeginTakeover: %v", err)
	}
	if _, err = cli.Write([]byte("dropped")); err != nil {
		t.Fatalf("client write: %v", err)
	}
	pipeInR := r.sr.ptyPipes.pipeInR
	_ = pipeInR.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	buf := make([]byte, 64)
	if n, _ := pipeInR.Read(buf); n > 0 {
		t.Errorf("keystrokes reached the workload during InputDrop: %q", buf[:n])
	}

	if err = r.sr.Detach(&id); err != nil {
		t.Fatalf("Detach during takeover: %v", err)
	}
	if _, ok := r.sr.getClient(id); ok {
		t.Errorf("client still registered after Detach")
	}
	done := make(chan error, 1)
	go func() { done <- tk.End() }()
	select {
	case endErr := <-done:
		if endErr != nil {
			t.Errorf("End after detach: %v", endErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("End blocked on a detached client")
	}
}

// AC: a client with NoOverlays receives zero overlay bytes and Clients()
// reports its opt-out.
func TestNoOverlaysClient(t *testing.T) {
	r := newOverlayRig(t)
	r.out(t, styledScreen)
	in := r.attach(t, "in", false)
	out := r.attach(t, "out", true)
	seed := out.raw.Len()

	ctx := context.Background()
	id, err := r.sr.ShowToast(ctx, Target{All: true}, Toast{Lines: []string{"hi"}})
	if err != nil {
		t.Fatalf("ShowToast: %v", err)
	}
	r.out(t, "live\r\n")
	tk, err := r.sr.BeginTakeover(ctx, Target{All: true}, TakeoverOptions{})
	if err != nil {
		t.Fatalf("BeginTakeover: %v", err)
	}
	if got := tk.Clients(); len(got) != 1 || got[0] != "in" {
		t.Errorf("takeover clients = %v, want [in]", got)
	}
	_ = tk.Write([]byte("frame"))
	r.out(t, "held\r\n")
	_ = tk.End()
	_ = r.sr.DismissToast(id)
	in.drain()
	out.drain()

	if got, want := out.raw.String()[seed:], "live\r\nheld\r\n"; got != want {
		t.Errorf("opted-out client got %q, want only live output %q", got, want)
	}
	if !strings.Contains(in.raw.String(), "frame") {
		t.Errorf("overlay client should have received the frame")
	}

	infos := r.sr.Clients()
	if len(infos) != 2 || infos[0].ID != "in" || infos[0].NoOverlays || infos[1].ID != "out" || !infos[1].NoOverlays {
		t.Errorf("Clients() = %+v", infos)
	}
}

func TestTakeoverBusyAndEnded(t *testing.T) {
	r := newOverlayRig(t)
	r.attach(t, "c", false)
	ctx := context.Background()
	tk, err := r.sr.BeginTakeover(ctx, Target{All: true}, TakeoverOptions{})
	if err != nil {
		t.Fatalf("BeginTakeover: %v", err)
	}
	if _, err = r.sr.BeginTakeover(ctx, Target{Only: []api.ID{"c"}}, TakeoverOptions{}); !errors.Is(
		err,
		ErrClientBusy,
	) {
		t.Errorf("overlapping takeover: got %v, want ErrClientBusy", err)
	}
	if cols, rows := tk.Size(); cols != vt100Cols || rows != vt100Rows {
		t.Errorf("Size = %dx%d", cols, rows)
	}
	r.screen.resize(90, 30)
	r.sr.overlayResized()
	select {
	case <-tk.Resized():
	default:
		t.Errorf("Resized not signalled")
	}
	if err = tk.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if err = tk.End(); !errors.Is(err, ErrTakeoverEnded) {
		t.Errorf("second End: got %v", err)
	}
	if err = tk.Write([]byte("x")); !errors.Is(err, ErrTakeoverEnded) {
		t.Errorf("Write after End: got %v", err)
	}
	if _, err = r.sr.ShowToast(ctx, Target{All: true}, Toast{}); !errors.Is(err, ErrEmptyToast) {
		t.Errorf("empty toast: got %v", err)
	}
}

func TestOverlaysNotRunning(t *testing.T) {
	sr := newScreenExec()
	ctx := context.Background()
	if _, err := sr.ShowToast(ctx, Target{All: true}, Toast{Lines: []string{"x"}}); !errors.Is(
		err,
		ErrOverlaysNotRunning,
	) {
		t.Errorf("ShowToast: got %v", err)
	}
	if _, err := sr.BeginTakeover(ctx, Target{All: true}, TakeoverOptions{}); !errors.Is(err, ErrOverlaysNotRunning) {
		t.Errorf("BeginTakeover: got %v", err)
	}
}

// Redraws keep text styles: a styled screen round-trips through a repaint.
func TestRepaintKeepsTextStyles(t *testing.T) {
	m := newScreenModel()
	feed(m, styledScreen, "\x1b[3mitalic\x1b[0m \x1b[5mblink\x1b[0m")
	for _, clear := range []bool{true, false} {
		client := newScreenModel()
		_, _ = client.Write(m.repaint(clear))
		if d := screenDiff(client, m, nil); d != "" {
			t.Errorf("repaint(clear=%v): %s", clear, d)
		}
	}
	snap := m.snapshot()
	for _, want := range []string{"\x1b[0;1;39;49mbold", "\x1b[0;4;39;49munder", "\x1b[0;7;39;49mreverse"} {
		if !strings.Contains(snap.ANSI, want) {
			t.Errorf("Screenshot.ANSI missing %q:\n%q", want, snap.ANSI)
		}
	}
}

// Colors-only rows keep the historical rendering byte for byte.
func TestRenderLineColorsOnlyUnchanged(t *testing.T) {
	m := newScreenModel()
	feed(m, "\x1b[31mred\x1b[0m plain \x1b[38;5;200;48;2;1;2;3mext\x1b[0m")
	_, ansi := renderLine(m.vt, vt100Cols, 0)
	want := "\x1b[31;49mred\x1b[39;49m plain \x1b[38;5;200;48;2;1;2;3mext\x1b[0m"
	if ansi != want {
		t.Errorf("renderLine = %q, want %q", ansi, want)
	}
}

func fdConn(t *testing.T, fd int) net.Conn {
	t.Helper()
	f := os.NewFile(uintptr(fd), "attach-client")
	conn, err := net.FileConn(f)
	_ = f.Close()
	if err != nil {
		t.Fatalf("FileConn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 2s")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// Overlay bytes sent with a forwarded live chunk (the toast erase before it,
// the redraw after it) keep the chunk's backpressure policy (issue #312): a
// live client showing a toast whose ring is briefly full is paced, not
// dropped, and keeps tracking the model.
func TestToastLiveClientStillPaced(t *testing.T) {
	r := newOverlayRig(t)
	r.out(t, styledScreen)
	c := r.attach(t, "c", false)
	toast := Toast{Lines: []string{"hello"}}
	mustToast(t, r, toast)
	c.drain()

	c.w.mu.Lock()
	filler := c.w.maxBytes - c.w.buf.Len()
	_, _ = c.w.buf.WriteString(strings.Repeat("x", filler))
	c.w.mu.Unlock()

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		time.Sleep(50 * time.Millisecond)
		c.w.mu.Lock()
		c.w.buf.Next(filler) // the terminal reads the filler
		c.w.cond.Broadcast()
		c.w.mu.Unlock()
	}()
	r.out(t, "more output\r\n")
	<-drained

	c.w.mu.Lock()
	lagged, closed := c.w.lagged, c.w.closed
	c.w.mu.Unlock()
	if lagged || closed {
		t.Fatalf("live toasted client was dropped (lagged=%v closed=%v), want paced", lagged, closed)
	}
	c.drain()
	expectScreen(t, "client vs model after a paced chunk", c.term, r.screen, r.toastBoxFor(t, toast), true)
}
