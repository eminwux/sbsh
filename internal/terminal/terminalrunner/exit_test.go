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
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

// runWatchChildExit runs watchChildExit on sr and returns the error it
// publishes on closeReqCh.
func runWatchChildExit(t *testing.T, sr *Exec) error {
	t.Helper()
	go sr.watchChildExit()
	select {
	case err := <-sr.closeReqCh:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("watchChildExit did not publish on closeReqCh within timeout")
		return nil
	}
}

func requireExitError(t *testing.T, err error) *ExitError {
	t.Helper()
	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("err %v (%T) is not an *ExitError", err, err)
	}
	return ee
}

// exitDocAfterClose persists Exited the way Close does and returns the
// on-disk document plus its raw JSON.
func exitDocAfterClose(t *testing.T, sr *Exec) (api.TerminalDoc, string) {
	t.Helper()
	dir := t.TempDir()
	sr.metadataMu.Lock()
	sr.metadata.Spec.MetadataDir = dir
	sr.metadataMu.Unlock()
	if err := sr.markExited(); err != nil {
		t.Fatalf("markExited: %v", err)
	}
	path := filepath.Join(dir, "metadata.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	return readDocAt(t, path), string(raw)
}

func TestWatchChildExit_NonInit_ExitCode3(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 3")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	sr := newWatchChildExitTestExec(t, cmd)
	err := runWatchChildExit(t, sr)

	ee := requireExitError(t, err)
	if ee.Code == nil || *ee.Code != 3 || ee.Signal != "" {
		t.Fatalf("ExitError = {Code:%v Signal:%q}, want code 3", ee.Code, ee.Signal)
	}
	if got := err.Error(); got != "shell process exited: exit status 3" {
		t.Fatalf("Error() = %q, want unchanged text", got)
	}
	var xe *exec.ExitError
	if !errors.As(err, &xe) || xe.ExitCode() != 3 {
		t.Fatalf("errors.As(*exec.ExitError) no longer works through ExitError: %v", err)
	}

	doc, raw := exitDocAfterClose(t, sr)
	if doc.Status.State != api.Exited || doc.Status.ExitCode == nil || *doc.Status.ExitCode != 3 {
		t.Fatalf("doc status = %+v, want Exited with exitCode 3", doc.Status)
	}
	if doc.Status.ExitedAt.IsZero() || doc.Status.ExitSignal != "" {
		t.Fatalf("doc exitedAt=%v exitSignal=%q, want set / empty", doc.Status.ExitedAt, doc.Status.ExitSignal)
	}
	if !strings.Contains(raw, `"exitCode": 3`) || !strings.Contains(raw, `"exitedAt"`) {
		t.Fatalf("raw document missing exitCode/exitedAt:\n%s", raw)
	}
}

func TestWatchChildExit_NonInit_SIGKILL(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "kill -9 $$")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	sr := newWatchChildExitTestExec(t, cmd)
	ee := requireExitError(t, runWatchChildExit(t, sr))
	if ee.Code != nil || ee.Signal != "SIGKILL" {
		t.Fatalf("ExitError = {Code:%v Signal:%q}, want nil code and SIGKILL", ee.Code, ee.Signal)
	}

	doc, raw := exitDocAfterClose(t, sr)
	if doc.Status.ExitSignal != "SIGKILL" || doc.Status.ExitCode != nil || doc.Status.ExitedAt.IsZero() {
		t.Fatalf("doc status = %+v, want exitSignal SIGKILL, no exitCode, exitedAt set", doc.Status)
	}
	if strings.Contains(raw, `"exitCode"`) {
		t.Fatalf("raw document has exitCode for a signaled death:\n%s", raw)
	}
}

func TestWatchChildExit_NonInit_CleanExitIsExitErrorCode0(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	sr := newWatchChildExitTestExec(t, cmd)
	err := runWatchChildExit(t, sr)
	ee := requireExitError(t, err)
	if ee.Code == nil || *ee.Code != 0 || ee.Signal != "" {
		t.Fatalf("ExitError = {Code:%v Signal:%q}, want code 0", ee.Code, ee.Signal)
	}
	if got := err.Error(); got != "shell process exited (code 0)" {
		t.Fatalf("Error() = %q, want unchanged text", got)
	}
}

// TestWatchChildExit_CtxCancelIsNotExitError asserts the teardown path that
// never observed a wait status stays a plain error and records nothing.
func TestWatchChildExit_CtxCancelIsNotExitError(t *testing.T) {
	sr := newWatchChildExitTestExec(t, nil)
	sr.initMode = true
	sr.reaper = newReaper(newDiscardLogger())
	sr.ctxCancel()
	// closeReqCh's send is ctx-guarded too; the buffered channel takes it.
	go sr.watchChildExit()
	select {
	case <-sr.childDoneCh:
	case <-time.After(5 * time.Second):
		t.Fatal("watchChildExit did not finish after ctx cancel")
	}
	select {
	case err := <-sr.closeReqCh:
		var ee *ExitError
		if errors.As(err, &ee) {
			t.Fatalf("ctx-cancel teardown produced an ExitError: %v", err)
		}
	default:
	}
	if sr.exitInfo != nil {
		t.Fatalf("ctx-cancel teardown recorded exit info %+v", sr.exitInfo)
	}
}

// TestClose_ExitDetailsLateWrite covers the Stop/ctx ordering: Exited is
// written before the child dies, then the details arrive and the second
// write persists them. A running terminal's document has no exitCode.
func TestClose_ExitDetailsLateWrite(t *testing.T) {
	dir := t.TempDir()
	sr := newMetadataExec(t, t.TempDir(), dir, "term-late")
	if err := sr.updateTerminalState(api.Ready); err != nil {
		t.Fatalf("updateTerminalState: %v", err)
	}
	path := filepath.Join(dir, "metadata.json")
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "exit") {
		t.Fatalf("running terminal document carries exit fields:\n%s", raw)
	}

	if err := sr.markExited(); err != nil {
		t.Fatalf("markExited: %v", err)
	}
	if doc := readDocAt(t, path); doc.Status.State != api.Exited || doc.Status.ExitCode != nil {
		t.Fatalf("after markExited status = %+v, want Exited without exitCode yet", doc.Status)
	}

	code := 143
	sr.recordExit(&ExitError{Code: &code})
	sr.persistLateExitInfo()
	doc := readDocAt(t, path)
	if doc.Status.ExitCode == nil || *doc.Status.ExitCode != 143 || doc.Status.ExitedAt.IsZero() {
		t.Fatalf("after late write status = %+v, want exitCode 143 and exitedAt", doc.Status)
	}

	// A second late write with nothing new is a no-op.
	before, _ := os.ReadFile(path)
	sr.persistLateExitInfo()
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("persistLateExitInfo rewrote the document with nothing new")
	}
}

// TestMarkExited_CommitsInMemoryWhenWriteFails asserts Close's Exited
// reaches memory even when the directory cannot be written (#345).
func TestMarkExited_CommitsInMemoryWhenWriteFails(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	sr := newMetadataExec(t, t.TempDir(), missing, "term-unwritable")
	sr.metadata.Status.State = api.Ready
	if err := sr.markExited(); err == nil {
		t.Fatal("markExited into a missing dir: want write error")
	}
	md, _ := sr.Metadata()
	if md.Status.State != api.Exited {
		t.Fatalf("in-memory state = %v after failed write, want Exited", md.Status.State)
	}
}
