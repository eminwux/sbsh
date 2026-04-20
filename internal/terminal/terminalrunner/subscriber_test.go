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
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// shortenWriteTimeout lowers the drain-goroutine write deadline for the
// duration of a test so a hung-peer scenario detaches quickly instead of
// waiting the production 5s. Returns a restore func.
func shortenWriteTimeout(d time.Duration) func() {
	prev := subscriberWriteTimeout
	subscriberWriteTimeout = d
	return func() { subscriberWriteTimeout = prev }
}

// newPipeConnPair returns a (clientSide, serverSide) net.Pipe pair. The
// subscriberWriter forwards bytes into serverSide; the test reads from
// clientSide to verify output.
func newPipeConnPair() (net.Conn, net.Conn) {
	a, b := net.Pipe()
	return a, b
}

func TestSubscriber_DeliversBytesInOrder(t *testing.T) {
	// Not t.Parallel: tests in this file mutate subscriberWriteTimeout.
	clientSide, serverSide := newPipeConnPair()
	defer clientSide.Close()

	var detached atomic.Bool
	sub := newSubscriberWriter(serverSide, 1<<20, func() { detached.Store(true) }, slog.Default())
	go sub.Run()

	want := []byte("hello world")
	go func() {
		_, _ = sub.Write(want)
		_ = sub.Close()
	}()

	got, err := io.ReadAll(clientSide)
	if err != nil && err != io.EOF {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("bytes = %q, want %q", got, want)
	}

	// Wait briefly for detach callback to run.
	deadline := time.Now().Add(time.Second)
	for !detached.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !detached.Load() {
		t.Fatal("detach callback not invoked on clean close")
	}
}

func TestSubscriber_LaggedReaderIsDropped(t *testing.T) {
	// Cannot t.Parallel alongside shortenWriteTimeout — it mutates a
	// package-level var read by the drain goroutine.
	// The client never reads, so serverSide writes will block. With the
	// production timeout of 5s the detach would lag this test; drop it
	// so the hung-peer path bails quickly.
	restore := shortenWriteTimeout(100 * time.Millisecond)
	defer restore()

	clientSide, serverSide := newPipeConnPair()
	defer clientSide.Close()

	const maxBytes = 128
	var detached atomic.Bool
	sub := newSubscriberWriter(serverSide, maxBytes, func() { detached.Store(true) }, slog.Default())
	go sub.Run()

	// Push well past maxBytes. Writes must always return (len(p), nil)
	// from the fan-out's perspective; the subscriber absorbs the signal.
	chunk := bytes.Repeat([]byte("A"), 64)
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		for i := 0; i < 100; i++ {
			n, err := sub.Write(chunk)
			if err != nil {
				t.Errorf("unexpected Write error: %v", err)
				return
			}
			if n != len(chunk) {
				t.Errorf("short write: got %d want %d", n, len(chunk))
				return
			}
		}
	}()

	select {
	case <-writeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Write path blocked despite overflow — backpressure escaped to fan-out")
	}

	// Detach callback must fire once the drain goroutine exits.
	deadline := time.Now().Add(2 * time.Second)
	for !detached.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !detached.Load() {
		t.Fatal("detach callback not invoked after overflow")
	}
}

// TestSubscriber_LaggedNoticeReachesClient covers the bug flagged in the
// PR #119 review: on overflow, the sentinel must actually arrive on the
// client side so the reader can surface ErrSubscriberLagged instead of a
// clean EOF. Before the fix, Write closed the conn before Run could
// drain, so the sentinel was silently dropped.
func TestSubscriber_LaggedNoticeReachesClient(t *testing.T) {
	// Not t.Parallel for the same reason as above.
	clientSide, serverSide := newPipeConnPair()

	const maxBytes = 128
	var detached atomic.Bool
	sub := newSubscriberWriter(serverSide, maxBytes, func() { detached.Store(true) }, slog.Default())
	go sub.Run()

	// Reader drains the client side so drain goroutine can make progress
	// until the overflow fires and the sentinel is written.
	type readResult struct {
		data []byte
		err  error
	}
	readCh := make(chan readResult, 1)
	go func() {
		got, err := io.ReadAll(clientSide)
		readCh <- readResult{data: got, err: err}
	}()

	// Force an overflow immediately: one chunk bigger than maxBytes makes
	// the first Write hit the lagged branch. The subsequent Close is a
	// no-op because Write already marked closed.
	big := bytes.Repeat([]byte("A"), maxBytes*2)
	if n, err := sub.Write(big); err != nil || n != len(big) {
		t.Fatalf("Write: n=%d err=%v", n, err)
	}

	select {
	case res := <-readCh:
		if res.err != nil && res.err != io.EOF {
			t.Fatalf("read: %v", res.err)
		}
		if !bytes.Contains(res.data, laggedNotice) {
			t.Fatalf("laggedNotice not present in stream; got %q", res.data)
		}
	case <-time.After(2 * time.Second):
		_ = clientSide.Close()
		t.Fatal("timed out waiting for lagged sentinel on client side")
	}

	deadline := time.Now().Add(time.Second)
	for !detached.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !detached.Load() {
		t.Fatal("detach callback not invoked after sentinel delivery")
	}
}
