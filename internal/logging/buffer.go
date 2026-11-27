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

package logging

import (
	"context"
	"log/slog"
	"sync"
)

// BufferedLogger is a bootstrap logger for work that must run before the
// real log destination is known — `sbsh terminal` resolves logOutput only
// after loading the profile, but the profile loader already logs. Records
// at or above the level are held in memory and later replayed, in order,
// into the real logger with Flush, so they land wherever the terminal's
// own logs go (the log file, or stdout) and nothing is written early to a
// destination the terminal may not end up using.
type BufferedLogger struct {
	*slog.Logger

	buf *recordBuffer
}

type recordBuffer struct {
	mu   sync.Mutex
	recs []slog.Record
}

// NewBufferedLogger returns a BufferedLogger that keeps records at or
// above loglevel (see ParseLevel).
func NewBufferedLogger(loglevel string) *BufferedLogger {
	b := &recordBuffer{}
	return &BufferedLogger{
		Logger: slog.New(&bufferHandler{level: ParseLevel(loglevel), buf: b}),
		buf:    b,
	}
}

// Flush replays every buffered record into dst in arrival order and empties
// the buffer. dst's own level gate still applies.
func (l *BufferedLogger) Flush(ctx context.Context, dst *slog.Logger) {
	l.buf.mu.Lock()
	recs := l.buf.recs
	l.buf.recs = nil
	l.buf.mu.Unlock()
	h := dst.Handler()
	for _, r := range recs {
		if h.Enabled(ctx, r.Level) {
			_ = h.Handle(ctx, r)
		}
	}
}

// bufferHandler stores records for BufferedLogger. WithAttrs/WithGroup
// return the same handler: ReformatHandler, the only replay target, prints
// record attrs only, so logger-scoped attrs would be dropped on replay
// anyway.
type bufferHandler struct {
	level slog.Level
	buf   *recordBuffer
}

func (h *bufferHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	return lvl >= h.level
}

func (h *bufferHandler) Handle(_ context.Context, r slog.Record) error {
	h.buf.mu.Lock()
	h.buf.recs = append(h.buf.recs, r.Clone())
	h.buf.mu.Unlock()
	return nil
}

func (h *bufferHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *bufferHandler) WithGroup(string) slog.Handler      { return h }
