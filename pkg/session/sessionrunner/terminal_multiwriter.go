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
