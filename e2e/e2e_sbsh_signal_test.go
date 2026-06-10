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
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// ptyTermios reads the line-discipline state of the pty referred to by fd.
// Reading from the pty master reflects the termios the attached client set on
// the slave, so the test can observe raw->cooked transitions from the parent
// side. (The precondition assertions below confirm the master view tracks the
// slave: it reports raw while attached and canonical after restore.)
func ptyTermios(t *testing.T, fd uintptr) *unix.Termios {
	t.Helper()
	tio, err := unix.IoctlGetTermios(int(fd), unix.TCGETS)
	if err != nil {
		t.Fatalf("IoctlGetTermios: %v", err)
	}
	return tio
}

// isCanonical reports whether the terminal is in cooked mode with echo — the
// "stty -a shows canonical mode with echo" state the issue's AC names.
func isCanonical(tio *unix.Termios) bool {
	return tio.Lflag&unix.ICANON != 0 && tio.Lflag&unix.ECHO != 0
}

// waitClientReady blocks until the attached client logs that it has finished
// attaching and entered its steady-state event loop, or the deadline elapses.
//
// The signal must land in that event loop, not during the earlier WaitReady
// phase: the IO copiers begin relaying bytes while Attach is still in progress
// (so a visible prompt or even a command round-trip is *not* proof of
// readiness), and a signal caught mid-WaitReady takes an early-return path that
// legitimately skips restore and is out of scope here. The controller writes
// "controller ready, entering main event loop" at info level to its per-client
// log under <run_path>/clients/<id>/log exactly when WaitReady unblocks, which
// is the precise, race-free gate.
func waitClientReady(t *testing.T, runPath string, timeout time.Duration) {
	t.Helper()
	const marker = "controller ready, entering main event loop"
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		logs, _ := filepath.Glob(filepath.Join(runPath, "clients", "*", "log"))
		for _, lf := range logs {
			if b, err := os.ReadFile(lf); err == nil && strings.Contains(string(b), marker) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("client did not reach steady-state event loop within %v", timeout)
}

// spawnSbshOnPty starts sbsh attached to pts and returns the process handle so
// the caller can deliver a signal directly to the client. It mirrors
// runPersistentBinaryPty but hands back *os.Process instead of swallowing it.
func spawnSbshOnPty(t *testing.T, pts *os.File, env []string) *os.Process {
	t.Helper()

	bin := resolveBinary(t, sbsh)

	env = append(env,
		`PS1="__P__ "`,
		"TERM=xterm",
		"LANG=C",
		"COLUMNS=120",
		"LINES=40",
	)
	files := []*os.File{pts, pts, pts}
	procAttr := &os.ProcAttr{
		Env:   env,
		Files: files,
		Sys: &syscall.SysProcAttr{
			Setsid:  true,
			Setctty: true,
			Ctty:    0,
		},
	}

	p, err := os.StartProcess(bin, []string{bin}, procAttr)
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}
	return p
}

// TestSbsh_SignalRestoresTerminal asserts that a SIGHUP or SIGQUIT delivered to
// an attached client (tty surviving) drives the ctx-cancel -> restore path so
// the controlling terminal is left in canonical mode with echo, rather than
// stranded in raw mode with DEC private modes still active. Regression test for
// the client trapping only INT/TERM: with HUP/QUIT untrapped they took the
// default terminate disposition and skipped restore entirely.
func TestSbsh_SignalRestoresTerminal(t *testing.T) {
	for _, tc := range []struct {
		name string
		sig  syscall.Signal
	}{
		{"SIGHUP", syscall.SIGHUP},
		{"SIGQUIT", syscall.SIGQUIT},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireBinaries(t, sb, sbsh)
			runPath := newRunPath(t)

			t.Cleanup(func() {
				runPathEnv := buildSbRunPathEnv(t, runPath)
				runReturningBinary(t, []string{runPathEnv}, sb, "prune", "terminals", "--verbose", "--log-level", "debug")
				runReturningBinary(t, []string{runPathEnv}, sb, "prune", "clients", "--verbose", "--log-level", "debug")
			})

			ptmx, pts := setupPty(t)
			defer func() { _ = ptmx.Close() }()

			// Drain the pty master so the client never blocks writing output.
			go func() {
				buf := make([]byte, 4096)
				for {
					if _, err := ptmx.Read(buf); err != nil {
						return
					}
				}
			}()

			env := append([]string{}, getRandomSbshRunPath(t, runPath))
			proc := spawnSbshOnPty(t, pts, env)
			// pts belongs to the child now; drop our copy so the child and the
			// kernel line discipline are the only holders.
			_ = pts.Close()

			waitCh := make(chan error, 1)
			go func() { _, werr := proc.Wait(); waitCh <- werr }()

			// Gate on the client reaching its steady-state event loop before the
			// signal — see waitClientReady for why a prompt/round-trip won't do.
			waitClientReady(t, runPath, 8*time.Second)

			// Precondition: attached client put the terminal into raw mode.
			if isCanonical(ptyTermios(t, ptmx.Fd())) {
				t.Fatalf("expected raw mode while attached, but terminal is canonical")
			}

			// Deliver the signal directly to the client process.
			if err := proc.Signal(tc.sig); err != nil {
				t.Fatalf("signalling %v: %v", tc.sig, err)
			}

			select {
			case <-waitCh:
			case <-time.After(5 * time.Second):
				_ = proc.Kill()
				t.Fatalf("timeout waiting for client to exit after %v", tc.sig)
			}

			// Restore runs during ctx-cancel teardown before the process exits;
			// poll briefly to absorb teardown latency.
			deadline := time.Now().Add(2 * time.Second)
			for {
				if isCanonical(ptyTermios(t, ptmx.Fd())) {
					break
				}
				if !time.Now().Before(deadline) {
					t.Fatalf("terminal not restored to canonical mode after %v", tc.sig)
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}
