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
	"fmt"
	"io"
	"log/slog"
	"sync"
)

type DynamicMultiWriter struct {
	mu      sync.RWMutex
	writers []io.Writer
}

func NewDynamicMultiWriter(writers ...io.Writer) *DynamicMultiWriter {
	return &DynamicMultiWriter{writers: writers}
}

func (dmw *DynamicMultiWriter) Write(p []byte) (int, error) {
	slog.Debug("pre-lock")
	dmw.mu.RLock()
	ws := append([]io.Writer(nil), dmw.writers...)
	dmw.mu.RUnlock()
	slog.Debug("post-lock")

	for i, w := range ws {
		slog.Debug(fmt.Sprintf("pre-write to %d: %d", i, len(p)))
		if _, err := w.Write(p); err != nil {
			return 0, err
		}
		slog.Debug("post-write")
	}
	return len(p), nil
}

func (dmw *DynamicMultiWriter) Add(w io.Writer) {
	dmw.mu.Lock()
	defer dmw.mu.Unlock()
	dmw.writers = append(dmw.writers, w)
}

func (dmw *DynamicMultiWriter) Remove(w io.Writer) {
	dmw.mu.Lock()
	defer dmw.mu.Unlock()
	newWriters := dmw.writers[:0]
	for _, writer := range dmw.writers {
		if writer != w {
			newWriters = append(newWriters, writer)
		}
	}
	dmw.writers = newWriters
}
