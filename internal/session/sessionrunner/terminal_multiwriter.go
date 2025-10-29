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

package sessionrunner

import (
	"io"
	"log/slog"
	"sync"
)

type DynamicMultiWriter struct {
	mu      sync.RWMutex
	writers []io.Writer
	logger  *slog.Logger
}

func NewDynamicMultiWriter(logger *slog.Logger, writers ...io.Writer) *DynamicMultiWriter {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &DynamicMultiWriter{writers: writers, logger: logger}
}

func (dmw *DynamicMultiWriter) Write(p []byte) (int, error) {
	dmw.logger.Debug("acquiring read lock for writers")
	dmw.mu.RLock()
	ws := append([]io.Writer(nil), dmw.writers...)
	dmw.mu.RUnlock()
	dmw.logger.Debug("read lock released, writers snapshot taken", "writers_count", len(ws), "write_len", len(p))

	for i, w := range ws {
		dmw.logger.Debug("writing to writer", "index", i, "write_len", len(p))
		n, err := w.Write(p)
		if err != nil {
			dmw.logger.Error("write to writer failed", "index", i, "error", err)
			return 0, err
		}
		if n != len(p) {
			dmw.logger.Warn("partial write to writer", "index", i, "expected", len(p), "actual", n)
			// continue to next writer, but still return full len(p) if all succeed
		} else {
			dmw.logger.Debug("write to writer succeeded", "index", i, "bytes_written", n)
		}
	}
	dmw.logger.Info("write completed for all writers", "writers_count", len(ws), "bytes", len(p))
	return len(p), nil
}

func (dmw *DynamicMultiWriter) Add(w io.Writer) {
	dmw.logger.Debug("acquiring write lock to add writer")
	dmw.mu.Lock()
	defer dmw.mu.Unlock()
	dmw.writers = append(dmw.writers, w)
	dmw.logger.Info("writer added", "total_writers", len(dmw.writers))
}

func (dmw *DynamicMultiWriter) Remove(w io.Writer) {
	dmw.logger.Debug("acquiring write lock to remove writer")
	dmw.mu.Lock()
	defer dmw.mu.Unlock()
	found := false
	newWriters := dmw.writers[:0]
	for _, writer := range dmw.writers {
		if writer != w {
			newWriters = append(newWriters, writer)
		} else {
			found = true
		}
	}
	dmw.writers = newWriters
	if found {
		dmw.logger.Info("writer removed", "remaining_writers", len(dmw.writers))
	} else {
		dmw.logger.Warn("writer to remove not found", "remaining_writers", len(dmw.writers))
	}
}
