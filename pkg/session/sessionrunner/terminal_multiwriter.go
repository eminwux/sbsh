package sessionrunner

import (
	"io"
	"sync"
)

type DynamicMultiWriter struct {
	mu      sync.RWMutex
	writers []io.Writer
}

func NewDynamicMultiWriter(writers ...io.Writer) *DynamicMultiWriter {
	return &DynamicMultiWriter{writers: writers}
}

func (dmw *DynamicMultiWriter) Write(p []byte) (n int, err error) {
	dmw.mu.RLock()
	defer dmw.mu.RUnlock()

	for _, w := range dmw.writers {
		n, err = w.Write(p)
		if err != nil {
			return n, err
		}
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
