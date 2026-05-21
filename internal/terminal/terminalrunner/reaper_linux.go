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
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// reaper drains SIGCHLD for the whole PID namespace by looping
// Wait4(-1, WNOHANG) on each signal. It is only active when sbsh is acting as
// container init (see internal/initmode). The tracked child's exit status is
// routed to a registered callback so the normal "child exited" path in
// startPty still fires exactly once even though the kernel-level SIGCHLD was
// consumed here instead of by os/exec.Cmd.Wait.
type reaper struct {
	logger *slog.Logger

	sigCh chan os.Signal
	stop  chan struct{}
	done  chan struct{}

	mu             sync.Mutex
	trackedPid     int
	trackedOnce    sync.Once
	trackedExitCh  chan int // buffered 1; receives exit code for trackedPid
	trackedStopped bool
	// pendingExits records exit codes of children reaped before RegisterChild
	// set trackedPid. startPty (and the tests) call RegisterChild *after*
	// cmd.Start, so a fast-exiting child can be reaped here before its pid is
	// known. Recording the exit lets a late RegisterChild still resolve the
	// tracked exit instead of silently treating the child as an orphan and
	// timing out on TrackedExitCh. Only populated while trackedPid == 0, so it
	// is bounded to the (microsecond) pre-registration window.
	pendingExits map[int]int
}

// newReaper constructs a reaper but does not start it. Call Start to install
// the signal handler and kick off the loop.
func newReaper(logger *slog.Logger) *reaper {
	return &reaper{
		logger:        logger,
		sigCh:         make(chan os.Signal, 16),
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
		trackedExitCh: make(chan int, 1),
	}
}

// Start installs the SIGCHLD handler and begins the drain loop. Safe to call
// exactly once.
func (r *reaper) Start() {
	signal.Notify(r.sigCh, syscall.SIGCHLD)
	// Prime the pump: drain any SIGCHLDs that fired before Notify was
	// installed. Harmless if there are none.
	go r.loop()
}

// Stop tears down the reaper and waits for the loop to exit. Unreaped
// descendants (there should be none after a clean shutdown) are not waited on.
func (r *reaper) Stop() {
	select {
	case <-r.stop:
		// already stopped
		return
	default:
	}
	close(r.stop)
	signal.Stop(r.sigCh)
	<-r.done
}

// RegisterChild tells the reaper which PID corresponds to the tracked
// terminal child. Must be called at most once per reaper instance; subsequent
// calls are ignored to make the Start / RegisterChild / Wait ordering robust
// against races.
func (r *reaper) RegisterChild(pid int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.trackedPid != 0 {
		return
	}
	r.trackedPid = pid
	// If the child already exited before we learned its pid, the reaper
	// recorded its exit code in pendingExits; resolve the tracked exit now.
	if code, ok := r.pendingExits[pid]; ok && !r.trackedStopped {
		r.deliverTrackedExitLocked(code)
		r.logger.Info("reaper: tracked child exited before registration", "pid", pid, "exit", code)
	}
	// Past registration, an unmatched reaped pid is a genuine orphan, so the
	// recording window is closed; drop the map.
	r.pendingExits = nil
}

// TrackedExitCh returns a channel that receives the exit code of the
// registered tracked child, exactly once, when the reaper reaps it. The
// channel is buffered so the reaper never blocks delivering.
func (r *reaper) TrackedExitCh() <-chan int {
	return r.trackedExitCh
}

// drainOnce runs a single WNOHANG Wait4 sweep, returning the number of
// children reaped. Exported for tests.
func (r *reaper) drainOnce() int {
	reaped := 0
	for {
		var status syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		switch {
		case pid > 0:
			reaped++
			r.handleReaped(pid, status)
			continue
		case pid == 0:
			// No more children ready to be reaped right now.
			return reaped
		case errors.Is(err, syscall.ECHILD):
			// No children at all (normal once the tracked child is gone).
			return reaped
		case errors.Is(err, syscall.EINTR):
			continue
		default:
			r.logger.Warn("reaper: wait4 error", "err", err)
			return reaped
		}
	}
}

func (r *reaper) handleReaped(pid int, status syscall.WaitStatus) {
	exitCode := status.ExitStatus()
	if status.Signaled() {
		// 128 + signal number is the shell-conventional encoding.
		//nolint:mnd // shell-conventional encoding of signal death
		exitCode = 128 + int(status.Signal())
	}

	// The match/record decision is taken under a single lock so it cannot race
	// RegisterChild: either we observe trackedPid and deliver, or we observe
	// the pre-registration window and record for RegisterChild to resolve.
	r.mu.Lock()
	defer r.mu.Unlock()

	switch {
	case r.trackedPid != 0 && pid == r.trackedPid && !r.trackedStopped:
		r.deliverTrackedExitLocked(exitCode)
		r.logger.Info("reaper: tracked child exited", "pid", pid, "exit", exitCode)
	case r.trackedPid == 0:
		// RegisterChild has not run yet, so this could be the tracked child
		// reaped before its pid was registered. Record the exit; a late
		// RegisterChild will resolve it.
		if r.pendingExits == nil {
			r.pendingExits = make(map[int]int)
		}
		r.pendingExits[pid] = exitCode
		r.logger.Debug("reaper: recorded pre-registration exit", "pid", pid, "exit", exitCode)
	default:
		r.logger.Debug("reaper: reaped orphan", "pid", pid, "status", status)
	}
}

// deliverTrackedExitLocked sends the tracked child's exit code on
// trackedExitCh exactly once and closes the channel. The caller must hold
// r.mu; the channel is buffered(1) and trackedOnce guards the single send, so
// the send never blocks and holding the lock across it cannot deadlock.
func (r *reaper) deliverTrackedExitLocked(exitCode int) {
	r.trackedOnce.Do(func() {
		r.trackedStopped = true
		r.trackedExitCh <- exitCode
		close(r.trackedExitCh)
	})
}

func (r *reaper) loop() {
	defer close(r.done)
	// Initial drain catches pre-Notify SIGCHLDs.
	r.drainOnce()
	for {
		select {
		case <-r.stop:
			// Final drain before exit.
			r.drainOnce()
			return
		case <-r.sigCh:
			r.drainOnce()
		}
	}
}
