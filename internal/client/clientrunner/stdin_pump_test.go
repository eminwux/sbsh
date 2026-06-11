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

package clientrunner

import (
	"context"
	"errors"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

// countCopierGoroutines reports how many dualcopier read/write loops are
// currently running, by counting runReadWriter frames in the full goroutine
// dump. Each live copier (stdin->socket, socket->stdout) sits in exactly one.
func countCopierGoroutines() int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), "dualcopier.(*Copier).runReadWriter")
}

// waitCopierGoroutines polls until countCopierGoroutines() reaches want or the
// deadline elapses, returning the final count.
func waitCopierGoroutines(want int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for {
		got := countCopierGoroutines()
		if got == want || time.Now().After(deadline) {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type pumpReadOut struct {
	n   int
	buf []byte
	err error
}

// TestStdinPump_ByteSurvivesConsumerCancellation is the pump-level invariant
// behind issue #399: cancelling a consumer returns promptly without consuming
// from the source, and a byte the source produces while no consumer is active
// is delivered to the next consumer rather than lost or stolen.
func TestStdinPump_ByteSurvivesConsumerCancellation(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })

	p := newStdinPump(r)

	// Consumer 1 parks on an empty source, then is cancelled.
	ctx1, cancel1 := context.WithCancel(context.Background())
	done := make(chan pumpReadOut, 1)
	go func() {
		buf := make([]byte, 16)
		n, rerr := p.read(ctx1, buf)
		done <- pumpReadOut{n, append([]byte(nil), buf[:n]...), rerr}
	}()

	time.Sleep(50 * time.Millisecond) // best-effort: let consumer 1 park
	cancel1()

	select {
	case out := <-done:
		if !errors.Is(out.err, context.Canceled) {
			t.Fatalf("consumer 1: want context.Canceled, got n=%d err=%v", out.n, out.err)
		}
		if out.n != 0 {
			t.Fatalf("consumer 1 consumed %d bytes on cancellation; want 0", out.n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("consumer 1 did not return within 2s of cancellation")
	}

	// A byte arrives while no consumer is active.
	if _, werr := w.WriteString("Z"); werr != nil {
		t.Fatalf("write between consumers: %v", werr)
	}

	// Consumer 2 must receive that exact byte.
	buf := make([]byte, 16)
	n, rerr := p.read(context.Background(), buf)
	if rerr != nil {
		t.Fatalf("consumer 2 read: %v", rerr)
	}
	if n != 1 || buf[0] != 'Z' {
		t.Fatalf("consumer 2: got n=%d buf=%q; want 1 byte %q", n, buf[:n], "Z")
	}
}

// newPumpExec wires a minimal Exec onto the given stdin/stdout handles and a
// pre-connected socket, suitable for driving startConnectionManager and
// restoreParentTerminal directly. DetachKeystroke is off so no escape filter
// rewrites the byte stream. The session context is created internally and torn
// down on test cleanup.
func newPumpExec(t *testing.T, stdin, stdout *os.File, ioConn net.Conn) *Exec {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Exec{
		id:         api.ID("client-pump"),
		ctx:        ctx,
		ctxCancel:  cancel,
		logger:     testLogger(),
		events:     make(chan Event, 8),
		metadataMu: sync.RWMutex{},
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stdout,
		ioConn:     ioConn,
		terminal:   &api.AttachedTerminal{Spec: &api.TerminalSpec{ID: api.ID("term-pump")}},
		metadata: api.ClientDoc{
			APIVersion: api.APIVersionV1Beta1,
			Kind:       api.KindClient,
			Spec: api.ClientSpec{
				ID:              api.ID("client-pump"),
				DetachKeystroke: false,
			},
		},
	}
}

// TestStdinPump_SequentialAttachReuse is the issue #399 acceptance criterion at
// the layer the bug lives in: two sequential attach sessions over a
// non-pollable stdin (an os.Pipe, same pollability as the real os.Stdin) must
// leave no copier goroutine from session 1 alive into session 2, and a byte
// written between the sessions must be delivered to session 2's socket rather
// than stolen by an abandoned session-1 reader.
func TestStdinPump_SequentialAttachReuse(t *testing.T) {
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() { _ = stdinR.Close(); _ = stdinW.Close() })

	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = devnull.Close() })

	baseline := countCopierGoroutines()

	// ---- session 1 ----
	client1, server1 := unixSocketPair(t)
	sr1 := newPumpExec(t, stdinR, devnull, client1)
	if startErr := sr1.startConnectionManager(); startErr != nil {
		t.Fatalf("session 1 startConnectionManager: %v", startErr)
	}
	if got := waitCopierGoroutines(baseline+2, 2*time.Second); got != baseline+2 {
		t.Fatalf("session 1: want %d copier goroutines running, got %d", baseline+2, got)
	}
	_ = server1 // session 1 sends nothing; the connection is torn down below.

	// Tear session 1 down. No copier goroutine from it may survive.
	sr1.restoreParentTerminal("", false)
	if got := waitCopierGoroutines(baseline, 2*time.Second); got != baseline {
		t.Fatalf("after session 1 teardown: %d copier goroutines survived; want %d", got, baseline)
	}

	// A byte typed between sessions must not be lost.
	if _, werr := stdinW.WriteString("Z"); werr != nil {
		t.Fatalf("write between sessions: %v", werr)
	}

	// ---- session 2 (same stdin) ----
	client2, server2 := unixSocketPair(t)
	sr2 := newPumpExec(t, stdinR, devnull, client2)
	if startErr := sr2.startConnectionManager(); startErr != nil {
		t.Fatalf("session 2 startConnectionManager: %v", startErr)
	}

	// The between-sessions byte must arrive at session 2's socket.
	if derr := server2.SetReadDeadline(time.Now().Add(2 * time.Second)); derr != nil {
		t.Fatalf("set read deadline: %v", derr)
	}
	rb := make([]byte, 1)
	n, rerr := server2.Read(rb)
	if rerr != nil {
		t.Fatalf("session 2 socket read: %v (byte written between sessions was lost or stolen)", rerr)
	}
	if n != 1 || rb[0] != 'Z' {
		t.Fatalf("session 2 received n=%d %q; want the between-sessions byte %q", n, rb[:n], "Z")
	}

	sr2.restoreParentTerminal("", false)
}
