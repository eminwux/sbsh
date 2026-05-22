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
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
	"golang.org/x/sys/unix"
)

// waitFor polls cond every 10ms until it returns true or the timeout
// elapses. Used to settle async-state assertions in the race test; the
// caller always re-checks the invariant after this returns, so the result
// is intentionally not surfaced.
func waitFor(timeout time.Duration, cond func() bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// newSubscribeRaceExec scaffolds the minimum Exec that lets Subscribe and
// Close exercise the full ctx/ptyPipes/subscribers/multiOutW interaction
// without a real PTY. Mirrors newCloseRaceExec but installs a live
// DynamicMultiWriter so Subscribe doesn't trip the "terminal not running"
// guard.
func newSubscribeRaceExec(t *testing.T) *Exec {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Exec{
		ctx:       ctx,
		ctxCancel: cancel,
		logger:    logger,
		id:        "subscribe-race-test",
		metadata: api.TerminalDoc{
			Spec: api.TerminalSpec{RunPath: t.TempDir()},
		},
		clients:       make(map[api.ID]*ioClient),
		subscribers:   make(map[*subscriberWriter]struct{}),
		closeReqCh:    make(chan error, 1),
		closedCh:      make(chan struct{}),
		childDoneCh:   make(chan struct{}),
		childDoneOnce: &sync.Once{},
		ptyPipes:      &ptyPipes{multiOutW: NewDynamicMultiWriter(logger)},
		closePTY:      &sync.Once{},
		closeClosedCh: &sync.Once{},
	}
}

// TestSubscribe_NoOrphanDrainOnConcurrentClose is the regression for #226.
//
// Pre-fix, Subscribe could addSubscriber + start the drain goroutine after
// Close's closeAllSubscribers had already snapshotted sr.subscribers. The
// drain parked on cond.Wait() with no surviving writer to signal it — the
// goroutine and the socketpair server-side fd leaked. Under repeated
// runner restart in tests this accumulates.
//
// Post-fix, Subscribe runs a late ctx-check after `go sub.Run()` and, on
// observed shutdown, calls sub.Close() to wake the drain and let its
// deferred onDetach + conn.Close run. We assert two invariants once Close
// has settled:
//
//  1. sr.subscribers is empty — every registered subscriber was detached.
//  2. The drain goroutine count returns to baseline — no parked
//     cond.Wait() callers survive shutdown.
//
// Race-detector enabled (`go test -race`) doubles the value: the
// subsMu/ctxCancel ordering is checked at the same time.
func TestSubscribe_NoOrphanDrainOnConcurrentClose(t *testing.T) {
	const concurrentSubscribers = 100

	sr := newSubscribeRaceExec(t)

	// Collect client-side fds so the test can close them deterministically
	// at the end — the server-side fd is owned by the subscriberWriter and
	// is freed when the drain goroutine's deferred conn.Close runs.
	var cliFDsMu sync.Mutex
	cliFDs := make([]int, 0, concurrentSubscribers)
	collectCliFD := func(fd int) {
		cliFDsMu.Lock()
		cliFDs = append(cliFDs, fd)
		cliFDsMu.Unlock()
	}

	goroutinesBefore := runtime.NumGoroutine()

	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := range concurrentSubscribers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			req := &api.SubscribeRequest{ClientID: api.ID(fmt.Sprintf("client-%d", idx))}
			resp := &api.ResponseWithFD{}
			err := sr.Subscribe(req, resp)
			if err != nil {
				// Post-Close rejection is the expected outcome for the
				// late-arriving half; the cliFD was already closed by
				// Subscribe's error path.
				if !errors.Is(err, errTerminalClosing) {
					t.Errorf("subscribe (idx=%d): unexpected error: %v", idx, err)
				}
				return
			}
			for _, fd := range resp.FDs {
				collectCliFD(fd)
			}
		}(i)
	}

	close(start)

	// Deterministically gate Close on the race window opening rather than a
	// fixed pre-Close sleep. The meaningful #226 window is a subscriber that
	// passed Subscribe's early ctx-check and ran addSubscriber (observable as
	// a non-empty registry) — its drain goroutine is what Close's
	// closeAllSubscribers (or Subscribe's own late ctx-check) must reap. A
	// fixed sleep raced scheduler jitter under full-package parallel load:
	// Close could cancel ctx before any goroutine reached addSubscriber, so
	// every subscriber took the early rejection and the old
	// subscribeErrCount==concurrentSubscribers guard fired spuriously (#356).
	// Nothing drains the registry before Close, so once non-empty it stays
	// non-empty until Close — no TOCTOU. With 100 concurrent subscribers,
	// firing Close right after the first registration still leaves the bulk
	// of the burst arriving post-Close, so the post-snapshot arrival path the
	// test exists to cover stays well exercised.
	windowOpened := func() bool {
		sr.subsMu.Lock()
		defer sr.subsMu.Unlock()
		return len(sr.subscribers) > 0
	}
	waitFor(2*time.Second, windowOpened)
	if !windowOpened() {
		t.Fatal("race window never opened — no subscriber registered before Close")
	}

	if err := sr.Close(nil); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	wg.Wait()

	// Invariant 1 (eventually): every registered subscriber was detached.
	// The drain goroutine runs onDetach in its defer, so the registry
	// drains asynchronously after sub.Close fires — poll instead of
	// snapshotting. closeAllSubscribers + Subscribe's late ctx-check
	// together must leave the map empty within the settling window.
	subscribersRemaining := func() int {
		sr.subsMu.Lock()
		defer sr.subsMu.Unlock()
		return len(sr.subscribers)
	}
	waitFor(2*time.Second, func() bool { return subscribersRemaining() == 0 })
	if remaining := subscribersRemaining(); remaining != 0 {
		t.Fatalf("sr.subscribers has %d residual entries after Close; regression of #226", remaining)
	}

	// Invariant 2: drain goroutines have exited. The drain goroutine
	// detaches on close-or-EOF, so once the registry is empty all drains
	// should be observable as exited within a short settling window.
	waitFor(2*time.Second, func() bool { return runtime.NumGoroutine() <= goroutinesBefore+2 })
	if goroutinesAfter := runtime.NumGoroutine(); goroutinesAfter > goroutinesBefore+2 {
		t.Fatalf("goroutine count did not settle: before=%d after=%d (drain leak; regression of #226)",
			goroutinesBefore, goroutinesAfter)
	}

	// Close client-side fds — these are the kernel handles the test owns
	// directly; the server-side fds were freed by the drain goroutines.
	for _, fd := range cliFDs {
		_ = unix.Close(fd)
	}
}

// TestSubscribe_PostCloseRejectsCleanly is the deterministic single-thread
// half of the same regression. Close runs first; then a fresh Subscribe is
// invoked. Pre-fix this registered a sub whose drain would never be woken
// (orphaned goroutine + leaked fd). Post-fix the early ctx-check rejects
// with errTerminalClosing and frees both socketpair ends.
func TestSubscribe_PostCloseRejectsCleanly(t *testing.T) {
	sr := newSubscribeRaceExec(t)

	if err := sr.Close(nil); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	req := &api.SubscribeRequest{ClientID: api.ID("client-post-close")}
	resp := &api.ResponseWithFD{}
	err := sr.Subscribe(req, resp)
	if err == nil {
		t.Fatal("Subscribe after Close should fail; got nil error")
	}
	if !errors.Is(err, errTerminalClosing) {
		t.Fatalf("Subscribe after Close: got err=%v, want errTerminalClosing", err)
	}
	if len(resp.FDs) != 0 {
		t.Fatalf("Subscribe error path leaked %d fds in response; want 0", len(resp.FDs))
	}

	sr.subsMu.Lock()
	remaining := len(sr.subscribers)
	sr.subsMu.Unlock()
	if remaining != 0 {
		t.Fatalf("post-Close Subscribe registered %d subscribers; want 0", remaining)
	}
}
