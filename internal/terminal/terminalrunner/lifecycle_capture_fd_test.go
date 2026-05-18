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
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/eminwux/sbsh/pkg/api"
)

// newCaptureFDExec builds a minimal Exec wired with a real *os.File as the
// owned capture-file handle. Mirrors newCloseRaceExec in shape; the only
// extra step is opening the temp capture file and assigning it through the
// same path startPty does after applyArtifactPerms succeeds.
func newCaptureFDExec(t *testing.T) (*Exec, *os.File) {
	t.Helper()
	dir := t.TempDir()
	logf, err := os.OpenFile(
		filepath.Join(dir, "capture.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0o600,
	)
	if err != nil {
		t.Fatalf("open capture file: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sr := &Exec{
		ctx:       ctx,
		ctxCancel: cancel,
		logger:    logger,
		id:        "capture-fd-test",
		metadata: api.TerminalDoc{
			Spec: api.TerminalSpec{RunPath: dir},
		},
		clients:       make(map[api.ID]*ioClient),
		subscribers:   make(map[*subscriberWriter]struct{}),
		closeReqCh:    make(chan error, 1),
		closedCh:      make(chan struct{}),
		childDoneCh:   make(chan struct{}),
		childDoneOnce: &sync.Once{},
		ptyPipes:      &ptyPipes{},
		closePTY:      &sync.Once{},
		closeCapture:  &sync.Once{},
		closeClosedCh: &sync.Once{},
		captureFile:   logf,
	}
	return sr, logf
}

// TestExecClose_CapturesFileFD is the direct regression for #229's first AC:
// Close must close the capture file fd. Verified by asking the *os.File for
// a second Close — a fd already closed by Close returns os.ErrClosed; a
// leaked fd would return nil.
func TestExecClose_CapturesFileFD(t *testing.T) {
	sr, logf := newCaptureFDExec(t)

	if err := sr.Close(nil); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := logf.Close(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("captureFile not closed by Close: second Close returned %v, want os.ErrClosed", err)
	}
}

// TestExecClose_CaptureFileDoubleCloseIsHarmless is the regression for #229's
// idempotency AC plus the closedCh follow-up in #242. closeCapture and
// closeClosedCh are sync.Once, so a second Close on the runner must not
// double-close the underlying fd (which would error and log a warning), must
// not panic on `close of closed channel`, and must return nil.
func TestExecClose_CaptureFileDoubleCloseIsHarmless(t *testing.T) {
	sr, _ := newCaptureFDExec(t)

	if err := sr.Close(nil); err != nil {
		t.Fatalf("first Close returned error: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("second Close panicked: %v", r)
		}
	}()
	if err := sr.Close(nil); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
}

// TestExecClose_CaptureFileFDNoAccumulation is the regression for #229's
// loop AC: repeated New→assign→Close cycles must not leak one fd per cycle.
// Counts /proc/self/fd entries before and after a tight loop; a per-cycle
// leak would scale the delta linearly with iterations, so we assert the
// delta is well below the loop count.
//
// Linux-only: /proc/self/fd is the cheapest cross-runtime fd inventory; on
// other platforms the test skips rather than hand-roll a portable count.
func TestExecClose_CaptureFileFDNoAccumulation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("requires /proc/self/fd (linux); GOOS=%s", runtime.GOOS)
	}
	const iterations = 50

	countFDs := func() int {
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatalf("read /proc/self/fd: %v", err)
		}
		return len(entries)
	}

	// Warm the path once so any one-shot allocations (logger init, tempdir
	// bookkeeping) don't get counted as per-iteration growth.
	srWarm, _ := newCaptureFDExec(t)
	if err := srWarm.Close(nil); err != nil {
		t.Fatalf("warm Close: %v", err)
	}

	before := countFDs()
	for i := range iterations {
		sr, _ := newCaptureFDExec(t)
		if err := sr.Close(nil); err != nil {
			t.Fatalf("iter %d Close: %v", i, err)
		}
	}
	after := countFDs()

	// A per-cycle leak would push delta >= iterations. Allow a small
	// constant for test-runtime noise (goroutine-scheduler internals,
	// transient log handles) without masking a real linear leak.
	const noiseTolerance = 5
	if delta := after - before; delta >= noiseTolerance {
		t.Fatalf("fd count grew by %d over %d iterations (before=%d after=%d); expected < %d",
			delta, iterations, before, after, noiseTolerance)
	}
}
