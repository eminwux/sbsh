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
	"io"
	"os"
	"sync"
)

// stdinPump is a single persistent reader over a stdin *os.File (typically the
// process's os.Stdin) that outlives any one attach session. Reading the
// descriptor in one long-lived, process-scoped goroutine — rather than
// directly inside each session's stdin->socket copier — is what makes
// sequential pkg/attach.Run reuse safe (issue #399):
//
//   - On teardown a session detaches its consumer via context cancellation
//     (see pumpReader) instead of abandoning a blocking Read on the
//     descriptor. The dualcopier goroutine therefore exits promptly and does
//     not survive into the next session.
//   - Any byte the pump has already pulled off the descriptor but not yet
//     handed to a consumer is buffered and delivered to the next consumer
//     rather than written at the torn-down session's dead socket.
//
// The real os.Stdin (a blocking fd 0) is non-pollable: SetReadDeadline is a
// no-op on it, so the pre-#399 deadline-based unblock could not interrupt the
// parked read. The bounded copier wait then gave up and left the reader
// parked, where it consumed and mis-delivered the next session's first byte.
type stdinPump struct {
	ch chan pumpResult

	mu       sync.Mutex
	leftover []byte // bytes read but not yet consumed; survives across sessions
	readErr  error  // sticky: once the source errors, every read returns it
}

type pumpResult struct {
	data []byte
	err  error
}

//nolint:gochecknoglobals // process-wide registry: one persistent stdin reader per descriptor, shared across sequential attach sessions
var (
	stdinPumpMu       sync.Mutex
	stdinPumpRegistry = map[*os.File]*stdinPump{}
)

// pumpForStdin returns the process-wide pump for f, creating and starting it on
// first use. Keying on the *os.File pointer — not the int fd — avoids aliasing
// a closed descriptor whose number was later reused by a different file.
func pumpForStdin(f *os.File) *stdinPump {
	stdinPumpMu.Lock()
	defer stdinPumpMu.Unlock()
	if p, ok := stdinPumpRegistry[f]; ok {
		return p
	}
	p := newStdinPump(f)
	stdinPumpRegistry[f] = p
	return p
}

func newStdinPump(src io.Reader) *stdinPump {
	p := &stdinPump{ch: make(chan pumpResult)}
	go p.run(src)
	return p
}

// run is the single descriptor-owning goroutine. The unbuffered channel makes
// it park on the send after each read until a consumer receives, so at most one
// read's worth of input is ever held outside the descriptor — and that held
// input is what gets delivered to the next session rather than lost.
func (p *stdinPump) run(src io.Reader) {
	for {
		buf := make([]byte, 32*1024) //nolint:mnd // 32 KiB, matches dualcopier's buffer
		n, err := src.Read(buf)
		if n > 0 {
			p.ch <- pumpResult{data: buf[:n]}
		}
		if err != nil {
			p.ch <- pumpResult{err: err}
			return
		}
	}
}

// read serves bytes to the current consumer, honoring ctx. It returns ctx.Err()
// promptly on cancellation. Crucially, once ctx is cancelled it never hands
// bytes to the cancelled consumer: any data already pulled from the source is
// stashed for the next consumer, so a torn-down session cannot write a stale
// byte at its dead socket and the next session receives it intact.
func (p *stdinPump) read(ctx context.Context, b []byte) (int, error) {
	p.mu.Lock()
	if len(p.leftover) > 0 {
		n := copy(b, p.leftover)
		p.leftover = p.leftover[n:]
		if len(p.leftover) == 0 {
			p.leftover = nil
		}
		p.mu.Unlock()
		return n, nil
	}
	if p.readErr != nil {
		err := p.readErr
		p.mu.Unlock()
		return 0, err
	}
	p.mu.Unlock()

	// Prefer an already-cancelled context over a byte that happens to be ready
	// on the channel: a tearing-down consumer must not win the race and forward
	// the byte to its dead socket.
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case res := <-p.ch:
		if res.err != nil {
			p.mu.Lock()
			p.readErr = res.err
			p.mu.Unlock()
			return 0, res.err
		}
		// If ctx fired concurrently with the receive, do not deliver to the
		// cancelled consumer — stash the bytes for the next one. leftover is
		// empty here (checked above), so ordering is preserved.
		select {
		case <-ctx.Done():
			p.mu.Lock()
			p.leftover = append(p.leftover, res.data...)
			p.mu.Unlock()
			return 0, ctx.Err()
		default:
		}
		n := copy(b, res.data)
		if n < len(res.data) {
			p.mu.Lock()
			p.leftover = append(p.leftover, res.data[n:]...)
			p.mu.Unlock()
		}
		return n, nil
	}
}

// pumpReader adapts a stdinPump to the io.Reader the dualcopier expects,
// binding reads to a single session's context so that session teardown
// unblocks the copier without disturbing the persistent pump.
type pumpReader struct {
	ctx  context.Context
	pump *stdinPump
}

func (r *pumpReader) Read(b []byte) (int, error) {
	return r.pump.read(r.ctx, b)
}
