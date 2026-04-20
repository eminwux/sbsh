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
	"log/slog"
	"net"
	"sync"
)

// defaultSubscriberBufferBytes bounds how far a Subscribe stream can fall
// behind the PTY reader before the subscriber is disconnected. A slow or
// abandoned subscriber must never be able to stall the shared fan-out.
const defaultSubscriberBufferBytes = 1 << 20 // 1 MiB

// laggedNotice is appended to a subscriber stream when the ring overflows
// so a caller can distinguish disconnection from a clean EOF.
var laggedNotice = []byte("\r\n[sbsh: subscriber lagged, disconnecting]\r\n")

// subscriberWriter absorbs PTY output from the multiwriter into a bounded
// in-memory ring and forwards it to an external net.Conn on a dedicated
// goroutine. The Write path never blocks: on overflow the subscriber is
// marked lagged, detached, and its conn is closed.
type subscriberWriter struct {
	mu       sync.Mutex
	cond     *sync.Cond
	buf      bytes.Buffer
	maxBytes int
	closed   bool
	lagged   bool
	conn     net.Conn
	onDetach func()
	logger   *slog.Logger
}

func newSubscriberWriter(conn net.Conn, maxBytes int, onDetach func(), logger *slog.Logger) *subscriberWriter {
	if maxBytes <= 0 {
		maxBytes = defaultSubscriberBufferBytes
	}
	s := &subscriberWriter{
		maxBytes: maxBytes,
		conn:     conn,
		onDetach: onDetach,
		logger:   logger,
	}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// Write is called by DynamicMultiWriter on the PTY reader goroutine. It
// enqueues bytes into the bounded ring and returns immediately. When the
// ring would exceed maxBytes the subscriber is marked lagged+closed and
// the underlying conn is closed so the drain goroutine unblocks and
// detaches. Write always reports success to the fan-out so one bad
// subscriber cannot abort output to other attachers or the capture file.
func (s *subscriberWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return len(p), nil
	}
	if s.buf.Len()+len(p) > s.maxBytes {
		s.lagged = true
		s.closed = true
		s.cond.Broadcast()
		s.mu.Unlock()
		_ = s.conn.Close()
		return len(p), nil
	}
	_, _ = s.buf.Write(p)
	s.cond.Signal()
	s.mu.Unlock()
	return len(p), nil
}

// Close requests a graceful shutdown. It signals drain so any pending
// bytes are flushed to the conn; drain itself closes the conn on exit.
// Safe to call repeatedly.
func (s *subscriberWriter) Close() error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.cond.Broadcast()
	}
	s.mu.Unlock()
	return nil
}

// Run drains the ring to the conn on a dedicated goroutine. It exits
// when the conn is closed, the peer goes away, or Close is called.
// Always invokes onDetach on exit so the subscriber is removed from
// the fan-out exactly once.
func (s *subscriberWriter) Run() {
	defer func() {
		if s.onDetach != nil {
			s.onDetach()
		}
	}()
	for {
		s.mu.Lock()
		for s.buf.Len() == 0 && !s.closed {
			s.cond.Wait()
		}
		if s.buf.Len() == 0 && s.closed {
			lagged := s.lagged
			s.mu.Unlock()
			if lagged {
				_, _ = s.conn.Write(laggedNotice)
			}
			_ = s.conn.Close()
			return
		}
		chunk := make([]byte, s.buf.Len())
		_, _ = s.buf.Read(chunk)
		s.mu.Unlock()

		if _, err := s.conn.Write(chunk); err != nil {
			s.logger.Debug("subscriber conn write failed; closing", "err", err)
			s.mu.Lock()
			s.closed = true
			s.mu.Unlock()
			_ = s.conn.Close()
			return
		}
	}
}
