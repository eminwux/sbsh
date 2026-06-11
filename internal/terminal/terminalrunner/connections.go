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
	"net"
	"sync"

	"github.com/eminwux/sbsh/internal/dualcopier"
	"github.com/eminwux/sbsh/pkg/api"
)

// cleanupClient is the converging cleanup path for an interactive attach.
// It is invoked exactly once per client (guarded by detachOnce at the call
// site): on graceful Detach, on reader-side error from the dualcopier,
// and on bounded-writer lagged-detach. It is responsible for removing the
// attacher from the PTY fan-out, signalling the drain goroutine to exit
// (the drain owns the conn close on its defer), and updating
// metadata.Status.Attachers.
func (sr *Exec) cleanupClient(client *ioClient) {
	sr.logger.Info("client connection handler exiting", "client", client.id)

	// Drop this client's writer from the fan-out *before* closing it so no
	// further PTY output targets a dying ring.
	sr.ptyPipesMu.RLock()
	multiOutW := sr.ptyPipes.multiOutW
	sr.ptyPipesMu.RUnlock()

	// Snapshot outWriter under clientsMu — handleClient publishes it under
	// the same lock, so reading it unsynchronized here races the writer
	// (issue #355).
	sr.clientsMu.RLock()
	outWriter := client.outWriter
	sr.clientsMu.RUnlock()

	if multiOutW != nil && outWriter != nil {
		multiOutW.Remove(outWriter)
	}

	// Closing outWriter signals its drain goroutine to flush any pending
	// bytes and exit; the drain's deferred conn.Close releases the
	// socketpair fd. We do not close conn directly here so the drain
	// remains the sole owner of the lifecycle — double-close on a
	// *net.UnixConn just logs and is harmless, but a single owner keeps
	// the path easy to reason about.
	if outWriter != nil {
		_ = outWriter.Close()
	} else if client.conn != nil {
		// Early-failure fallback: handleClient never wired the writer,
		// so there is no drain goroutine to take the conn with it.
		if cerr := client.conn.Close(); cerr != nil {
			sr.logger.Warn("error closing client connection", "err", cerr, "client", client.id)
		}
	}

	// removeClient must happen before updateTerminalAttachers so the
	// metadata snapshot reflects the just-detached client's absence.
	// Inverting this order leaves the attacher visible until the next
	// metadata write.
	sr.removeClient(client)
	if err := sr.updateTerminalAttachers(); err != nil {
		sr.logger.Warn("failed to update metadata on client cleanup", "err", err)
	}
}

// handleClient drives an interactive attach end to end:
//
//   - WRITER side (pty -> conn): bytes from the PTY arrive via the
//     DynamicMultiWriter fan-out. To avoid one paused attacher
//     head-of-line blocking the fan-out (cf. the subscriber path), we
//     register a subscriberWriter wrapping client.conn instead of an
//     os.Pipe. The wrapper absorbs PTY output into a bounded ring;
//     a dedicated drain goroutine drains it to client.conn with a
//     per-write SetWriteDeadline so a hung peer detaches within a bounded
//     wait instead of stalling other attachers and the capture sink.
//
//   - READER side (conn -> pty): unchanged — dualcopier.RunCopier from
//     client.conn into the shared pipeInW that feeds the PTY master.
//     Keeping dualcopier here preserves its semantics for the other
//     consumer (internal/client/clientrunner/io.go).
func (sr *Exec) handleClient(client *ioClient) {
	sr.logger.Info("client connection handler started", "client", client.id)

	uc, ok := client.conn.(*net.UnixConn)
	if !ok {
		sr.logger.Error("client connection is not a UnixConn", "client", client.id)
		return
	}

	sr.ptyPipesMu.RLock()
	multiOutW := sr.ptyPipes.multiOutW
	pipeInW := sr.ptyPipes.pipeInW
	sr.ptyPipesMu.RUnlock()

	// detachOnce funnels every cleanup trigger (reader error, drain
	// lagged-detach, ctx cancel) into a single cleanupClient call.
	var detachOnce sync.Once
	detach := func() {
		detachOnce.Do(func() {
			sr.cleanupClient(client)
		})
	}

	// Snapshot the initial attach paint *before* wiring the fan-out so it can
	// be seeded ahead of any live PTY output in the ring. Seeding here (rather
	// than writing it directly to client.conn after Add) keeps the single
	// drain goroutine the sole writer of client.conn: paint and live output
	// are serialized through one ring, so the drain's per-write
	// SetWriteDeadline can never race a large initial paint written on a
	// second goroutine (issue #299).
	//
	// Default: a bounded repaint of the current screen from the vt-parser
	// model, so a long-lived terminal no longer floods every newly attached
	// client with its whole history. --full-capture (client.fullCapture) opts
	// back into replaying the entire raw capture buffer. An empty paint (no
	// screen model, or an unreadable capture) is non-fatal — the live attach
	// proceeds with no seed rather than being denied.
	initial := sr.initialAttachPaint(client)

	// WRITER: bounded ring + per-write deadline + lagged-detach. Seed the
	// initial paint into the ring (seedReplay grows the byte bound to fit it
	// so a >1 MiB full capture cannot trip the lagged path on the first live
	// Write). Adding the writer to multiOutW *before* starting Run is safe —
	// Write just enqueues into the ring until Run starts draining, and the
	// seeded paint already sits ahead of any live bytes that arrive after Add.
	//
	// Backpressure policy (issue #312): the interactive attacher is the
	// session's primary terminal — its screen *is* the output device — so on
	// ring overflow the producer is paced to this attacher's drain rate
	// (flow-control, the pre-#217 PTY behavior) rather than dropping it with a
	// lagged sentinel. enableBackpressure bounds that pacing so a paused or
	// abandoned attacher still falls through to drop-on-lag and never
	// head-of-line-blocks the capture sink or sibling attachers (the #217
	// invariant). Passive Subscribe / `sb read` consumers (subscribe.go)
	// deliberately stay on the unbounded drop-on-lag path — see subscriber.go.
	aw := newSubscriberWriter(client.conn, defaultSubscriberBufferBytes, detach, sr.logger)
	aw.enableBackpressure()
	aw.seedReplay(initial)
	// outWriter is read by the Detach RPC, cleanupClient, and Close on
	// other goroutines, so publish it under clientsMu — the same lock those
	// readers take — rather than racing the unsynchronized field write a
	// just-launched handleClient would otherwise expose to a concurrent
	// Detach (issue #355).
	sr.clientsMu.Lock()
	client.outWriter = aw
	sr.clientsMu.Unlock()
	multiOutW.Add(aw)
	go aw.Run()

	// READER: socket -> stdin. Reader-side error triggers detach so an
	// abrupt client disconnect doesn't park the drain goroutine waiting
	// for the next PTY byte to discover the broken conn.
	dc := dualcopier.NewCopier(sr.ctx, sr.logger)
	readyReader := make(chan struct{})
	go dc.RunCopier(client.conn, pipeInW, readyReader, func() {
		sr.logger.Debug("closing UnixConn read side", "client", client.id)
		_ = uc.CloseRead()
		detach()
	}, nil)

	<-readyReader

	// MANAGER: ctx cancel also funnels to detach. CopierManager's errgroup
	// arm is intentionally not given a finish func — detach above already
	// covers the reader-error path.
	go dc.CopierManager(uc, detach)

	if errAttach := sr.updateTerminalAttachers(); errAttach != nil {
		sr.logger.Warn("failed to update metadata on attach", "err", errAttach)
		return
	}
}

// initialAttachPaint returns the bytes written to a newly attached client
// before live PTY output begins. For a full-capture client it is the entire
// raw capture buffer (legacy behavior); otherwise it is a repaint of the
// current screen from the vt-parser model. A nil/empty result means "write
// nothing" — a missing capture or an absent screen model never denies the
// attach, which proceeds with live output only.
func (sr *Exec) initialAttachPaint(client *ioClient) []byte {
	if client.fullCapture {
		log, errLog := sr.readCaptureFile()
		if errLog != nil {
			sr.logger.Warn("failed to read capture file for client attach", "err", errLog)
			return nil
		}
		return log
	}

	sr.ptyPipesMu.RLock()
	screen := sr.ptyPipes.screen
	sr.ptyPipesMu.RUnlock()
	if screen == nil {
		sr.logger.Warn("no screen model for client attach repaint", "client", client.id)
		return nil
	}
	return screen.repaint(client.clearScreen)
}

func (sr *Exec) addClient(c *ioClient) {
	sr.clientsMu.Lock()
	sr.clients[*c.id] = c
	sr.clientsMu.Unlock()
}

func (sr *Exec) removeClient(c *ioClient) {
	sr.clientsMu.Lock()
	delete(sr.clients, *c.id)
	sr.clientsMu.Unlock()
}

func (sr *Exec) getClient(id api.ID) (*ioClient, bool) {
	sr.clientsMu.RLock()
	defer sr.clientsMu.RUnlock()

	c, ok := sr.clients[id]
	return c, ok
}
