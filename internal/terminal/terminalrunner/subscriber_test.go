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

// newPipeConnPair returns a (clientSide, serverSide) net.Pipe pair. The
// subscriberWriter forwards bytes into serverSide; the test reads from
// clientSide to verify output.
func newPipeConnPair() (net.Conn, net.Conn) {
	a, b := net.Pipe()
	return a, b
}

func TestSubscriber_DeliversBytesInOrder(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	// The client never reads, so serverSide writes will block once the
	// net.Pipe buffer fills. Any overflow beyond maxBytes must close
	// the subscriber rather than stall the fan-out.
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
