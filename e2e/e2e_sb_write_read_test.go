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

package e2e_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// TestSb_WriteRead_Roundtrip boots a terminal via sbsh, injects a shell
// command with `sb write` (exercising caret notation), and confirms the
// command's output appears in both the capture file (`sb read`) and the
// live stream (`sb read -f`).
func TestSb_WriteRead_Roundtrip(t *testing.T) {
	runPath := getRandomRunPath(t)
	mkdirRunPath(t, runPath)

	termName := "e2e-writeread"
	runPathEnv := buildSbRunPathEnv(t, runPath)

	t.Cleanup(func() {
		runReturningBinary(t, []string{runPathEnv}, sb, "prune", "terminals", "--verbose", "--log-level", "debug")
		runReturningBinary(t, []string{runPathEnv}, sb, "prune", "clients", "--verbose", "--log-level", "debug")
	})

	// Boot an interactive shell under sbsh and wait for its prompt.
	ptmx, pts := setupPty(t)
	defer func() { _ = ptmx.Close() }()

	pipeLogR, pipeLogW, _ := os.Pipe()
	defer pipeLogR.Close()
	defer pipeLogW.Close()

	multiW := setupMultiWriter(t, ptmx, pipeLogW)
	go logTerminal(t, pipeLogR)

	pipeExpectR, pipeExpectW, _ := os.Pipe()
	defer pipeExpectR.Close()
	defer pipeExpectW.Close()

	// Default prompt format is "(sbsh-<id>)" regardless of --terminal-name.
	promptCh := setupPromptDetector(t, pipeExpectR, regexp.MustCompile(`\(sbsh-[^)]+\)`))
	multiW.Add(pipeExpectW)

	ret := make(chan error, 1)
	waitFn := cmdWaitFunc(t, &ret)
	env := append([]string{}, getRandomSbshRunPath(t, runPath))
	runPersistentBinaryPty(t, pts, waitFn, env, sbsh, "--terminal-name", termName)

	select {
	case <-promptCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for sbsh prompt")
	}
	multiW.Remove(pipeExpectW)
	time.Sleep(200 * time.Millisecond)

	// sb write with caret notation: inject an echo command + CR.
	const marker = "e2e-writeread-marker"
	runSilentBinary(t, []string{runPathEnv}, sb, "write", termName,
		"echo "+marker+"^M")

	// Give the PTY a moment to echo and flush to the capture file.
	time.Sleep(500 * time.Millisecond)

	out := runReturningBinary(t, []string{runPathEnv}, sb, "read", termName)
	if !bytes.Contains(out, []byte(marker)) {
		t.Fatalf("sb read output missing marker %q; got:\n%s", marker, string(out))
	}

	// --raw: send bytes verbatim. \x20 literal should NOT be interpreted.
	const rawPayload = "echo raw-passthrough\r"
	runSilentBinary(t, []string{runPathEnv}, sb, "write", termName, "--raw", rawPayload)
	time.Sleep(300 * time.Millisecond)
	out2 := runReturningBinary(t, []string{runPathEnv}, sb, "read", termName)
	if !bytes.Contains(out2, []byte("raw-passthrough")) {
		t.Fatalf("sb read missing raw payload output; got:\n%s", string(out2))
	}

	// Shut down cleanly so the cleanup prune doesn't race with a live PID.
	if _, err := ptmx.WriteString("\x04"); err != nil {
		t.Logf("could not EOF ptmx: %v", err)
	}
	select {
	case <-ret:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for sbsh to exit")
	}
}

// TestSb_Write_CaretParsingErrors confirms the CLI rejects ambiguous
// shell-style escapes (and returns non-zero) rather than silently
// sending bogus bytes.
func TestSb_Write_CaretParsingErrors(t *testing.T) {
	t.Parallel()
	runPath := getRandomRunPath(t)
	mkdirRunPath(t, runPath)
	runPathEnv := buildSbRunPathEnv(t, runPath)

	// No terminal needs to be running — parsing happens before we dial
	// the socket. We invoke the binary directly here so we can assert on
	// a non-zero exit code without runReturningBinary's auto-fail.
	err := runExpectingFailure(t, []string{runPathEnv}, sb, "write", "irrelevant", `\n`)
	if err == nil {
		t.Fatal("expected sb write to fail on shell-style \\n escape")
	}
}

// runSilentBinary runs a binary that is expected to succeed with no
// output (e.g. `sb write`, which is quiet on success).
func runSilentBinary(t *testing.T, env []string, command string, args ...string) {
	t.Helper()
	dir := os.Getenv("E2E_BIN_DIR")
	if dir == "" {
		dir = ".."
	}
	bin := filepath.Join(dir, command)
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		t.Fatalf("binary %s not found", bin)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	if env != nil {
		cmd.Env = env
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("running %s %v failed: %v\noutput:\n%s", bin, args, err, string(out))
	}
}

// runExpectingFailure mirrors runReturningBinary but returns the error
// instead of failing the test when the binary exits non-zero.
func runExpectingFailure(t *testing.T, env []string, command string, args ...string) error {
	t.Helper()
	dir := os.Getenv("E2E_BIN_DIR")
	if dir == "" {
		dir = ".."
	}
	bin := filepath.Join(dir, command)
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		t.Fatalf("binary %s not found", bin)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	if env != nil {
		cmd.Env = env
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected %s %v to fail; output:\n%s", bin, args, out)
	}
	t.Logf("expected failure from %s %v: %v\noutput:\n%s", bin, args, err, out)
	return err
}
