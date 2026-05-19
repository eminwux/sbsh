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

// childDrainBufSize is the read buffer used by each child's drain goroutine
// when copying the child's socketpair output into its capture file. Matches
// the PTY reader's buffer size in terminal.go.
const childDrainBufSize = 8192

// childProc holds the runtime state for one supervised child spawned from a
// non-empty TerminalSpec.Children. Each child runs with socketpair stdio (not
// a PTY); the parent end is drained into the child's capture file by a
// per-child goroutine. Phase 2 owns spawn + drain + exit observation;
// multiplexing the child onto the operator's attach session is phase 3.
type childProc struct {
	spec      api.ChildSpec
	cmd       *exec.Cmd
	parentEnd *os.File // parent side of the socketpair; child side is dup'd to the child's 0/1/2

	// captureFile backs the drain goroutine's write target. nil when the
	// child declared no CaptureFile — output is then drained to discard so a
	// full socket buffer never blocks the child.
	captureFile      *os.File
	closeCaptureOnce sync.Once

	// doneCh is closed once the child's exit has been observed (via the reaper
	// in PID-1 init mode, or os/exec.Wait otherwise). exitErr carries the
	// observed exit cause for later phases; nil means a clean exit.
	doneCh   chan struct{}
	doneOnce sync.Once
	exitErr  error
}

func (c *childProc) closeCapture() {
	c.closeCaptureOnce.Do(func() {
		if c.captureFile != nil {
			_ = c.captureFile.Close()
		}
	})
}

func (c *childProc) markDone(err error) {
	c.doneOnce.Do(func() {
		c.exitErr = err
		close(c.doneCh)
	})
}

// startChildren spawns every child declared in Spec.Children with socketpair
// stdio, ordered by ChildSpec.Turn. Children are grouped by Turn ascending;
// group N is spawned only after every child in group N-1 has fork+exec'd.
// Children sharing a Turn (including the default Turn 0) are spawned
// concurrently — no intra-group ordering is guaranteed. The gating is
// process-started only: there are no health or readiness semantics.
//
// It is the child-set counterpart to prepareTerminalCommand+startPty and is
// reached from StartTerminal when Spec.Children is non-empty. On a spawn
// failure within a group the already-started children keep running (they are
// torn down when the runner's context is canceled by Close); the error is
// returned so the caller can surface it.
func (sr *Exec) startChildren() error {
	sr.metadataMu.RLock()
	specs := make([]api.ChildSpec, len(sr.metadata.Spec.Children))
	copy(specs, sr.metadata.Spec.Children)
	sr.metadataMu.RUnlock()

	if len(specs) == 0 {
		return errors.New("startChildren called with empty Spec.Children")
	}

	for _, turn := range orderedTurns(specs) {
		group := childrenWithTurn(specs, turn)
		if err := sr.spawnGroup(turn, group); err != nil {
			return err
		}
	}
	return nil
}

// orderedTurns returns the distinct Turn values present in specs, ascending.
func orderedTurns(specs []api.ChildSpec) []int {
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

// childrenWithTurn returns the children whose Turn equals turn, preserving
// their declaration order in the spec.
func childrenWithTurn(specs []api.ChildSpec, turn int) []api.ChildSpec {
	group := make([]api.ChildSpec, 0, len(specs))
	for _, s := range specs {
		if s.Turn == turn {
			group = append(group, s)
		}
	}
	return group
}

// spawnGroup fork+execs every child in a single Turn group concurrently and
// blocks until all have started (or failed to start). The barrier is what
// enforces the turn-ordered gating in startChildren: the next group is not
// touched until this one has fully fork+exec'd.
func (sr *Exec) spawnGroup(turn int, group []api.ChildSpec) error {
	sr.logger.Debug("spawning child group", "turn", turn, "count", len(group))
	var wg sync.WaitGroup
	errs := make([]error, len(group))
	for i := range group {
		wg.Add(1)
		go func(idx int, spec api.ChildSpec) {
			defer wg.Done()
			errs[idx] = sr.spawnChild(spec)
		}(i, group[i])
	}
	wg.Wait()
	return errors.Join(errs...)
}

// spawnChild allocates a socketpair, fork+execs the child with the child end
// wired to its stdin/stdout/stderr, and starts the per-child drain and
// exit-observation goroutines. The child's pid joins the PID-1 reaper's watch
// set in init mode so its exit is observed without racing os/exec.Wait.
func (sr *Exec) spawnChild(spec api.ChildSpec) error {
	sv, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|sockCloexec, 0)
	if err != nil {
		return fmt.Errorf("child %q: socketpair: %w", spec.Name, err)
	}
	parentEnd := os.NewFile(uintptr(sv[0]), "child-"+spec.Name+"-parent")
	childEnd := os.NewFile(uintptr(sv[1]), "child-"+spec.Name+"-child")

	// A child with no CaptureFile leaves captureFile nil; drainChild then
	// discards its output so a full socket buffer never blocks the child.
	var captureFile *os.File
	if spec.CaptureFile != "" {
		captureFile, err = sr.openChildCapture(spec)
		if err != nil {
			_ = parentEnd.Close()
			_ = childEnd.Close()
			return err
		}
	}

	//nolint:gosec // the operator declares the child command and its args
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
		return fmt.Errorf("child %q: start: %w", spec.Name, errStart)
	}
	// The child now owns its dup'd copy of childEnd (fds 0/1/2); the parent's
	// reference is no longer needed and would otherwise hold the socket open
	// past the child's exit, hiding EOF from the drain goroutine.
	_ = childEnd.Close()

	c := &childProc{
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

	sr.childrenMu.Lock()
	sr.children = append(sr.children, c)
	sr.childrenMu.Unlock()

	sr.logger.Info("supervised child started",
		"child", spec.Name, "pid", cmd.Process.Pid, "turn", spec.Turn)

	go sr.drainChild(c)
	go sr.watchChildProcExit(c, reaperCh)
	return nil
}

// openChildCapture opens (creating/appending) the child's capture file at the
// legacy 0o600 and then re-chmods it to the resolved ChildSpec.CaptureMode
// (0o600 when unset), mirroring the single-child capture path. Callers must
// only invoke this when spec.CaptureFile is non-empty.
func (sr *Exec) openChildCapture(spec api.ChildSpec) (*os.File, error) {
	f, err := os.OpenFile(spec.CaptureFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("child %q: open capture file %q: %w", spec.Name, spec.CaptureFile, err)
	}
	if errPerm := sr.applyArtifactPerms("child-capture", spec.CaptureFile, spec.CaptureMode, nil); errPerm != nil {
		_ = f.Close()
		return nil, errPerm
	}
	return f, nil
}

// drainChild continuously copies the child's socketpair output into its
// capture file until EOF (child closed its stdio) or the runner's context is
// canceled. The capture fd is closed when the drain ends so a child's
// transcript is flushed and the fd does not leak across New→Start→Close
// cycles.
func (sr *Exec) drainChild(c *childProc) {
	defer c.closeCapture()

	// Closing parentEnd on ctx cancel unblocks the Read below so the drain
	// goroutine exits on Close even if the child is still alive.
	go func() {
		<-sr.ctx.Done()
		_ = c.parentEnd.Close()
	}()

	buf := make([]byte, childDrainBufSize)
	for {
		n, errRead := c.parentEnd.Read(buf)
		if n > 0 && c.captureFile != nil {
			if _, errWrite := c.captureFile.Write(buf[:n]); errWrite != nil {
				sr.logger.Error("child capture write error", "child", c.spec.Name, "err", errWrite)
				return
			}
		}
		if errRead != nil {
			if !errors.Is(errRead, io.EOF) &&
				!errors.Is(errRead, os.ErrClosed) &&
				!errors.Is(errRead, net.ErrClosed) {
				sr.logger.Debug("child drain read ended", "child", c.spec.Name, "err", errRead)
			}
			return
		}
	}
}

// watchChildProcExit observes the child's exit and records the cause on the
// child's doneCh. In PID-1 init mode the reaper has already consumed the
// SIGCHLD, so the reaper's per-child channel is authoritative and a cmd.Wait
// would race to ECHILD; outside init mode cmd.Wait carries the real status.
// Mirrors watchChildExit's dual-path logic for the single-child case.
func (sr *Exec) watchChildProcExit(c *childProc, reaperCh <-chan int) {
	var exitErr error
	if sr.initMode && sr.reaper != nil {
		if code, ok := <-reaperCh; ok && code != 0 {
			exitErr = fmt.Errorf("child %q exited: code=%d", c.spec.Name, code)
		}
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Release()
		}
	} else if werr := c.cmd.Wait(); werr != nil {
		exitErr = fmt.Errorf("child %q exited: %w", c.spec.Name, werr)
	}

	sr.logger.Info("supervised child exited", "child", c.spec.Name, "err", exitErr)
	c.markDone(exitErr)
}
