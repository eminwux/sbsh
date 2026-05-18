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
	"fmt"
	"io"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

// newDummyClient mints an ioClient with a real net.Conn so Close's
// per-client conn.Close call has something to close. net.Pipe returns two
// connected in-memory conns; we keep just one — the other is GC'd, which
// is fine for a teardown-only test.
func newDummyClient(idx int) *ioClient {
	id := api.ID(fmt.Sprintf("client-%d", idx))
	c, _ := net.Pipe()
	return &ioClient{id: &id, conn: c}
}

// newCloseRaceExec builds the smallest Exec that lets Close() run end-to-end
// without nil-deref while still exercising the clients-map teardown. Every
// heavy field Close inspects (cmd, ptmx, lnCtrl, reaper, signal forwarder)
// is already nil-checked there, and metadata writes are pointed at
// t.TempDir() so they pass silently. The shape mirrors the existing
// newShutdownTestExec helper.
func newCloseRaceExec(t *testing.T) *Exec {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Exec{
		ctx:       ctx,
		ctxCancel: cancel,
		logger:    logger,
		id:        "race-test",
		metadata: api.TerminalDoc{
			Spec: api.TerminalSpec{RunPath: t.TempDir()},
		},
		clients:       make(map[api.ID]*ioClient),
		subscribers:   make(map[*subscriberWriter]struct{}),
		closeReqCh:    make(chan error, 1),
		closedCh:      make(chan struct{}),
		childDoneCh:   make(chan struct{}),
		childDoneOnce: &sync.Once{},
		ptyPipes:      &ptyPipes{},
		closePTY:      &sync.Once{},
	}
}

// TestExecClose_NoNilMapOnConcurrentAddClient is the regression for #225.
//
// Pre-fix, Close() set sr.clients = nil and dropped clientsMu. A concurrent
// addClient (from a per-conn RPC handler still draining after the accept
// loop's ctx-cancel exit) then wrote to a nil map and panicked the runner
// process. Post-fix, clear(sr.clients) leaves the map addressable so the
// late write is a benign stale-entry no-op.
//
// Race-detector enabled (`go test -race`) doubles the value: lock discipline
// in Close versus addClient is checked at the same time.
func TestExecClose_NoNilMapOnConcurrentAddClient(t *testing.T) {
	const concurrentClients = 200

	sr := newCloseRaceExec(t)

	var wg sync.WaitGroup
	var panicCount atomic.Int32
	start := make(chan struct{})

	for i := range concurrentClients {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicCount.Add(1)
					t.Errorf("addClient panicked (idx=%d): %v\n%s", idx, r, debug.Stack())
				}
			}()
			<-start
			sr.addClient(newDummyClient(idx))
		}(i)
	}

	close(start)
	// Small window so a chunk of addClient calls land before Close acquires
	// clientsMu and a chunk arrive after — the bug window is the post-Close
	// arrivals, where the pre-fix `sr.clients = nil` turned addClient into
	// a panic.
	time.Sleep(2 * time.Millisecond)
	if err := sr.Close(nil); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	wg.Wait()

	if n := panicCount.Load(); n > 0 {
		t.Fatalf("addClient panicked %d times under concurrent Close; regression of #225", n)
	}
}

// TestExecClose_PostCloseAddClientDoesNotPanic is the narrower deterministic
// half of the same regression. Close runs to completion first; then a fresh
// addClient is invoked. Pre-fix this was a guaranteed panic (nil-map write);
// post-fix the map is empty-but-addressable and the call returns cleanly.
//
// This complements the race test above: the race version catches the
// concurrent ordering, this one is the always-fires single-thread proof.
func TestExecClose_PostCloseAddClientDoesNotPanic(t *testing.T) {
	sr := newCloseRaceExec(t)

	if err := sr.Close(nil); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("post-Close addClient panicked: %v\n%s", r, debug.Stack())
		}
	}()
	sr.addClient(newDummyClient(0))
}
