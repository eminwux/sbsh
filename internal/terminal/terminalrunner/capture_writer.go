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
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/eminwux/sbsh/internal/capture"
	"github.com/eminwux/sbsh/internal/defaults"
	pcapture "github.com/eminwux/sbsh/pkg/capture"
)

// captureFormatOpts configures the on-disk capture format for a captureWriter.
// The zero value is raw (append-only PTY bytes — byte-exact, no per-write
// overhead). When Asciicast is true the writer emits asciicast v2: a header
// line at the top of every segment and of every run, one timed `[t,"o",data]`
// event per Write, and one `[t,"r","<cols>x<rows>"]` event per size change.
// Cols/Rows are the launch geometry.
type captureFormatOpts struct {
	Asciicast bool
	Cols      int
	Rows      int
}

// captureWriter is the always-on capture sink registered first in multiOutW.
// It writes the live segment append-only at the canonical path and, once the
// live segment crosses maxBytes, rotates: close → rename to a deterministic
// sibling (pcapture.SegmentPath) → gzip-compress the closed segment
// (capture.CompressSegment) → reopen a fresh canonical → prune oldest closed
// segments by count and total bytes. The live segment is never compressed (it
// must stay raw and seekable); only closed segments are. Readers reassemble
// and transparently decompress the full transcript via the pkg/capture
// helpers.
//
// Rotation failures never silently kill capture: the writer always restores a
// writable live segment (reopening the canonical) and logs, so a transient
// rename/prune error degrades to "rotation skipped this round", not "capture
// stops". A bare write error on the live segment is returned as before so
// DynamicMultiWriter's failed-writer eviction still applies.
type captureWriter struct {
	mu        sync.Mutex
	canonical string
	f         *os.File
	size      int64
	nextSeq   uint64

	maxBytes       int64
	retainSegments int
	retainBytes    int64

	// asciicast records the asciicast v2 format instead of raw bytes. When
	// set, every freshly opened (empty) segment gets a header line, a
	// non-empty segment reopened by a new run gets a run header, each Write
	// is emitted as one timed output event, and each size change as one
	// resize event. cols/rows are the size in effect (launch size, then the
	// latest Resize) and seed every header written; start anchors the run's
	// t=0 origin and seeds the header timestamp.
	asciicast bool
	cols      int
	rows      int
	start     time.Time

	// carry holds an incomplete trailing UTF-8 sequence (at most 3 bytes)
	// from the previous asciicast Write, prepended to the next one so a
	// character split across two PTY reads is never encoded as U+FFFD.
	carry []byte

	// headerJustWritten is set when onSegmentOpened wrote a segment header,
	// so newCaptureWriter does not follow it with a redundant run header.
	headerJustWritten bool

	// applyPerms re-applies the resolved capture mode/group to a freshly
	// opened segment path. nil leaves perms at the open default.
	applyPerms func(path string) error
	logger     *slog.Logger
}

// newCaptureWriter opens (or reopens, append-only) the canonical capture path
// and returns a rotating writer seeded from any closed segments already on
// disk, so sequence numbering resumes gap-free across restarts. In asciicast
// mode, reopening a non-empty live segment starts a new run: the clock
// restarts at t=0, so a run header with the new start time is appended before
// the first event (see writeRunHeader). applyPerms is
// invoked on the freshly opened canonical immediately, mirroring the explicit
// permissions step the non-rotating path performed after Open.
func newCaptureWriter(
	canonical string,
	logger *slog.Logger,
	applyPerms func(path string) error,
	format captureFormatOpts,
) (*captureWriter, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	f, err := os.OpenFile(canonical, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open capture file: %w", err)
	}
	if applyPerms != nil {
		if perr := applyPerms(canonical); perr != nil {
			_ = f.Close()
			return nil, perr
		}
	}
	nextSeq, err := pcapture.NextSeq(canonical)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("seed capture sequence: %w", err)
	}
	w := &captureWriter{
		canonical:      canonical,
		f:              f,
		nextSeq:        nextSeq,
		maxBytes:       defaults.CaptureSegmentMaxBytes,
		retainSegments: defaults.CaptureRetentionMaxSegments,
		retainBytes:    defaults.CaptureRetentionMaxBytes,
		asciicast:      format.Asciicast,
		cols:           format.Cols,
		rows:           format.Rows,
		start:          time.Now(),
		applyPerms:     applyPerms,
		logger:         logger,
	}
	// Seed size from the opened inode and, in asciicast mode, write the header
	// when the segment is empty (a fresh capture, or a fresh canonical after a
	// restart that pruned everything).
	if oerr := w.onSegmentOpened(); oerr != nil {
		_ = f.Close()
		return nil, oerr
	}
	// A non-empty segment here belongs to an earlier run on the same capture
	// path (a spec that reuses CaptureFile). This run's t restarts at 0, so
	// mark the boundary with a run header; otherwise times go backwards.
	if w.asciicast && w.size > 0 && !w.headerJustWritten {
		if rerr := w.writeRunHeader(); rerr != nil {
			_ = w.Close()
			return nil, rerr
		}
	}
	return w, nil
}

// writeRunHeader appends a header line with this run's start time and size to
// a non-empty live segment. If the previous run died mid-line (a torn tail),
// a newline is written first so the header is not merged into the partial
// line — which the decoder would drop, leaving this run's events under the
// previous run's header.
func (w *captureWriter) writeRunHeader() error {
	torn, err := lastByteNotNewline(w.canonical, w.size)
	if err != nil {
		return err
	}
	hdr, err := capture.EncodeHeader(w.cols, w.rows, w.start.Unix())
	if err != nil {
		return err
	}
	if torn {
		hdr = append([]byte{'\n'}, hdr...)
	}
	if _, werr := w.writeRecord(hdr); werr != nil {
		return fmt.Errorf("write asciicast run header: %w", werr)
	}
	return nil
}

// lastByteNotNewline reports whether the file at path, of the given size,
// ends in something other than '\n'.
func lastByteNotNewline(path string, size int64) (bool, error) {
	if size <= 0 {
		return false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open capture file for tail check: %w", err)
	}
	defer func() { _ = f.Close() }()
	var b [1]byte
	if _, rerr := f.ReadAt(b[:], size-1); rerr != nil {
		return false, fmt.Errorf("read capture file tail: %w", rerr)
	}
	return b[0] != '\n', nil
}

// onSegmentOpened syncs w.size to the just-opened segment's on-disk size and,
// in asciicast mode, writes the format header when that segment is empty. It
// runs at construction and after every reopenCanonical so each segment is
// self-describing — the header-survives-pruning mechanism: even once retention
// drops segment 0, the new oldest segment still starts with a header carrying
// the size in effect, so the reassembled stream stays decodable at the right
// size. A non-empty segment gets no header here: at construction that is a
// previous run's segment, which newCaptureWriter marks with a run header
// instead; after reopenCanonical it is a rename-failed rotation, where the
// same run continues and t keeps increasing.
func (w *captureWriter) onSegmentOpened() error {
	info, err := w.f.Stat()
	if err != nil {
		return fmt.Errorf("stat capture file: %w", err)
	}
	w.size = info.Size()
	w.headerJustWritten = false
	if !w.asciicast || w.size > 0 {
		return nil
	}
	hdr, herr := capture.EncodeHeader(w.cols, w.rows, w.start.Unix())
	if herr != nil {
		return herr
	}
	n, werr := w.f.Write(hdr)
	w.size += int64(n)
	if werr != nil {
		return fmt.Errorf("write asciicast header: %w", werr)
	}
	w.headerJustWritten = true
	return nil
}

// Write appends to the live segment and rotates once it crosses maxBytes. A
// single write is never split across segments; the live segment may exceed the
// threshold by up to one write before rotating.
func (w *captureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return 0, os.ErrClosed
	}
	if !w.asciicast {
		return w.writeRecord(p)
	}
	// In asciicast mode each Write becomes at most one timed output record, so
	// rotation (which checks size after the write) never bisects a record line
	// — the whole-event-writes guarantee the decoder relies on. An incomplete
	// trailing UTF-8 sequence is held back and prepended to the next Write:
	// JSON-encoding it alone would replace it with U+FFFD.
	buf := p
	if len(w.carry) > 0 {
		buf = make([]byte, 0, len(w.carry)+len(p))
		buf = append(buf, w.carry...)
		buf = append(buf, p...)
		w.carry = nil
	}
	cut := incompleteUTF8Tail(buf)
	if cut < len(buf) {
		// Copy: p belongs to the caller and is reused for the next read.
		w.carry = append([]byte(nil), buf[cut:]...)
	}
	if cut == 0 {
		return len(p), nil
	}
	if err := w.writeOutputEvent(buf[:cut]); err != nil {
		// A partial event line is unusable, so report nothing of p durably
		// recorded.
		return 0, err
	}
	// io.Writer contract: report bytes consumed from p, not bytes written to
	// disk — an asciicast event line is longer than its payload.
	return len(p), nil
}

// writeOutputEvent encodes data as one asciicast output event and writes it.
func (w *captureWriter) writeOutputEvent(data []byte) error {
	ev, err := capture.EncodeOutputEvent(time.Since(w.start).Seconds(), data)
	if err != nil {
		return err
	}
	_, err = w.writeRecord(ev)
	return err
}

// writeRecord appends one whole record (raw bytes, or one complete asciicast
// line) to the live segment and rotates once the segment crosses maxBytes.
// Every line — output, resize, run header — goes through here, so a record is
// never split across segments. The caller holds mu and has checked f != nil.
func (w *captureWriter) writeRecord(rec []byte) (int, error) {
	n, err := w.f.Write(rec)
	w.size += int64(n)
	if err != nil {
		return n, err
	}
	if w.maxBytes > 0 && w.size >= w.maxBytes {
		if rerr := w.rotate(); rerr != nil {
			w.logger.Warn("capture rotation failed; continuing on current segment", "err", rerr)
		}
	}
	return n, nil
}

// incompleteUTF8Tail returns the index where an incomplete trailing UTF-8
// sequence of b starts, or len(b) when b does not end mid-character. Only the
// last utf8.UTFMax-1 bytes are inspected. utf8.FullRune treats invalid bytes
// as complete, so bytes invalid for another reason are not held back.
func incompleteUTF8Tail(b []byte) int {
	lo := max(len(b)-(utf8.UTFMax-1), 0)
	for i := len(b) - 1; i >= lo; i-- {
		if utf8.RuneStart(b[i]) {
			if !utf8.FullRune(b[i:]) {
				return i
			}
			return len(b)
		}
	}
	return len(b)
}

// Resize records a terminal size change. In asciicast mode a change to a
// different, positive size appends one `[t, "r", "<cols>x<rows>"]` event and
// makes that the size every later header carries (segment headers after
// rotation, so the recording stays correct once retention prunes the segment
// holding the event). A resize to the current size, a raw capture, or a closed
// writer records nothing. The caller invokes it before applying the size to
// the PTY so the child's redraw lands after the event.
func (w *captureWriter) Resize(cols, rows int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.asciicast || w.f == nil || cols <= 0 || rows <= 0 {
		return nil
	}
	if cols == w.cols && rows == w.rows {
		return nil
	}
	ev, err := capture.EncodeResizeEvent(time.Since(w.start).Seconds(), cols, rows)
	if err != nil {
		return err
	}
	// Set the size before the write: if this event triggers a rotation, the
	// fresh segment's header must already carry the new size.
	w.cols, w.rows = cols, rows
	if _, werr := w.writeRecord(ev); werr != nil {
		return fmt.Errorf("write asciicast resize event: %w", werr)
	}
	return nil
}

// rotate closes the live segment, renames it to its deterministic sibling,
// reopens a fresh canonical, and prunes. Every failure path leaves a writable
// live segment (or, only if reopening the canonical itself fails, a nil f that
// surfaces os.ErrClosed on the next write).
func (w *captureWriter) rotate() error {
	if cerr := w.f.Close(); cerr != nil {
		// Could not flush/close cleanly; do not rename a half-written file.
		// Restore a live segment (best-effort; reopenCanonical logs its own
		// failure) and report the close error.
		_ = w.reopenCanonical()
		return fmt.Errorf("close live segment: %w", cerr)
	}
	w.f = nil

	target := pcapture.SegmentPath(w.canonical, w.nextSeq)
	if rerr := os.Rename(w.canonical, target); rerr != nil {
		_ = w.reopenCanonical()
		return fmt.Errorf("rename live segment to %q: %w", target, rerr)
	}
	w.nextSeq++

	// Compress the just-closed segment synchronously (gzip of a bounded
	// segment is tens of ms). On failure the raw segment stays on disk —
	// still spliceable by readers — so a compression error degrades to
	// "kept raw this round", never lost capture.
	if gzPath, cerr := capture.CompressSegment(target); cerr != nil {
		w.logger.Warn("capture segment compression failed; keeping raw segment", "path", target, "err", cerr)
	} else if w.applyPerms != nil {
		if perr := w.applyPerms(gzPath); perr != nil {
			// Perms are best-effort: a chmod/chown failure on the compressed
			// segment must not stop rotation.
			w.logger.Warn("capture perms reapply failed", "path", gzPath, "err", perr)
		}
	}

	if oerr := w.reopenCanonical(); oerr != nil {
		return oerr
	}
	w.prune()
	return nil
}

// reopenCanonical opens a fresh live segment at the canonical path and resets
// the size counter. On success f is writable and size is 0; on failure f stays
// nil and the error is logged so the caller can surface it.
func (w *captureWriter) reopenCanonical() error {
	f, err := os.OpenFile(w.canonical, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		w.logger.Error("capture reopen failed; capture stops", "path", w.canonical, "err", err)
		return fmt.Errorf("reopen capture file %q: %w", w.canonical, err)
	}
	if w.applyPerms != nil {
		if perr := w.applyPerms(w.canonical); perr != nil {
			// Perms are best-effort on reopen: a chmod/chown failure on the
			// new segment must not stop capture.
			w.logger.Warn("capture perms reapply failed", "path", w.canonical, "err", perr)
		}
	}
	w.f = f
	// Sync size to the (normally empty) fresh segment and, in asciicast mode,
	// write its header so the segment is self-describing. A header failure is
	// non-fatal: the fd is writable, so degrade to "no header this segment"
	// rather than stopping capture.
	if herr := w.onSegmentOpened(); herr != nil {
		w.logger.Warn("capture segment header init failed", "path", w.canonical, "err", herr)
	}
	return nil
}

// prune drops the oldest closed segments until both retention bounds hold:
// at most retainSegments closed segments, and at most retainBytes total across
// them. The live segment is never pruned.
func (w *captureWriter) prune() {
	segs, err := pcapture.ClosedSegments(w.canonical)
	if err != nil {
		w.logger.Warn("capture retention listing failed; skipping prune", "err", err)
		return
	}

	// Count-based: drop oldest (front) until at or under the segment cap.
	if w.retainSegments > 0 {
		for len(segs) > w.retainSegments {
			segs = w.dropOldest(segs)
		}
	}

	// Byte-based: drop oldest until total closed bytes fit the cap.
	if w.retainBytes > 0 {
		var total int64
		for _, s := range segs {
			total += s.Size
		}
		for total > w.retainBytes && len(segs) > 0 {
			total -= segs[0].Size
			segs = w.dropOldest(segs)
		}
	}
}

// dropOldest removes the front (oldest) segment from disk and returns the
// remaining slice. A removal error is logged but still advances the slice so
// pruning makes progress rather than spinning on an undeletable file.
func (w *captureWriter) dropOldest(segs []pcapture.Segment) []pcapture.Segment {
	oldest := segs[0]
	if rerr := os.Remove(oldest.Path); rerr != nil && !os.IsNotExist(rerr) {
		w.logger.Warn("capture retention prune failed", "path", oldest.Path, "err", rerr)
	}
	return segs[1:]
}

// Close writes out any held-back partial UTF-8 sequence as a final event, then
// closes the live segment fd. Idempotency across repeated runner Close calls
// is provided by the runner's closeCapture sync.Once (mirroring the
// pre-rotation contract); this method itself nils f so a late call is a no-op.
func (w *captureWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	var flushErr error
	if len(w.carry) > 0 {
		carry := w.carry
		w.carry = nil
		if err := w.writeOutputEvent(carry); err != nil {
			flushErr = fmt.Errorf("write held-back capture bytes: %w", err)
		}
	}
	if w.f == nil {
		// The final event's rotation failed to reopen a live segment.
		return flushErr
	}
	f := w.f
	w.f = nil
	if err := f.Close(); err != nil {
		return err
	}
	return flushErr
}
