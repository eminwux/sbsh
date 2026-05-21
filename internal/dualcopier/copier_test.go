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

package dualcopier

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/internal/filter"
)

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// scriptReader returns scripted (chunk, err) pairs, then io.EOF.
type scriptReader struct {
	chunks [][]byte
	errs   []error
	i      int
}

func (s *scriptReader) Read(p []byte) (int, error) {
	if s.i >= len(s.chunks) {
		return 0, io.EOF
	}
	n := copy(p, s.chunks[s.i])
	err := s.errs[s.i]
	s.i++
	return n, err
}

// faultyWriter returns a scripted (n, err) for every Write.
type faultyWriter struct {
	n   int
	err error
}

func (w faultyWriter) Write(p []byte) (int, error) { return w.n, w.err }

// scriptedFilter returns a fixed Process result on every call.
type scriptedFilter struct {
	out    []byte
	nwrite int
	ncons  int
	err    error
}

func (f scriptedFilter) Process([]byte, int) ([]byte, int, int, error) {
	return f.out, f.nwrite, f.ncons, f.err
}

func TestNewCopier(t *testing.T) {
	c := NewCopier(context.Background(), noopLogger())
	if c == nil || c.ctx == nil || c.errGroup == nil {
		t.Fatalf("NewCopier returned incomplete Copier: %+v", c)
	}
}

func TestReadWriteBytes_NoFilterPassthrough(t *testing.T) {
	c := NewCopier(context.Background(), noopLogger())
	r := &scriptReader{chunks: [][]byte{[]byte("hello")}, errs: []error{nil}}
	var w bytes.Buffer
	if err := c.readWriteBytes(r, &w, nil); err != nil {
		t.Fatalf("readWriteBytes error: %v", err)
	}
	if w.String() != "hello" {
		t.Errorf("written = %q, want hello", w.String())
	}
}

func TestReadWriteBytes_FilterPassthrough(t *testing.T) {
	c := NewCopier(context.Background(), noopLogger())
	// ClientEscapeFilter on plain text returns out=nil, nwrite=n → in-place
	// window write path.
	f := filter.NewClientEscapeFilter(0, false, nil)
	r := &scriptReader{chunks: [][]byte{[]byte("plain text")}, errs: []error{nil}}
	var w bytes.Buffer
	if err := c.readWriteBytes(r, &w, f); err != nil {
		t.Fatalf("readWriteBytes error: %v", err)
	}
	if w.String() != "plain text" {
		t.Errorf("written = %q, want 'plain text'", w.String())
	}
}

func TestReadWriteBytes_FilterEmitsAlternateSlice(t *testing.T) {
	c := NewCopier(context.Background(), noopLogger())
	// Single ^] then a second ^] should make the filter emit a BEL (alternate
	// slice, out != nil) on the second byte.
	f := filter.NewClientEscapeFilter(0, false, func() {})
	r := &scriptReader{chunks: [][]byte{{0x1d, 0x1d}}, errs: []error{nil}}
	var w bytes.Buffer
	if err := c.readWriteBytes(r, &w, f); err != nil {
		t.Fatalf("readWriteBytes error: %v", err)
	}
	// First ^] forwarded, second swallowed and replaced with BEL.
	if !bytes.Equal(w.Bytes(), []byte{0x1d, 0x07}) {
		t.Errorf("written = %v, want [0x1d 0x07]", w.Bytes())
	}
}

func TestReadWriteBytes_FilterConsumesWithoutWriting(t *testing.T) {
	c := NewCopier(context.Background(), noopLogger())
	// nwrite == 0, ncons == n → advance without writing.
	f := scriptedFilter{out: nil, nwrite: 0, ncons: 4}
	r := &scriptReader{chunks: [][]byte{[]byte("abcd")}, errs: []error{nil}}
	var w bytes.Buffer
	if err := c.readWriteBytes(r, &w, f); err != nil {
		t.Fatalf("readWriteBytes error: %v", err)
	}
	if w.Len() != 0 {
		t.Errorf("written %d bytes, want 0 (consume-only)", w.Len())
	}
}

func TestReadWriteBytes_ZeroConsumeSafetyFallback(t *testing.T) {
	c := NewCopier(context.Background(), noopLogger())
	// Filter that consumes and writes nothing → infinite-loop guard forwards
	// 1 byte at a time.
	f := scriptedFilter{out: nil, nwrite: 0, ncons: 0}
	r := &scriptReader{chunks: [][]byte{[]byte("xy")}, errs: []error{nil}}
	var w bytes.Buffer
	if err := c.readWriteBytes(r, &w, f); err != nil {
		t.Fatalf("readWriteBytes error: %v", err)
	}
	if w.String() != "xy" {
		t.Errorf("written = %q, want xy (fallback 1-byte forwarding)", w.String())
	}
}

func TestReadWriteBytes_FilterError(t *testing.T) {
	c := NewCopier(context.Background(), noopLogger())
	boom := errors.New("filter boom")
	f := scriptedFilter{err: boom}
	r := &scriptReader{chunks: [][]byte{[]byte("data")}, errs: []error{nil}}
	if err := c.readWriteBytes(r, io.Discard, f); !errors.Is(err, boom) {
		t.Fatalf("readWriteBytes err = %v, want filter boom", err)
	}
}

func TestReadWriteBytes_NwriteExceedsOutLength(t *testing.T) {
	c := NewCopier(context.Background(), noopLogger())
	f := scriptedFilter{out: []byte{1, 2}, nwrite: 5, ncons: 1}
	r := &scriptReader{chunks: [][]byte{[]byte("data")}, errs: []error{nil}}
	err := c.readWriteBytes(r, io.Discard, f)
	if err == nil {
		t.Fatal("expected error for nwrite exceeding out length")
	}
}

func TestReadWriteBytes_NwriteExceedsAvailableInput(t *testing.T) {
	c := NewCopier(context.Background(), noopLogger())
	// out == nil but nwrite > remaining input → "exceeds available input".
	f := scriptedFilter{out: nil, nwrite: 99, ncons: 1}
	r := &scriptReader{chunks: [][]byte{[]byte("ab")}, errs: []error{nil}}
	err := c.readWriteBytes(r, io.Discard, f)
	if err == nil {
		t.Fatal("expected error for nwrite exceeding available input")
	}
}

func TestReadWriteBytes_WriteError(t *testing.T) {
	c := NewCopier(context.Background(), noopLogger())
	boom := errors.New("write boom")
	r := &scriptReader{chunks: [][]byte{[]byte("data")}, errs: []error{nil}}
	if err := c.readWriteBytes(r, faultyWriter{n: 0, err: boom}, nil); !errors.Is(err, boom) {
		t.Fatalf("readWriteBytes err = %v, want write boom", err)
	}
}

func TestReadWriteBytes_ShortWriteNoProgress(t *testing.T) {
	c := NewCopier(context.Background(), noopLogger())
	r := &scriptReader{chunks: [][]byte{[]byte("data")}, errs: []error{nil}}
	err := c.readWriteBytes(r, faultyWriter{n: 0, err: nil}, nil)
	if err == nil || err.Error() != "short write with no progress" {
		t.Fatalf("readWriteBytes err = %v, want 'short write with no progress'", err)
	}
}

func TestReadWriteBytes_ReadError(t *testing.T) {
	c := NewCopier(context.Background(), noopLogger())
	boom := errors.New("read boom")
	// n == 0 with a read error → error returned without writing.
	r := &scriptReader{chunks: [][]byte{nil}, errs: []error{boom}}
	if err := c.readWriteBytes(r, io.Discard, nil); !errors.Is(err, boom) {
		t.Fatalf("readWriteBytes err = %v, want read boom", err)
	}
}

func TestRunReadWriter_ContextCancelledReturnsCtxErr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	c := NewCopier(ctx, noopLogger())

	ready := make(chan struct{})
	// A blocking reader so the only way out is the ctx.Done() select arm.
	pr, pw := io.Pipe()
	defer pw.Close()
	defer pr.Close()

	err := c.runReadWriter(pr, io.Discard, ready, func() {}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runReadWriter err = %v, want context.Canceled", err)
	}
	select {
	case <-ready:
	default:
		t.Error("ready channel was not closed")
	}
}

func TestRunReadWriter_ReadErrorInvokesErrFunc(t *testing.T) {
	c := NewCopier(context.Background(), noopLogger())
	ready := make(chan struct{})
	called := false
	boom := errors.New("read boom")
	r := &scriptReader{chunks: [][]byte{nil}, errs: []error{boom}}

	err := c.runReadWriter(r, io.Discard, ready, func() { called = true }, nil)
	if !errors.Is(err, boom) {
		t.Fatalf("runReadWriter err = %v, want read boom", err)
	}
	if !called {
		t.Error("errFunc was not invoked on read error")
	}
}

func TestRunCopier_AndCopierManager_ErrGroupPath(t *testing.T) {
	c := NewCopier(context.Background(), noopLogger())
	ready := make(chan struct{})
	// Reader yields one chunk then EOF → runReadWriter writes it, then the
	// EOF read terminates the goroutine via the error path.
	r := &scriptReader{chunks: [][]byte{[]byte("payload")}, errs: []error{nil}}
	var w bytes.Buffer

	c.RunCopier(r, &w, ready, func() {}, nil)
	<-ready // goroutine started

	// CopierManager blocks until the errgroup finishes (the EOF error path).
	c.CopierManager(nil, func() {})
	if w.String() != "payload" {
		t.Errorf("written = %q, want payload", w.String())
	}
}

func TestCopierManager_ContextCancelledNilConn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := NewCopier(ctx, noopLogger())
	cancel()

	finished := false
	c.CopierManager(nil, func() { finished = true })
	if !finished {
		t.Error("finish() was not called on context cancellation with nil conn")
	}
}

func TestCopierManager_ContextCancelledRealConn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := NewCopier(ctx, noopLogger())

	sock := filepath.Join(t.TempDir(), "s.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	dialed, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	uc := dialed.(*net.UnixConn)
	defer uc.Close()

	cancel()
	finished := false
	c.CopierManager(uc, func() { finished = true })
	if !finished {
		t.Error("finish() was not called on context cancellation with real conn")
	}
}
