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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdoutPipe replaces sr.stdout with the write end of a fresh pipe so
// the test can inspect the bytes the teardown paths write directly to the
// parent terminal. The returned write end must be closed after the teardown
// call so the drain goroutine sees EOF and the returned channel yields the
// accumulated bytes.
func captureStdoutPipe(t *testing.T, sr *Exec) (*os.File, <-chan []byte) {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { _ = pr.Close(); _ = pw.Close() })
	sr.stdout = pw

	ch := make(chan []byte, 1)
	go func() {
		buf, _ := io.ReadAll(pr)
		ch <- buf
	}()
	return pw, ch
}

// TestRestore_OutputReset_OnDetach asserts the detach teardown path writes the
// output-side normalization sequence (show cursor + DECSCUSR reset + leave
// alt-screen) to the parent terminal. Regression for issue #365.
func TestRestore_OutputReset_OnDetach(t *testing.T) {
	sr, _ := newAttachedExec(t)
	sr.terminalClient = &mockTerminalClient{} // detach succeeds by default
	pw, drained := captureStdoutPipe(t, sr)

	if err := sr.Detach(); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	_ = pw.Close() // EOF the drain so io.ReadAll returns

	got := string(<-drained)
	if !strings.Contains(got, parentTerminalReset) {
		t.Errorf("Detach output missing parent-terminal reset sequence;\n got: %q\nwant substring: %q",
			got, parentTerminalReset)
	}
}

// TestRestore_OutputReset_OnProcessExit asserts the process-exit teardown path
// (EvCmdExited -> Close) writes the output-side normalization sequence to the
// parent terminal. Regression for issue #365.
func TestRestore_OutputReset_OnProcessExit(t *testing.T) {
	sr, _ := newAttachedExec(t)
	pw, drained := captureStdoutPipe(t, sr)

	sockPath := filepath.Join(t.TempDir(), "ctrl.sock")
	if err := os.WriteFile(sockPath, []byte{}, 0o600); err != nil {
		t.Fatalf("seed socket file: %v", err)
	}
	sr.metadata.Spec.SockerCtrl = sockPath

	if err := sr.Close(nil); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_ = pw.Close()

	got := string(<-drained)
	if !strings.Contains(got, parentTerminalReset) {
		t.Errorf("Close output missing parent-terminal reset sequence;\n got: %q\nwant substring: %q",
			got, parentTerminalReset)
	}
}

// TestRestore_OutputReset_Once asserts the reset bytes are written exactly
// once even when both the detach and close teardown paths fire (the live
// sequence: user detaches, then conn-close EvError drives Close). The
// sync.Once that guards restoreParentTerminal must make the second teardown a
// no-op for the output-side write too — emitting the reset twice would
// double-toggle the alt-screen leave (no-op on the first call, then
// re-trigger if some intervening write entered it again, which never happens
// here) and double-show the cursor (idempotent but wasteful).
func TestRestore_OutputReset_Once(t *testing.T) {
	sr, _ := newAttachedExec(t)
	sr.terminalClient = &mockTerminalClient{}
	pw, drained := captureStdoutPipe(t, sr)

	if err := sr.Detach(); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	sockPath := filepath.Join(t.TempDir(), "ctrl.sock")
	if err := os.WriteFile(sockPath, []byte{}, 0o600); err != nil {
		t.Fatalf("seed socket file: %v", err)
	}
	sr.metadata.Spec.SockerCtrl = sockPath
	if err := sr.Close(nil); err != nil {
		t.Fatalf("Close after Detach: %v", err)
	}
	_ = pw.Close()

	got := string(<-drained)
	if n := strings.Count(got, parentTerminalReset); n != 1 {
		t.Errorf("reset sequence appeared %d times after Detach+Close; want exactly 1\n got: %q",
			n, got)
	}
}
