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

//go:build linux

package terminalrunner

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestSignalForwarder_TargetsProcessGroup verifies the forwarder delivers
// the observed signal to -pgid, reaching the process-group leader AND a
// sibling (subshell) in the same pgroup. Uses SIGHUP to avoid colliding
// with the Go test runner's SIGTERM/SIGINT handling.
func TestSignalForwarder_TargetsProcessGroup(t *testing.T) {
	// Leader script: installs a HUP trap, spawns a subshell sibling in the
	// same pgroup that also traps HUP, then waits on it. On SIGHUP to the
	// pgroup, both traps fire.
	dir := t.TempDir()
	leaderMarker := filepath.Join(dir, "leader")
	siblingMarker := filepath.Join(dir, "sibling")

	// Subshell uses "sleep & wait" so the shell does not tail-call-exec
	// sleep (dash optimizes "exec sleep" when sleep is the last command,
	// which would leave no shell alive to run the SIGHUP trap).
	// Markers are written to disk so the test does not depend on stdout
	// flush ordering of the subshell vs. the leader.
	script := `
trap 'touch "$LEADER"' HUP
(trap 'touch "$SIBLING"; exit 0' HUP; sleep 10 & wait) &
child=$!
echo ready
wait $child
echo done
exit 0
`
	leader := exec.Command("/bin/sh", "-c", script)
	leader.Env = append(os.Environ(), "LEADER="+leaderMarker, "SIBLING="+siblingMarker)
	leader.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	stdout, err := leader.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := leader.Start(); err != nil {
		t.Fatalf("start leader: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-leader.Process.Pid, syscall.SIGKILL)
		_, _ = leader.Process.Wait()
	})
	pgid := leader.Process.Pid

	reader := bufio.NewReader(stdout)
	if line, _ := reader.ReadString('\n'); line != "ready\n" {
		t.Fatalf("leader did not emit ready; got %q", line)
	}

	stop := startSignalForwarder(discardLogger(), pgid)
	t.Cleanup(stop)

	// Send SIGHUP to our own process. signal.Notify in the forwarder
	// catches it; the relay targets -pgid; default termination is
	// suppressed because the handler is registered.
	if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatalf("self-kill SIGHUP: %v", err)
	}

	_ = collectLinesUntil(t, reader, 3*time.Second, "done")

	if !fileExists(leaderMarker) {
		t.Fatalf("leader did not receive SIGHUP (marker %s absent)", leaderMarker)
	}
	if !fileExists(siblingMarker) {
		t.Fatalf("sibling did not receive SIGHUP (marker %s absent)", siblingMarker)
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// collectLinesUntil reads lines until the sentinel line (trimmed) is seen or
// the deadline elapses; returns all accumulated output.
func collectLinesUntil(t *testing.T, r *bufio.Reader, d time.Duration, sentinel string) string {
	t.Helper()
	deadline := time.Now().Add(d)
	var sb strings.Builder
	done := make(chan string, 1)
	go func() {
		for time.Now().Before(deadline) {
			line, err := r.ReadString('\n')
			if line != "" {
				sb.WriteString(line)
				if strings.TrimSpace(line) == sentinel {
					done <- sb.String()
					return
				}
			}
			if err != nil {
				if err == io.EOF {
					done <- sb.String()
					return
				}
				done <- sb.String()
				return
			}
		}
		done <- sb.String()
	}()
	select {
	case out := <-done:
		return out
	case <-time.After(d + 500*time.Millisecond):
		t.Fatalf("timed out reading lines; partial=%q", sb.String())
		return sb.String()
	}
}
