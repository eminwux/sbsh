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

	"github.com/eminwux/sbsh/internal/capture"
	"github.com/eminwux/sbsh/internal/defaults"
)

// captureWriter is the always-on capture sink registered first in multiOutW.
// It writes the live segment append-only at the canonical path and, once the
// live segment crosses maxBytes, rotates: close → rename to a deterministic
// sibling (capture.SegmentPath) → gzip-compress the closed segment
// (capture.CompressSegment) → reopen a fresh canonical → prune oldest closed
// segments by count and total bytes. The live segment is never compressed (it
// must stay raw and seekable); only closed segments are. Readers reassemble
// and transparently decompress the full transcript via the internal/capture
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

	// applyPerms re-applies the resolved capture mode/group to a freshly
	// opened segment path. nil leaves perms at the open default.
	applyPerms func(path string) error
	logger     *slog.Logger
}

// newCaptureWriter opens (or reopens, append-only) the canonical capture path
// and returns a rotating writer seeded from any closed segments already on
// disk, so sequence numbering resumes gap-free across restarts. applyPerms is
// invoked on the freshly opened canonical immediately, mirroring the explicit
// permissions step the non-rotating path performed after Open.
func newCaptureWriter(
	canonical string,
	logger *slog.Logger,
	applyPerms func(path string) error,
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
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat capture file: %w", err)
	}
	nextSeq, err := capture.NextSeq(canonical)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("seed capture sequence: %w", err)
	}
	return &captureWriter{
		canonical:      canonical,
		f:              f,
		size:           info.Size(),
		nextSeq:        nextSeq,
		maxBytes:       defaults.CaptureSegmentMaxBytes,
		retainSegments: defaults.CaptureRetentionMaxSegments,
		retainBytes:    defaults.CaptureRetentionMaxBytes,
		applyPerms:     applyPerms,
		logger:         logger,
	}, nil
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
	n, err := w.f.Write(p)
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

	target := capture.SegmentPath(w.canonical, w.nextSeq)
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
	w.size = 0
	return nil
}

// prune drops the oldest closed segments until both retention bounds hold:
// at most retainSegments closed segments, and at most retainBytes total across
// them. The live segment is never pruned.
func (w *captureWriter) prune() {
	segs, err := capture.ClosedSegments(w.canonical)
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
func (w *captureWriter) dropOldest(segs []capture.Segment) []capture.Segment {
	oldest := segs[0]
	if rerr := os.Remove(oldest.Path); rerr != nil && !os.IsNotExist(rerr) {
		w.logger.Warn("capture retention prune failed", "path", oldest.Path, "err", rerr)
	}
	return segs[1:]
}

// Close closes the live segment fd. Idempotency across repeated runner Close
// calls is provided by the runner's closeCapture sync.Once (mirroring the
// pre-rotation contract); this method itself nils f so a late call is a no-op.
func (w *captureWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	f := w.f
	w.f = nil
	return f.Close()
}
