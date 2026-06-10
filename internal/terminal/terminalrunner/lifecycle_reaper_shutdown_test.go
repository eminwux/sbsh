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
	"context"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestShutdownChild_InitMode_ReapsAfterSIGKILL reconstructs the #398 race: in
// PID-1 init mode, Close's SIGKILL escalation used to return before the child
// was a zombie, so reaper.Stop's final drain ran before the kernel turned the
// child into a zombie — leaving a permanent zombie and parking watchChildExit
// forever on TrackedExitCh.
//
// The fix makes shutdownChild block (bounded) on childDoneCh after SIGKILL so
// the still-running reaper harvests the zombie before Close tears it down. This
// test drives the exact Close-order sequence (shutdownChild -> ctxCancel ->
// reaper.Stop) against a child that ignores SIGTERM and asserts both ACs: the
// child is reaped (no zombie) and watchChildExit is not parked.
func TestShutdownChild_InitMode_ReapsAfterSIGKILL(t *testing.T) {
	rp := newReaper(discardLogger())
	rp.Start()

	// Child ignores SIGTERM; only the uncatchable SIGKILL takes it down.
	// Setsid makes pgid == pid so the process-group kill in shutdownChild
	// targets exactly this child.
	cmd := exec.Command("/bin/sh", "-c", `trap '' TERM; while :; do sleep 1; done`)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := cmd.Process.Pid
	rp.RegisterChild(pid)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sr := &Exec{
		ctx:           ctx,
		ctxCancel:     cancel,
		logger:        discardLogger(),
		id:            "test",
		cmd:           cmd,
		closeReqCh:    make(chan error, 1),
		childDoneCh:   make(chan struct{}),
		childDoneOnce: &sync.Once{},
		initMode:      true,
		reaper:        rp,
	}
	sr.metadata.Spec.ShutdownGrace = 100 * time.Millisecond

	// watchChildExit is the production consumer of TrackedExitCh; on delivery
	// it calls markChildDone, which closes childDoneCh and unblocks
	// shutdownChild's post-SIGKILL wait.
	go sr.watchChildExit()

	// Let the shell install `trap '' TERM` before we start signaling, so
	// SIGTERM is genuinely ignored and the escalation path under test fires.
	time.Sleep(150 * time.Millisecond)

	// Drive the Close-order sequence the bug is about.
	sr.shutdownChild(sr.metadata.Spec.ShutdownGrace)
	sr.ctxCancel()
	rp.Stop()

	// AC1: the tracked child must be reaped — no zombie left behind. A zombie
	// is still addressable (Kill(pid, 0) returns nil) until it is reaped;
	// after reaping the pid is gone and Kill returns ESRCH.
	if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
		t.Fatalf("tracked child pid %d not reaped after Close sequence (Kill(0)=%v); want ESRCH (zombie leaked)",
			pid, err)
	}

	// AC2: watchChildExit must not be parked — childDoneCh closed means it
	// received the tracked exit (or escaped on ctx.Done) and ran markChildDone.
	select {
	case <-sr.childDoneCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("watchChildExit still parked: childDoneCh not closed after Close sequence")
	}
}
