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
	"os"
	"regexp"
	"testing"
	"time"
)

// TestLifecycle_SpawnAttachWriteReadDetachStop drives the full core flow end
// to end through the real CLI: a terminal is spawned under sbsh, a second
// client attaches with `sb attach`, a command is injected with `sb write` and
// read back via both `sb read` and the live attach stream, the attach client
// detaches with the ^]^] keystroke, and the terminal is stopped gracefully
// with `sb stop`.
func TestLifecycle_SpawnAttachWriteReadDetachStop(t *testing.T) {
	requireBinaries(t, sb, sbsh)

	runPath := newRunPath(t)
	runPathEnv := buildSbRunPathEnv(t, runPath)
	termName := "e2e-lifecycle"

	t.Cleanup(func() {
		runReturningBinary(t, []string{runPathEnv}, sb, "prune", "terminals", "--verbose", "--log-level", "debug")
		runReturningBinary(t, []string{runPathEnv}, sb, "prune", "clients", "--verbose", "--log-level", "debug")
	})

	// --- spawn: boot an interactive shell under sbsh and wait for its prompt.
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
	promptCh := setupPromptDetector(t, pipeExpectR, regexp.MustCompile(`\(sbsh-[^)]+\)`))
	multiW.Add(pipeExpectW)

	retSpawn := make(chan error, 1)
	spawnEnv := append([]string{}, getRandomSbshRunPath(t, runPath))
	runPersistentBinaryPty(t, pts, cmdWaitFunc(t, &retSpawn), spawnEnv, sbsh, "--terminal-name", termName)

	select {
	case <-promptCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for sbsh prompt (spawn)")
	}
	multiW.Remove(pipeExpectW)
	time.Sleep(200 * time.Millisecond)

	// --- attach: a second client connects via `sb attach`. On connect it
	// repaints the current screen, which carries the sbsh prompt — detecting
	// it confirms the attach client is wired to the controller's fan-out.
	ptmxA, ptsA := setupPty(t)
	defer func() { _ = ptmxA.Close() }()

	pipeLogRA, pipeLogWA, _ := os.Pipe()
	defer pipeLogRA.Close()
	defer pipeLogWA.Close()
	multiWA := setupMultiWriter(t, ptmxA, pipeLogWA)
	go logTerminal(t, pipeLogRA)

	pipeExpectRA, pipeExpectWA, _ := os.Pipe()
	defer pipeExpectRA.Close()
	defer pipeExpectWA.Close()
	attachPromptCh := setupPromptDetector(t, pipeExpectRA, regexp.MustCompile(`\(sbsh-[^)]+\)`))
	multiWA.Add(pipeExpectWA)

	retAttach := make(chan error, 1)
	runPersistentBinaryPty(t, ptsA, cmdWaitFunc(t, &retAttach), []string{runPathEnv}, sb, "attach", termName)

	select {
	case <-attachPromptCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for sb attach to repaint the prompt")
	}
	multiWA.Remove(pipeExpectWA)
	time.Sleep(200 * time.Millisecond)

	// --- write/read: inject a command, confirm it round-trips through the
	// capture file (`sb read`) and the live attach stream.
	const marker = "e2e-lifecycle-marker"
	pipeMarkerRA, pipeMarkerWA, _ := os.Pipe()
	defer pipeMarkerRA.Close()
	defer pipeMarkerWA.Close()
	attachMarkerCh := setupPromptDetector(t, pipeMarkerRA, regexp.MustCompile(regexp.QuoteMeta(marker)))
	multiWA.Add(pipeMarkerWA)

	runSilentBinary(t, []string{runPathEnv}, sb, "write", termName, "echo "+marker+"^M")
	time.Sleep(500 * time.Millisecond)

	out := runReturningBinary(t, []string{runPathEnv}, sb, "read", termName)
	if !bytes.Contains(out, []byte(marker)) {
		t.Fatalf("sb read output missing marker %q; got:\n%s", marker, string(out))
	}

	select {
	case <-attachMarkerCh:
	case <-time.After(3 * time.Second):
		t.Fatal("attach client never observed the live command output")
	}
	multiWA.Remove(pipeMarkerWA)

	// --- detach: the ^]^] keystroke detaches the attach client; the sb
	// attach process should exit while the terminal keeps running.
	if _, err := ptmxA.WriteString("\x1d\x1d"); err != nil {
		t.Logf("could not send detach keystroke: %v", err)
	}
	select {
	case <-retAttach:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for sb attach to detach and exit")
	}

	// --- stop: graceful stop of the terminal controller. The spawning sbsh
	// is the recorded process, so it exits once the Stop RPC lands.
	stopOut := runReturningBinary(t, []string{runPathEnv}, sb, "stop", "terminal", termName)
	if !bytes.Contains(stopOut, []byte("stopped")) {
		t.Fatalf("sb stop did not report the terminal stopped; got:\n%s", string(stopOut))
	}
	select {
	case <-retSpawn:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for the terminal controller to exit after stop")
	}
}
