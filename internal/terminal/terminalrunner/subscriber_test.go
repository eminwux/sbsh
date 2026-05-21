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
	"errors"
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

// shortenBackpressureStall lowers the per-write backpressure stall budget so
// a stuck-attacher scenario falls through to drop-on-lag quickly instead of
// pacing the producer for the production 2s. Returns a restore func.
func shortenBackpressureStall(d time.Duration) func() {
	prev := subscriberBackpressureStall
	subscriberBackpressureStall = d
	return func() { subscriberBackpressureStall = prev }
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

// TestSubscriber_SeededReplayPrecedesLiveOutputLargeCapture is the
// regression for issue #299: attaching a second client to a busy terminal
// with a large (>1 MiB) capture used to write the replay directly to the
// conn on a second goroutine, racing the drain goroutine's per-write
// SetWriteDeadline and disconnecting the new client with i/o timeout. The
// fix seeds the replay into the ring and grows the byte bound so the single
// drain goroutine emits replay-then-live in order without tripping the
// lagged path. Without seedReplay's bound growth the first live Write would
// overflow the 1 MiB ring and the client would receive laggedNotice plus a
// truncated stream.
func TestSubscriber_SeededReplayPrecedesLiveOutputLargeCapture(t *testing.T) {
	// Not t.Parallel: tests in this file mutate subscriberWriteTimeout.
	clientSide, serverSide := newPipeConnPair()
	defer clientSide.Close()

	// Replay larger than the 1 MiB ring bound — the case that exposed the bug.
	replay := bytes.Repeat([]byte("R"), defaultSubscriberBufferBytes+4096)
	live := []byte("LIVE-AFTER-REPLAY")

	var detached atomic.Bool
	sub := newSubscriberWriter(
		serverSide,
		defaultSubscriberBufferBytes,
		func() { detached.Store(true) },
		slog.Default(),
	)
	sub.seedReplay(replay)
	go sub.Run()

	// Live output arrives via the fan-out after the seed, exactly as the PTY
	// reader would deliver it once the writer is registered in multiOutW.
	go func() {
		n, err := sub.Write(live)
		if err != nil || n != len(live) {
			t.Errorf("live Write: n=%d err=%v", n, err)
		}
		_ = sub.Close()
	}()

	got, err := io.ReadAll(clientSide)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("read: %v", err)
	}
	if bytes.Contains(got, laggedNotice) {
		t.Fatal("client was lagged-dropped during large replay (issue #299 regression)")
	}
	want := append(append([]byte(nil), replay...), live...)
	if !bytes.Equal(got, want) {
		t.Fatalf("stream mismatch: got %d bytes, want %d (replay=%d live=%d)",
			len(got), len(want), len(replay), len(live))
	}

	deadline := time.Now().Add(time.Second)
	for !detached.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !detached.Load() {
		t.Fatal("detach callback not invoked after clean close")
	}
}

// TestSubscriber_BackpressureThrottlesSlowInteractiveReader is the regression
// for issue #312: an interactive attacher running a firehose
// (`cat /dev/urandom | hexdump`) used to overflow the 1 MiB ring and be
// disconnected with the lagged sentinel (regressed by #217, f6b3f4f). With
// backpressure the producer is paced to the attacher's drain rate — slow but
// never dropped, exactly how a real PTY flow-controls a process to its
// terminal. The slow-but-live reader below consumes far less per tick than
// the producer emits, forcing the ring past maxBytes repeatedly; the writer
// must throttle (no lagged) and deliver the whole stream.
func TestSubscriber_BackpressureThrottlesSlowInteractiveReader(t *testing.T) {
	// Not t.Parallel: mutates package-level timeout vars.
	clientSide, serverSide := newPipeConnPair()
	defer clientSide.Close()

	const maxBytes = 4096
	var detached atomic.Bool
	sub := newSubscriberWriter(serverSide, maxBytes, func() { detached.Store(true) }, slog.Default())
	sub.enableBackpressure()
	go sub.Run()

	// total well past the ring bound: a drop-on-lag writer would disconnect
	// here. The reader is live (always reading) but slow — small reads with a
	// brief pause — so the producer outpaces it and the ring keeps filling.
	const total = maxBytes * 8
	readDone := make(chan int, 1)
	go func() {
		buf := make([]byte, 256)
		got := 0
		for got < total {
			n, err := clientSide.Read(buf)
			got += n
			if err != nil {
				break
			}
			time.Sleep(200 * time.Microsecond)
		}
		readDone <- got
	}()

	chunk := bytes.Repeat([]byte("A"), 512)
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		for sent := 0; sent < total; sent += len(chunk) {
			n, err := sub.Write(chunk)
			if err != nil || n != len(chunk) {
				t.Errorf("Write: n=%d err=%v", n, err)
				return
			}
		}
		_ = sub.Close()
	}()

	select {
	case <-writeDone:
	case <-time.After(10 * time.Second):
		t.Fatal("producer never completed — backpressure deadlocked")
	}

	got := <-readDone

	sub.mu.Lock()
	lagged := sub.lagged
	sub.mu.Unlock()
	if lagged {
		t.Fatal("interactive attacher was lagged-dropped despite actively reading (issue #312)")
	}
	if got < total {
		t.Fatalf("reader received %d of %d bytes — stream truncated", got, total)
	}

	deadline := time.Now().Add(2 * time.Second)
	for !detached.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !detached.Load() {
		t.Fatal("detach callback not invoked after clean close")
	}
}

// TestSubscriber_BackpressureStuckAttacherDropsAndFreesProducer locks in the
// #217 invariant under the new policy: a backpressure attacher whose peer
// stops reading must not pace the producer indefinitely. After the stall
// budget elapses the writer falls through to drop-on-lag — Write returns,
// the attacher is detached, and the shared fan-out is freed.
func TestSubscriber_BackpressureStuckAttacherDropsAndFreesProducer(t *testing.T) {
	// Not t.Parallel: mutates package-level timeout vars.
	restoreStall := shortenBackpressureStall(100 * time.Millisecond)
	defer restoreStall()
	restoreWrite := shortenWriteTimeout(100 * time.Millisecond)
	defer restoreWrite()

	clientSide, serverSide := newPipeConnPair()
	defer clientSide.Close()

	const maxBytes = 128
	var detached atomic.Bool
	sub := newSubscriberWriter(serverSide, maxBytes, func() { detached.Store(true) }, slog.Default())
	sub.enableBackpressure()
	go sub.Run()

	// Peer never reads, so the drain blocks and the ring fills. The producer's
	// overflowing Writes must still return (after pacing out the stall budget)
	// rather than parking forever.
	chunk := bytes.Repeat([]byte("A"), 64)
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		for range 100 {
			if n, err := sub.Write(chunk); err != nil || n != len(chunk) {
				t.Errorf("Write: n=%d err=%v", n, err)
				return
			}
		}
	}()

	select {
	case <-writeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("producer parked on a stuck attacher — #217 invariant violated")
	}

	deadline := time.Now().Add(2 * time.Second)
	for !detached.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !detached.Load() {
		t.Fatal("stuck backpressure attacher was not detached")
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
