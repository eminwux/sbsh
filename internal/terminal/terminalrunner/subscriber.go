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
	"log/slog"
	"net"
	"sync"
	"time"
)

// subscriberWriteTimeout bounds how long the drain goroutine will block
// in a single conn.Write on a hung peer. Without this, a peer that stops
// reading but never closes would pin the drain goroutine indefinitely
// and prevent the lagged sentinel from ever being delivered or detach
// from firing. var (not const) so tests can shorten it.
var subscriberWriteTimeout = 5 * time.Second

// defaultSubscriberBufferBytes bounds how far a Subscribe stream can fall
// behind the PTY reader before the subscriber is disconnected. A slow or
// abandoned subscriber must never be able to stall the shared fan-out.
const defaultSubscriberBufferBytes = 1 << 20 // 1 MiB

// subscriberBackpressureStall bounds how long a single overflowing Write may
// pace the producer on behalf of a backpressure-mode (interactive) attacher
// before giving up and dropping the attacher on lag. A *live* interactive
// reader frees ring space within microseconds, so its overflowing Writes
// return well inside this budget and the producer is throttled to terminal
// speed (issue #312). A *paused or abandoned* attacher frees nothing, so its
// Write trips the budget once, drops the attacher, and frees the producer —
// the bounded transient that keeps the #217 invariant intact (a stuck
// attacher must not head-of-line-block the capture sink and sibling
// attachers). Generous enough not to false-drop a momentarily slow live
// terminal; var (not const) so tests can shorten it.
//
//nolint:gochecknoglobals // test-tunable knob, mirrors subscriberWriteTimeout
var subscriberBackpressureStall = 2 * time.Second

// laggedNotice is appended to a subscriber stream when the ring overflows
// so a caller can distinguish disconnection from a clean EOF. The \r\n
// framing is intentional: it renders cleanly in terminal mode and the
// client-side needle (cmd/sb/read) matches only the bracketed message so
// framing can evolve without breaking detection.
var laggedNotice = []byte("\r\n[sbsh: subscriber lagged, disconnecting]\r\n")

// subscriberWriter absorbs PTY output from the multiwriter into a bounded
// in-memory ring and forwards it to an external net.Conn on a dedicated
// goroutine.
//
// Two fan-out policies share this type, selected per consumer class via
// enableBackpressure (issue #312):
//
//   - Passive observers (Subscribe / `sb read`, the capture writer, the
//     vt-parser screen model) keep backpressure==false: the Write path never
//     blocks, and on overflow the subscriber is marked lagged, detached, and
//     its conn is closed. A passive observer must never stall the session.
//
//   - The interactive attacher (connections.go) sets backpressure==true: on
//     overflow Write *paces the producer* to the attacher's drain rate
//     instead of dropping it, so the attached terminal flow-controls the
//     shell exactly as a real PTY does — slow but never dropped. The pacing
//     is bounded by subscriberBackpressureStall so a stuck attacher still
//     falls through to drop-on-lag and cannot head-of-line-block the shared
//     fan-out (the #217 invariant).
//
// #217 (f6b3f4f) collapsed both classes onto the single drop-on-lag path,
// which regressed the interactive firehose case (`cat /dev/urandom | hexdump`
// disconnected the client); the split must not be silently re-merged again.
type subscriberWriter struct {
	mu           sync.Mutex
	cond         *sync.Cond
	buf          bytes.Buffer
	maxBytes     int
	closed       bool
	lagged       bool
	backpressure bool
	conn         net.Conn
	onDetach     func()
	logger       *slog.Logger
}

func newSubscriberWriter(conn net.Conn, maxBytes int, onDetach func(), logger *slog.Logger) *subscriberWriter {
	if maxBytes <= 0 {
		maxBytes = defaultSubscriberBufferBytes
	}
	s := &subscriberWriter{
		maxBytes: maxBytes,
		conn:     conn,
		onDetach: onDetach,
		logger:   logger,
	}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// enableBackpressure switches this writer to the interactive fan-out policy:
// on ring overflow Write paces the producer until the drain frees space
// instead of dropping the attacher on lag. Call before the writer is
// registered in the fan-out. See the subscriberWriter doc and issue #312 for
// why interactive attach uses backpressure while passive subscribe does not.
func (s *subscriberWriter) enableBackpressure() {
	s.mu.Lock()
	s.backpressure = true
	s.mu.Unlock()
}

// Write is called by DynamicMultiWriter on the PTY reader goroutine. It
// enqueues bytes into the bounded ring. In passive (drop-on-lag) mode it
// returns immediately and, when the ring would exceed maxBytes, marks the
// subscriber lagged+closed and signals the drain; Run owns the conn lifecycle
// and will flush any buffered bytes, emit laggedNotice, and close the conn.
//
// In backpressure mode (interactive attach, issue #312) an overflowing Write
// instead waits for the drain to free space — pacing the producer to the
// attacher's terminal speed — and only falls through to the lagged path if
// the attacher makes no progress within subscriberBackpressureStall (a stuck
// or abandoned peer), so it can never head-of-line-block the fan-out.
//
// Write always reports success to the fan-out so one bad subscriber cannot
// abort output to other attachers or the capture file.
func (s *subscriberWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return len(p), nil
	}
	if s.buf.Len()+len(p) > s.maxBytes {
		// Backpressure mode: pace the producer to the live attacher's drain
		// rate. waitForRoom returns true once the drain has freed enough
		// space (throttle), or false if the writer closed or the stall
		// budget elapsed with no progress (drop the stuck attacher below).
		if s.backpressure && s.waitForRoom(len(p)) {
			_, _ = s.buf.Write(p)
			s.cond.Broadcast()
			return len(p), nil
		}
		if s.closed {
			return len(p), nil
		}
		s.lagged = true
		s.closed = true
		s.cond.Broadcast()
		return len(p), nil
	}
	_, _ = s.buf.Write(p)
	s.cond.Broadcast()
	return len(p), nil
}

// waitForRoom blocks (with s.mu held) until the ring has space for need bytes
// or the per-write stall budget elapses without the drain making progress.
// Returns true when space is available — the caller enqueues, pacing the
// producer to the drain — and false when the writer was closed or the budget
// elapsed (the caller drops the attacher on lag, freeing the producer). Only
// reached in backpressure mode. A one-shot timer broadcasts at the deadline
// so a producer parked on a stuck (non-reading) peer wakes even though the
// drain itself is blocked in conn.Write; for a live reader the drain's own
// post-dequeue Broadcast wakes the producer long before the timer fires.
func (s *subscriberWriter) waitForRoom(need int) bool {
	deadline := time.Now().Add(subscriberBackpressureStall)
	timer := time.AfterFunc(subscriberBackpressureStall, func() {
		s.mu.Lock()
		s.cond.Broadcast()
		s.mu.Unlock()
	})
	defer timer.Stop()
	for {
		if s.closed {
			return false
		}
		if s.buf.Len()+need <= s.maxBytes {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		s.cond.Wait()
	}
}

// seedReplay pre-loads the ring with the capture replay so the single
// drain goroutine emits it ahead of any live PTY output, and grows the
// byte bound by the replay size so a >1 MiB capture cannot make the first
// live Write trip the lagged path and disconnect a freshly attached
// client. Must be called before the writer is registered in the fan-out
// (i.e. before any concurrent Write) and before Run starts. This keeps the
// drain goroutine the sole writer of conn — its per-write SetWriteDeadline
// can never race a large initial replay written on a second goroutine
// (issue #299). A zero-length replay is a no-op, leaving the default bound.
func (s *subscriberWriter) seedReplay(b []byte) {
	if len(b) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.buf.Write(b)
	// Preserve the original headroom for live output on top of the seeded
	// replay; without this the first live Write would overflow the bound.
	s.maxBytes += len(b)
}

// Close requests a graceful shutdown. It signals drain so any pending
// bytes are flushed to the conn; drain itself closes the conn on exit.
// Safe to call repeatedly.
func (s *subscriberWriter) Close() error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.cond.Broadcast()
	}
	s.mu.Unlock()
	return nil
}

// Run drains the ring to the conn on a dedicated goroutine. It exits
// when the conn is closed, the peer goes away, or Close is called.
// Always invokes onDetach on exit so the subscriber is removed from
// the fan-out exactly once. A per-write deadline bounds how long a hung
// peer can stall drain — essential when Write(lagged) no longer closes
// the conn itself.
func (s *subscriberWriter) Run() {
	defer func() {
		if s.onDetach != nil {
			s.onDetach()
		}
		_ = s.conn.Close()
	}()
	for {
		s.mu.Lock()
		for s.buf.Len() == 0 && !s.closed {
			s.cond.Wait()
		}
		if s.buf.Len() == 0 && s.closed {
			lagged := s.lagged
			s.mu.Unlock()
			if lagged {
				_ = s.conn.SetWriteDeadline(time.Now().Add(subscriberWriteTimeout))
				if _, err := s.conn.Write(laggedNotice); err != nil {
					s.logger.Debug("subscriber lagged-notice write failed", "err", err)
				}
			}
			return
		}
		chunk := make([]byte, s.buf.Len())
		_, _ = s.buf.Read(chunk)
		// Wake any producer parked in waitForRoom now that the ring has
		// drained — this is the throttle release for backpressure mode and a
		// harmless re-check for passive mode (no producer ever parks there).
		s.cond.Broadcast()
		s.mu.Unlock()

		_ = s.conn.SetWriteDeadline(time.Now().Add(subscriberWriteTimeout))
		if _, err := s.conn.Write(chunk); err != nil {
			s.logger.Debug("subscriber conn write failed; closing", "err", err)
			s.mu.Lock()
			s.closed = true
			// Broadcast so a backpressure producer parked on this now-dead
			// peer unblocks immediately instead of waiting out its stall
			// budget; it will take the drop-on-lag path on wake.
			s.cond.Broadcast()
			s.mu.Unlock()
			return
		}
	}
}
