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
	"errors"
	"io"
	"testing"
)

type errWriter struct {
	err error
}

func (e *errWriter) Write(_ []byte) (int, error) {
	return 0, e.err
}

func TestDynamicMultiWriter_DropsFailingWriterAndContinues(t *testing.T) {
	good := &bytes.Buffer{}
	bad := &errWriter{err: errors.New("broken pipe")}
	other := &bytes.Buffer{}

	dmw := NewDynamicMultiWriter(nil, good, bad, other)

	n, err := dmw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write returned error with healthy writers present: %v", err)
	}
	if n != len("hello") {
		t.Fatalf("Write returned n=%d, want %d", n, len("hello"))
	}

	if got := good.String(); got != "hello" {
		t.Errorf("good writer got %q, want %q", got, "hello")
	}
	if got := other.String(); got != "hello" {
		t.Errorf("other writer got %q, want %q", got, "hello")
	}

	dmw.mu.RLock()
	count := len(dmw.writers)
	hasBad := false
	for _, w := range dmw.writers {
		if w == bad {
			hasBad = true
			break
		}
	}
	dmw.mu.RUnlock()
	if count != 2 {
		t.Errorf("after write, writer count = %d, want 2 (failing writer should be dropped)", count)
	}
	if hasBad {
		t.Errorf("failing writer was not dropped from the slice")
	}

	good.Reset()
	other.Reset()
	if _, errAgain := dmw.Write([]byte("again")); errAgain != nil {
		t.Fatalf("second Write returned error: %v", errAgain)
	}
	if got := good.String(); got != "again" {
		t.Errorf("good writer second write got %q, want %q", got, "again")
	}
	if got := other.String(); got != "again" {
		t.Errorf("other writer second write got %q, want %q", got, "again")
	}
}

func TestDynamicMultiWriter_AllFailingReturnsErr(t *testing.T) {
	bad1 := &errWriter{err: errors.New("dead 1")}
	bad2 := &errWriter{err: errors.New("dead 2")}

	dmw := NewDynamicMultiWriter(nil, bad1, bad2)

	_, err := dmw.Write([]byte("x"))
	if !errors.Is(err, ErrAllWritersFailed) {
		t.Fatalf("Write err = %v, want ErrAllWritersFailed", err)
	}

	dmw.mu.RLock()
	count := len(dmw.writers)
	dmw.mu.RUnlock()
	if count != 0 {
		t.Errorf("after all-failed write, writer count = %d, want 0", count)
	}
}

func TestDynamicMultiWriter_NoWritersReturnsNil(t *testing.T) {
	dmw := NewDynamicMultiWriter(nil)
	n, err := dmw.Write([]byte("noop"))
	if err != nil {
		t.Fatalf("Write on empty multiwriter returned err: %v", err)
	}
	if n != len("noop") {
		t.Fatalf("Write returned n=%d, want %d", n, len("noop"))
	}
}

func TestDynamicMultiWriter_RemoveAfterFailureIsNoop(t *testing.T) {
	good := &bytes.Buffer{}
	bad := &errWriter{err: errors.New("dead")}
	dmw := NewDynamicMultiWriter(nil, good, bad)

	if _, err := dmw.Write([]byte("a")); err != nil {
		t.Fatalf("Write returned err: %v", err)
	}

	// bad has already been auto-removed; an explicit Remove must not panic
	// or wedge the slice. The Remove logger emits a warning but the call
	// returns normally.
	dmw.Remove(bad)

	dmw.mu.RLock()
	count := len(dmw.writers)
	dmw.mu.RUnlock()
	if count != 1 {
		t.Errorf("writer count after redundant Remove = %d, want 1", count)
	}
}

// Sanity: io.Writer still satisfied.
var _ io.Writer = (*DynamicMultiWriter)(nil)
