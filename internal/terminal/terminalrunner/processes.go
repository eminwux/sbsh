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
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sort"
	"sync"
	"syscall"

	"github.com/eminwux/sbsh/pkg/api"
	"golang.org/x/sys/unix"
)

// processDrainBufSize is the read buffer used by each process's drain goroutine
// when copying the process's socketpair output into its capture file. Matches
// the PTY reader's buffer size in terminal.go.
const processDrainBufSize = 8192

// procState holds the runtime state for one supervised process spawned from a
// non-empty TerminalSpec.Processes. Each process runs with socketpair stdio
// (not a PTY); the parent end is drained into the process's capture file by a
// per-process goroutine. Phase 2 owns spawn + drain + exit observation;
// multiplexing the process onto the operator's attach session is phase 3.
type procState struct {
	spec      api.ProcessSpec
	cmd       *exec.Cmd
	parentEnd *os.File // parent side of the socketpair; child side is dup'd to the process's 0/1/2

	// captureFile backs the drain goroutine's write target. nil when the
	// process declared no CaptureFile — output is then drained to discard so a
	// full socket buffer never blocks the process.
	captureFile      *os.File
	closeCaptureOnce sync.Once

	// doneCh is closed once the process's exit has been observed (via the reaper
	// in PID-1 init mode, or os/exec.Wait otherwise). exitErr carries the
	// observed exit cause for later phases; nil means a clean exit.
	doneCh   chan struct{}
	doneOnce sync.Once
	exitErr  error
}

func (c *procState) closeCapture() {
	c.closeCaptureOnce.Do(func() {
		if c.captureFile != nil {
			_ = c.captureFile.Close()
		}
	})
}

func (c *procState) markDone(err error) {
	c.doneOnce.Do(func() {
		c.exitErr = err
		close(c.doneCh)
	})
}

// startProcesses spawns every process declared in Spec.Processes with
// socketpair stdio, ordered by ProcessSpec.Turn. Processes are grouped by Turn
// ascending; group N is spawned only after every process in group N-1 has
// fork+exec'd. Processes sharing a Turn (including the default Turn 0) are
// spawned concurrently — no intra-group ordering is guaranteed. The gating is
// process-started only: there are no health or readiness semantics.
//
// It is the process-set counterpart to prepareTerminalCommand+startPty and is
// reached from StartTerminal when Spec.Processes is non-empty. On a spawn
// failure within a group the already-started processes keep running (they are
// torn down when the runner's context is canceled by Close); the error is
// returned so the caller can surface it.
func (sr *Exec) startProcesses() error {
	sr.metadataMu.RLock()
	specs := make([]api.ProcessSpec, len(sr.metadata.Spec.Processes))
	copy(specs, sr.metadata.Spec.Processes)
	sr.metadataMu.RUnlock()

	if len(specs) == 0 {
		return errors.New("startProcesses called with empty Spec.Processes")
	}

	for _, turn := range orderedTurns(specs) {
		group := processesWithTurn(specs, turn)
		if err := sr.spawnGroup(turn, group); err != nil {
			return err
		}
	}
	return nil
}

// orderedTurns returns the distinct Turn values present in specs, ascending.
func orderedTurns(specs []api.ProcessSpec) []int {
	seen := make(map[int]struct{}, len(specs))
	turns := make([]int, 0, len(specs))
	for _, s := range specs {
		if _, ok := seen[s.Turn]; ok {
			continue
		}
		seen[s.Turn] = struct{}{}
		turns = append(turns, s.Turn)
	}
	sort.Ints(turns)
	return turns
}

// processesWithTurn returns the processes whose Turn equals turn, preserving
// their declaration order in the spec.
func processesWithTurn(specs []api.ProcessSpec, turn int) []api.ProcessSpec {
	group := make([]api.ProcessSpec, 0, len(specs))
	for _, s := range specs {
		if s.Turn == turn {
			group = append(group, s)
		}
	}
	return group
}

// spawnGroup fork+execs every process in a single Turn group concurrently and
// blocks until all have started (or failed to start). The barrier is what
// enforces the turn-ordered gating in startProcesses: the next group is not
// touched until this one has fully fork+exec'd.
func (sr *Exec) spawnGroup(turn int, group []api.ProcessSpec) error {
	sr.logger.Debug("spawning process group", "turn", turn, "count", len(group))
	var wg sync.WaitGroup
	errs := make([]error, len(group))
	for i := range group {
		wg.Add(1)
		go func(idx int, spec api.ProcessSpec) {
			defer wg.Done()
			errs[idx] = sr.spawnProcess(spec)
		}(i, group[i])
	}
	wg.Wait()
	return errors.Join(errs...)
}

// spawnProcess allocates a socketpair, fork+execs the process with the child
// end wired to its stdin/stdout/stderr, and starts the per-process drain and
// exit-observation goroutines. The process's pid joins the PID-1 reaper's watch
// set in init mode so its exit is observed without racing os/exec.Wait.
func (sr *Exec) spawnProcess(spec api.ProcessSpec) error {
	sv, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|sockCloexec, 0)
	if err != nil {
		return fmt.Errorf("process %q: socketpair: %w", spec.Name, err)
	}
	parentEnd := os.NewFile(uintptr(sv[0]), "child-"+spec.Name+"-parent")
	childEnd := os.NewFile(uintptr(sv[1]), "child-"+spec.Name+"-child")

	// A process with no CaptureFile leaves captureFile nil; drainProcess then
	// discards its output so a full socket buffer never blocks the process.
	var captureFile *os.File
	if spec.CaptureFile != "" {
		captureFile, err = sr.openProcessCapture(spec)
		if err != nil {
			_ = parentEnd.Close()
			_ = childEnd.Close()
			return err
		}
	}

	//nolint:gosec // the operator declares the process command and its args
	cmd := exec.CommandContext(sr.ctx, spec.Command, spec.CommandArgs...)
	cmd.Stdin = childEnd
	cmd.Stdout = childEnd
	cmd.Stderr = childEnd
	// New session + pgroup led by the child so later phases can signal the
	// child's pgroup independently (Setsid implies pgid == pid).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if errStart := cmd.Start(); errStart != nil {
		_ = parentEnd.Close()
		_ = childEnd.Close()
		if captureFile != nil {
			_ = captureFile.Close()
		}
		return fmt.Errorf("process %q: start: %w", spec.Name, errStart)
	}
	// The child now owns its dup'd copy of childEnd (fds 0/1/2); the parent's
	// reference is no longer needed and would otherwise hold the socket open
	// past the child's exit, hiding EOF from the drain goroutine.
	_ = childEnd.Close()

	c := &procState{
		spec:        spec,
		cmd:         cmd,
		parentEnd:   parentEnd,
		captureFile: captureFile,
		doneCh:      make(chan struct{}),
	}

	// Register with the reaper before any other work so the exit-observation
	// goroutine reads from a channel that is already in the reaper's table.
	var reaperCh <-chan int
	if sr.initMode && sr.reaper != nil {
		reaperCh = sr.reaper.WatchChild(cmd.Process.Pid)
	}

	sr.processesMu.Lock()
	sr.processes = append(sr.processes, c)
	sr.processesMu.Unlock()

	sr.logger.Info("supervised process started",
		"process", spec.Name, "pid", cmd.Process.Pid, "turn", spec.Turn)

	go sr.drainProcess(c)
	go sr.watchProcessExit(c, reaperCh)
	return nil
}

// openProcessCapture opens (creating/appending) the process's capture file at
// the legacy 0o600 and then re-chmods it to the resolved ProcessSpec.CaptureMode
// (0o600 when unset), mirroring the single-child capture path. Callers must
// only invoke this when spec.CaptureFile is non-empty.
func (sr *Exec) openProcessCapture(spec api.ProcessSpec) (*os.File, error) {
	f, err := os.OpenFile(spec.CaptureFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("process %q: open capture file %q: %w", spec.Name, spec.CaptureFile, err)
	}
	if errPerm := sr.applyArtifactPerms("process-capture", spec.CaptureFile, spec.CaptureMode, nil); errPerm != nil {
		_ = f.Close()
		return nil, errPerm
	}
	return f, nil
}

// drainProcess continuously copies the process's socketpair output into its
// capture file until EOF (process closed its stdio) or the runner's context is
// canceled. The capture fd is closed when the drain ends so a process's
// transcript is flushed and the fd does not leak across New→Start→Close
// cycles.
func (sr *Exec) drainProcess(c *procState) {
	defer c.closeCapture()

	// Closing parentEnd on ctx cancel unblocks the Read below so the drain
	// goroutine exits on Close even if the process is still alive.
	go func() {
		<-sr.ctx.Done()
		_ = c.parentEnd.Close()
	}()

	buf := make([]byte, processDrainBufSize)
	for {
		n, errRead := c.parentEnd.Read(buf)
		if n > 0 && c.captureFile != nil {
			if _, errWrite := c.captureFile.Write(buf[:n]); errWrite != nil {
				sr.logger.Error("process capture write error", "process", c.spec.Name, "err", errWrite)
				return
			}
		}
		if errRead != nil {
			if !errors.Is(errRead, io.EOF) &&
				!errors.Is(errRead, os.ErrClosed) &&
				!errors.Is(errRead, net.ErrClosed) {
				sr.logger.Debug("process drain read ended", "process", c.spec.Name, "err", errRead)
			}
			return
		}
	}
}

// watchProcessExit observes the process's exit and records the cause on the
// process's doneCh. In PID-1 init mode the reaper has already consumed the
// SIGCHLD, so the reaper's per-child channel is authoritative and a cmd.Wait
// would race to ECHILD; outside init mode cmd.Wait carries the real status.
// Mirrors watchChildExit's dual-path logic for the single-child case.
func (sr *Exec) watchProcessExit(c *procState, reaperCh <-chan int) {
	var exitErr error
	if sr.initMode && sr.reaper != nil {
		if code, ok := <-reaperCh; ok && code != 0 {
			exitErr = fmt.Errorf("process %q exited: code=%d", c.spec.Name, code)
		}
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Release()
		}
	} else if werr := c.cmd.Wait(); werr != nil {
		exitErr = fmt.Errorf("process %q exited: %w", c.spec.Name, werr)
	}

	sr.logger.Info("supervised process exited", "process", c.spec.Name, "err", exitErr)
	c.markDone(exitErr)
}
