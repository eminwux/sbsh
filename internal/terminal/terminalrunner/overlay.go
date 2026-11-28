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
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/eminwux/sbsh/pkg/api"
)

// Client overlays: bytes the embedder sends to attached interactive clients
// only. They never reach the capture writer, the screen model or the
// workload's input, so the model can always restore a client's screen
// afterwards. Two kinds exist:
//
//   - a toast, a small box redrawn over live output after every forwarded
//     chunk, with the cursor and pen put back to the model's state;
//   - a takeover, which holds live output for its clients (discarding it —
//     never buffering, so the workload is never slowed), lets the embedder
//     write raw frames, and ends with a full restore from the model.
//
// Overlay bytes are injected only when the client's byte stream is at
// escape-sequence ground (see escState). A switch that cannot happen at a
// chunk boundary waits for the next boundary at ground for at most
// overlayGroundWait, then aborts the client's partial sequence with CAN.
//
// Concurrency: overlay state lives in the client's subscriberWriter and is
// guarded by its mutex. Every state change happens either on the PTY reader
// goroutine inside the fan-out (onLive) or under DynamicMultiWriter.Quiesce,
// so the screen model and the client stream are always at the same chunk
// boundary when a paint is computed.

// overlayGroundWait bounds how long an overlay switch waits for the client
// stream to reach a chunk boundary at ground before forcing it with CAN. var
// (not const) so tests can shorten it.
//
//nolint:gochecknoglobals // test-tunable knob, mirrors subscriberWriteTimeout
var overlayGroundWait = 100 * time.Millisecond

// cancelSeq is written to a client to abort a partial escape sequence.
//
//nolint:gochecknoglobals // immutable; []byte has no const form
var cancelSeq = []byte{byteCAN}

var (
	// ErrOverlaysNotRunning is returned when the terminal has no live PTY
	// fan-out yet (or any more).
	ErrOverlaysNotRunning = errors.New("overlays: terminal not running")
	// ErrToastNotFound is returned by DismissToast for an unknown or
	// already-dismissed toast.
	ErrToastNotFound = errors.New("overlays: toast not found")
	// ErrTakeoverEnded is returned by Takeover.Write after End.
	ErrTakeoverEnded = errors.New("overlays: takeover ended")
	// ErrClientBusy is returned by BeginTakeover when a targeted client is
	// already held by another takeover.
	ErrClientBusy = errors.New("overlays: client already in a takeover")
	// ErrEmptyToast is returned by ShowToast for a toast with no lines.
	ErrEmptyToast = errors.New("overlays: toast has no lines")
)

// Target selects attached clients for an overlay. All selects every
// attached client; otherwise Only lists them. Except removes clients from
// either set. Unknown IDs are ignored, and clients that opted out of
// overlays are never selected. The zero Target selects nobody.
type Target struct {
	All    bool
	Only   []api.ID
	Except []api.ID
}

// ClientInfo describes an attached interactive client.
type ClientInfo struct {
	ID         api.ID
	AttachedAt time.Time
	// NoOverlays is true when the client opted out of overlays at attach;
	// it never receives overlay bytes and is never selected by a Target.
	NoOverlays bool
}

// ToastPosition is the screen corner a toast is drawn in.
type ToastPosition int

const (
	ToastTopRight ToastPosition = iota
	ToastTopLeft
	ToastBottomRight
	ToastBottomLeft
)

// ToastStyle selects the toast's colors.
type ToastStyle int

const (
	ToastInfo ToastStyle = iota
	ToastWarning
	ToastError
)

// Toast is a non-modal notice drawn in a corner box over live output.
// Lines are plain text: control characters are stripped and each rune takes
// one column. Lines that do not fit the terminal are truncated. TTL > 0
// dismisses the toast automatically; TTL <= 0 keeps it until DismissToast.
type Toast struct {
	Lines    []string
	Position ToastPosition
	TTL      time.Duration
	Style    ToastStyle
}

// ToastID identifies a shown toast.
type ToastID uint64

// InputMode decides what happens to a held client's keystrokes during a
// takeover. Keystrokes are never delivered to the embedder.
type InputMode int

const (
	// InputDrop discards the held clients' keystrokes (the default).
	InputDrop InputMode = iota
	// InputPass forwards them to the workload as usual.
	InputPass
)

// TakeoverOptions configures BeginTakeover.
type TakeoverOptions struct {
	Input InputMode
}

// overlayPhase is a client's takeover phase.
type overlayPhase uint8

const (
	phaseLive        overlayPhase = iota // live output flows to the client
	phaseHoldPending                     // takeover requested; waiting for ground
	phaseHeld                            // live output discarded; frames flow
	phaseEndPending                      // takeover ended; waiting to restore
)

// clientOverlay is the overlay state of one interactive attacher. All fields
// are guarded by the owning subscriberWriter's mutex.
type clientOverlay struct {
	screen  *screenModel
	quiesce func(func())

	// clientEsc tracks every byte enqueued to the client (live, frames and
	// overlay paints); liveEsc tracks the workload's live stream whether
	// forwarded or discarded. While live and not skipping they see the
	// same bytes.
	clientEsc escState
	liveEsc   escState

	phase  overlayPhase
	heldCh chan struct{} // closed when the hold engages (or on abort)
	liveCh chan struct{} // closed when live output resumes (or on abort)

	// skipToGround drops live bytes until liveEsc reaches ground. It is set
	// after a forced switch: the client never saw (or had CAN abort) the
	// start of the live sequence, so its tail must not reach the client.
	skipToGround bool
	// restorePending asks for a full restore at the next boundary at ground.
	restorePending bool

	toast   *Toast
	toastID ToastID
	// boxErase, when non-nil, means the toast box is on the client's screen;
	// it holds the bytes that erase it (the covered cells as the model held
	// them when the box was drawn, plus the cursor/pen state).
	boxErase []byte

	timer    *time.Timer
	timerGen uint64

	aborted bool
}

func newClientOverlay(screen *screenModel, quiesce func(func())) *clientOverlay {
	return &clientOverlay{screen: screen, quiesce: quiesce}
}

// seed feeds the initial attach paint to both trackers: the client terminal
// and the live stream are both "after the paint" when live output starts.
func (o *clientOverlay) seed(p []byte) {
	o.clientEsc.feed(p)
	o.liveEsc.feed(p)
}

// onLive handles one live chunk from the fan-out. Called with s.mu held, on
// the PTY reader goroutine, after the screen model consumed the same chunk.
func (o *clientOverlay) onLive(s *subscriberWriter, p []byte) {
	if o.phase == phaseHeld || o.phase == phaseEndPending {
		// Held: discard, never buffer (an overflowing interactive ring would
		// pace the workload). The model keeps the screen for the restore.
		o.liveEsc.feed(p)
		o.reconcile(s, false)
		return
	}
	if o.boxErase != nil {
		// Take the box down before the chunk so output that scrolls or
		// overwrites the screen moves the real cells, not the box.
		s.enqueueLocked(o.boxErase)
		o.boxErase = nil
	}
	if o.skipToGround {
		i := o.liveEsc.feedUntilGround(p)
		if i < 0 {
			return
		}
		o.skipToGround = false
		p = p[i:]
	}
	o.liveEsc.feed(p)
	s.enqueueLocked(p)
	o.reconcile(s, false)
}

// outstanding reports whether a live-phase paint is waiting for ground.
func (o *clientOverlay) outstanding() bool {
	return o.restorePending || (o.toast != nil && o.boxErase == nil)
}

// reconcile performs whatever transition or paint the client is waiting for,
// if the stream allows it now. forced is true when overlayGroundWait expired:
// the switch then happens anyway, aborting the partial sequence with CAN.
// Called with s.mu held, at a chunk boundary (in the fan-out or quiesced).
func (o *clientOverlay) reconcile(s *subscriberWriter, forced bool) {
	if s.closed || o.aborted {
		return
	}
	switch o.phase {
	case phaseHeld:
		return
	case phaseHoldPending:
		o.reconcileHold(s, forced)
		return
	case phaseEndPending:
		if !o.reconcileEnd(s, forced) {
			return
		}
	case phaseLive:
	}
	o.reconcileLive(s, forced)
}

// reconcileHold engages a pending hold once the client is at ground.
func (o *clientOverlay) reconcileHold(s *subscriberWriter, forced bool) {
	if !o.clientEsc.AtGround() {
		if !forced {
			o.armTimer(s)
			return
		}
		s.enqueueLocked(cancelSeq)
	}
	o.stopTimer()
	o.phase = phaseHeld
	// Whatever the box covered is now owned by the frames; the end restore
	// repaints everything.
	o.boxErase = nil
	o.skipToGround = false
	o.restorePending = false
	closeOnce(o.heldCh)
}

// reconcileEnd restores the client and resumes live output once the live
// stream is at a boundary at ground. It reports whether the client is live
// and ready for live-phase paints.
func (o *clientOverlay) reconcileEnd(s *subscriberWriter, forced bool) bool {
	if !o.clientEsc.AtGround() {
		// The embedder's frames left a partial sequence; no more frames are
		// coming, so abort it now.
		s.enqueueLocked(cancelSeq)
	}
	if !o.liveEsc.AtGround() {
		if !forced {
			o.armTimer(s)
			return false
		}
		// Resume anyway: drop the live sequence's tail, and repaint once it
		// completed so its effect reaches the client.
		o.skipToGround = true
		o.restorePending = true
	}
	o.stopTimer()
	s.enqueueLocked(o.screen.restorePaint(o.clientEsc.altScreen))
	o.phase = phaseLive
	closeOnce(o.liveCh)
	return !o.skipToGround
}

// reconcileLive performs pending live-phase paints: a full restore, then the
// toast box.
func (o *clientOverlay) reconcileLive(s *subscriberWriter, forced bool) {
	if !o.outstanding() {
		o.stopTimer()
		return
	}
	if o.skipToGround {
		// Waiting for the live tail; nothing can be painted until it ends.
		return
	}
	if !o.clientEsc.AtGround() {
		if !forced {
			o.armTimer(s)
			return
		}
		s.enqueueLocked(cancelSeq)
		o.skipToGround = true
		o.restorePending = true
		o.stopTimer()
		return
	}
	o.stopTimer()
	if o.restorePending {
		s.enqueueLocked(o.screen.restorePaint(o.clientEsc.altScreen))
		o.restorePending = false
		o.boxErase = nil
	}
	o.drawToastLocked(s)
}

// drawToastLocked (re)draws the active toast, taking down a previous box
// first. The client must be at ground.
func (o *clientOverlay) drawToastLocked(s *subscriberWriter) {
	if o.boxErase != nil {
		s.enqueueLocked(o.boxErase)
		o.boxErase = nil
	}
	if o.toast == nil {
		return
	}
	draw, erase, ok := o.screen.toastFrame(o.toast)
	if !ok {
		return
	}
	s.enqueueLocked(draw)
	o.boxErase = erase
}

// armTimer starts the overlayGroundWait deadline for the pending switch, if
// one is not already running.
func (o *clientOverlay) armTimer(s *subscriberWriter) {
	if o.timer != nil {
		return
	}
	o.timerGen++
	gen := o.timerGen
	o.timer = time.AfterFunc(overlayGroundWait, func() {
		o.quiesce(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if o.timer == nil || o.timerGen != gen {
				return
			}
			o.timer = nil
			o.reconcile(s, true)
		})
	})
}

func (o *clientOverlay) stopTimer() {
	if o.timer != nil {
		o.timer.Stop()
		o.timer = nil
	}
}

// abort releases waiters when the client goes away mid-transition.
func (o *clientOverlay) abort() {
	o.aborted = true
	o.stopTimer()
	closeOnce(o.heldCh)
	closeOnce(o.liveCh)
}

func closeOnce(ch chan struct{}) {
	if ch == nil {
		return
	}
	select {
	case <-ch:
	default:
		close(ch)
	}
}

// inputGate sits between an attacher's socket reader and the workload's
// input pipe. In drop mode keystrokes are discarded; otherwise they pass
// through untouched.
type inputGate struct {
	w    io.Writer
	drop atomic.Bool
}

func (g *inputGate) Write(p []byte) (int, error) {
	if g.drop.Load() {
		return len(p), nil
	}
	return g.w.Write(p)
}

// overlayRegistry is the Exec-wide overlay bookkeeping.
type overlayRegistry struct {
	mu        sync.Mutex
	nextToast ToastID
	toasts    map[ToastID]*toastEntry
	takeovers map[*Takeover]struct{}
}

type toastEntry struct {
	clients []overlayClient
	timer   *time.Timer
}

// overlayEnv snapshots what overlay operations need from a running Exec.
func (sr *Exec) overlayEnv() (*DynamicMultiWriter, *screenModel, error) {
	sr.ptyPipesMu.RLock()
	defer sr.ptyPipesMu.RUnlock()
	if sr.ptyPipes == nil || sr.ptyPipes.multiOutW == nil || sr.ptyPipes.screen == nil {
		return nil, nil, ErrOverlaysNotRunning
	}
	return sr.ptyPipes.multiOutW, sr.ptyPipes.screen, nil
}

// Clients lists the attached interactive clients, oldest first.
func (sr *Exec) Clients() []ClientInfo {
	sr.clientsMu.RLock()
	out := make([]ClientInfo, 0, len(sr.clients))
	for _, c := range sr.clients {
		out = append(out, ClientInfo{ID: *c.id, AttachedAt: c.attachedAt, NoOverlays: c.noOverlays})
	}
	sr.clientsMu.RUnlock()
	slices.SortFunc(out, func(a, b ClientInfo) int {
		if c := a.AttachedAt.Compare(b.AttachedAt); c != 0 {
			return c
		}
		return compareIDs(a.ID, b.ID)
	})
	return out
}

func compareIDs(a, b api.ID) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// overlayClient pairs an attacher with its writer, snapshotted under
// clientsMu (where handleClient publishes outWriter).
type overlayClient struct {
	c    *ioClient
	w    *subscriberWriter
	gate *inputGate
}

// targets resolves t to the attached clients that accept overlays.
func (sr *Exec) targets(t Target) []overlayClient {
	sr.clientsMu.RLock()
	defer sr.clientsMu.RUnlock()
	var out []overlayClient
	for id, c := range sr.clients {
		if c.noOverlays || c.outWriter == nil || c.outWriter.overlay == nil {
			continue
		}
		if !t.All && !slices.Contains(t.Only, id) {
			continue
		}
		if slices.Contains(t.Except, id) {
			continue
		}
		out = append(out, overlayClient{c: c, w: c.outWriter, gate: c.gate})
	}
	return out
}

// sanitizeToastLines strips control characters (C0, DEL, C1) so a toast can
// never inject escape sequences into a client's terminal.
func sanitizeToastLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		r := []rune(l)
		kept := r[:0]
		for _, c := range r {
			if unicode.IsControl(c) || c == unicode.ReplacementChar {
				continue
			}
			kept = append(kept, c)
		}
		out = append(out, string(kept))
	}
	return out
}

// ShowToast draws toast on the targeted clients and returns its ID. A
// client already showing a toast has it replaced. ctx is checked once; the
// call never blocks on a client.
func (sr *Exec) ShowToast(ctx context.Context, t Target, toast Toast) (ToastID, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(toast.Lines) == 0 {
		return 0, ErrEmptyToast
	}
	mw, _, err := sr.overlayEnv()
	if err != nil {
		return 0, err
	}
	toast.Lines = sanitizeToastLines(toast.Lines)
	clients := sr.targets(t)

	sr.overlays.mu.Lock()
	if sr.overlays.toasts == nil {
		sr.overlays.toasts = make(map[ToastID]*toastEntry)
	}
	sr.overlays.nextToast++
	id := sr.overlays.nextToast
	entry := &toastEntry{clients: clients}
	sr.overlays.toasts[id] = entry
	sr.overlays.mu.Unlock()

	mw.Quiesce(func() {
		for _, oc := range clients {
			w := oc.w
			w.mu.Lock()
			o := w.overlay
			o.toast = &toast
			o.toastID = id
			if o.phase == phaseLive && !o.skipToGround && !o.restorePending && o.clientEsc.AtGround() {
				o.drawToastLocked(w)
			} else {
				o.reconcile(w, false)
			}
			w.mu.Unlock()
		}
	})

	if toast.TTL > 0 {
		sr.overlays.mu.Lock()
		if e, ok := sr.overlays.toasts[id]; ok {
			e.timer = time.AfterFunc(toast.TTL, func() { _ = sr.DismissToast(id) })
		}
		sr.overlays.mu.Unlock()
	}
	return id, nil
}

// DismissToast removes a toast from every client still showing it,
// repainting the cells its box covered from the model.
func (sr *Exec) DismissToast(id ToastID) error {
	sr.overlays.mu.Lock()
	entry, ok := sr.overlays.toasts[id]
	if ok {
		delete(sr.overlays.toasts, id)
		if entry.timer != nil {
			entry.timer.Stop()
		}
	}
	sr.overlays.mu.Unlock()
	if !ok {
		return ErrToastNotFound
	}
	mw, _, err := sr.overlayEnv()
	if err != nil {
		return nil //nolint:nilerr // terminal gone: nothing left on any screen
	}
	mw.Quiesce(func() {
		for _, oc := range entry.clients {
			w := oc.w
			w.mu.Lock()
			o := w.overlay
			if o.toast != nil && o.toastID == id {
				o.toast = nil
				o.toastID = 0
				if o.boxErase != nil && !w.closed {
					// The box was drawn at a ground boundary and nothing has
					// been sent since, so the client is at ground.
					w.enqueueLocked(o.boxErase)
				}
				o.boxErase = nil
				o.reconcile(w, false)
			}
			w.mu.Unlock()
		}
	})
	return nil
}

// Takeover holds live output for a set of clients so the embedder can write
// full-screen frames to them. Create one with BeginTakeover; End it to
// restore the clients' screens from the model and resume live output.
type Takeover struct {
	sr      *Exec
	mw      *DynamicMultiWriter
	screen  *screenModel
	clients []overlayClient
	input   InputMode
	resized chan struct{}

	mu    sync.Mutex
	ended bool
}

// BeginTakeover holds live output for the targeted clients and returns once
// every one of them is held (at most overlayGroundWait plus scheduling) or
// ctx is done, in which case the takeover is rolled back. Fails with
// ErrClientBusy if a targeted client is already held by another takeover.
// Clients attaching later are not part of the takeover.
func (sr *Exec) BeginTakeover(ctx context.Context, t Target, opts TakeoverOptions) (*Takeover, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	mw, screen, err := sr.overlayEnv()
	if err != nil {
		return nil, err
	}
	clients := sr.targets(t)
	tk := &Takeover{
		sr:      sr,
		mw:      mw,
		screen:  screen,
		clients: clients,
		input:   opts.Input,
		resized: make(chan struct{}, 1),
	}

	var waits []chan struct{}
	busy := false
	mw.Quiesce(func() {
		if busy = anyHeld(clients); !busy {
			waits = holdClients(clients, opts.Input)
		}
	})
	if busy {
		return nil, ErrClientBusy
	}

	sr.overlays.mu.Lock()
	if sr.overlays.takeovers == nil {
		sr.overlays.takeovers = make(map[*Takeover]struct{})
	}
	sr.overlays.takeovers[tk] = struct{}{}
	sr.overlays.mu.Unlock()

	for _, ch := range waits {
		select {
		case <-ch:
		case <-ctx.Done():
			_ = tk.End()
			return nil, ctx.Err()
		}
	}
	return tk, nil
}

// anyHeld reports whether a client is already part of a takeover. Called
// quiesced.
func anyHeld(clients []overlayClient) bool {
	for _, oc := range clients {
		oc.w.mu.Lock()
		phase := oc.w.overlay.phase
		oc.w.mu.Unlock()
		if phase != phaseLive {
			return true
		}
	}
	return false
}

// holdClients requests the hold on every client and gates their input; it
// returns the channels that close once each hold engaged. Called quiesced.
func holdClients(clients []overlayClient, input InputMode) []chan struct{} {
	waits := make([]chan struct{}, 0, len(clients))
	for _, oc := range clients {
		w := oc.w
		w.mu.Lock()
		o := w.overlay
		o.phase = phaseHoldPending
		o.heldCh = make(chan struct{})
		if w.closed || o.aborted {
			closeOnce(o.heldCh)
		}
		waits = append(waits, o.heldCh)
		o.reconcile(w, false)
		w.mu.Unlock()
		if input == InputDrop && oc.gate != nil {
			oc.gate.drop.Store(true)
		}
	}
	return waits
}

// Clients returns the IDs of the clients this takeover holds.
func (tk *Takeover) Clients() []api.ID {
	ids := make([]api.ID, 0, len(tk.clients))
	for _, oc := range tk.clients {
		ids = append(ids, *oc.c.id)
	}
	return ids
}

// Size returns the terminal size frames should be drawn for.
func (tk *Takeover) Size() (int, int) {
	return tk.screen.size()
}

// Resized receives a value after the terminal was resized; read Size again
// and redraw. At most one notification is buffered.
func (tk *Takeover) Resized() <-chan struct{} {
	return tk.resized
}

// Write sends raw frame bytes to every held client, and to nothing else:
// not the capture, the screen model or the workload. Frames should leave
// modes other than the alternate screen, cursor visibility and SGR as they
// found them; End restores only those and the screen contents.
func (tk *Takeover) Write(frame []byte) error {
	tk.mu.Lock()
	defer tk.mu.Unlock()
	if tk.ended {
		return ErrTakeoverEnded
	}
	for _, oc := range tk.clients {
		w := oc.w
		w.mu.Lock()
		if w.overlay.phase == phaseHeld {
			w.enqueueLocked(frame)
		}
		w.mu.Unlock()
	}
	return nil
}

// End restores every held client's screen from the current model, resumes
// live output and keystrokes, and returns once each client resumed (at most
// overlayGroundWait plus scheduling). Calling End again returns
// ErrTakeoverEnded.
func (tk *Takeover) End() error {
	tk.mu.Lock()
	defer tk.mu.Unlock()
	if tk.ended {
		return ErrTakeoverEnded
	}
	tk.ended = true

	tk.sr.overlays.mu.Lock()
	delete(tk.sr.overlays.takeovers, tk)
	tk.sr.overlays.mu.Unlock()

	waits := make([]chan struct{}, 0, len(tk.clients))
	tk.mw.Quiesce(func() {
		for _, oc := range tk.clients {
			w := oc.w
			w.mu.Lock()
			o := w.overlay
			switch o.phase {
			case phaseHoldPending:
				// Rolled back before the hold engaged: nothing to restore.
				o.stopTimer()
				o.phase = phaseLive
				closeOnce(o.heldCh)
				o.reconcile(w, false)
			case phaseHeld:
				o.phase = phaseEndPending
				o.liveCh = make(chan struct{})
				if w.closed || o.aborted {
					closeOnce(o.liveCh)
				}
				waits = append(waits, o.liveCh)
				o.reconcile(w, false)
			case phaseLive, phaseEndPending:
			}
			w.mu.Unlock()
			if oc.gate != nil {
				oc.gate.drop.Store(false)
			}
		}
	})
	for _, ch := range waits {
		<-ch
	}
	return nil
}

// overlayResized is called after a terminal resize. Toast boxes are taken
// down by a full restore at the next ground boundary and redrawn for the new
// size (the client terminal reflowed them on its own); takeovers are told to
// redraw.
func (sr *Exec) overlayResized() {
	mw, _, err := sr.overlayEnv()
	if err != nil {
		return
	}
	clients := sr.targets(Target{All: true})

	mw.Quiesce(func() {
		for _, oc := range clients {
			w := oc.w
			w.mu.Lock()
			o := w.overlay
			if o.phase == phaseLive && (o.toast != nil || o.boxErase != nil) {
				o.boxErase = nil
				o.restorePending = true
				o.reconcile(w, false)
			}
			w.mu.Unlock()
		}
	})

	sr.overlays.mu.Lock()
	for tk := range sr.overlays.takeovers {
		select {
		case tk.resized <- struct{}{}:
		default:
		}
	}
	sr.overlays.mu.Unlock()
}
