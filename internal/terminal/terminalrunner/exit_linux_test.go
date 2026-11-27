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

	"github.com/eminwux/sbsh/pkg/api"
)

// newInitModeExitExec starts cmd under a running PID-1-style reaper and
// returns an init-mode Exec watching it, as startPty would set it up.
func newInitModeExitExec(t *testing.T, script string) *Exec {
	t.Helper()
	rp := newReaper(discardLogger())
	rp.Start()
	t.Cleanup(rp.Stop)

	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	rp.RegisterChild(cmd.Process.Pid)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Exec{
		ctx:           ctx,
		ctxCancel:     cancel,
		logger:        discardLogger(),
		id:            "test-init",
		cmd:           cmd,
		closeReqCh:    make(chan error, 1),
		childDoneCh:   make(chan struct{}),
		childDoneOnce: &sync.Once{},
		initMode:      true,
		reaper:        rp,
	}
}

func TestWatchChildExit_Init_ExitCode3(t *testing.T) {
	sr := newInitModeExitExec(t, "exit 3")
	err := runWatchChildExit(t, sr)
	ee := requireExitError(t, err)
	if ee.Code == nil || *ee.Code != 3 || ee.Signal != "" {
		t.Fatalf("ExitError = {Code:%v Signal:%q}, want code 3", ee.Code, ee.Signal)
	}
	if got := err.Error(); got != "shell process exited: code=3" {
		t.Fatalf("Error() = %q, want unchanged text", got)
	}
	doc, _ := exitDocAfterClose(t, sr)
	if doc.Status.State != api.Exited || doc.Status.ExitCode == nil || *doc.Status.ExitCode != 3 ||
		doc.Status.ExitedAt.IsZero() {
		t.Fatalf("doc status = %+v, want Exited with exitCode 3 and exitedAt", doc.Status)
	}
}

// TestWatchChildExit_Init_SIGKILLDistinctFromExit137 asserts the widened
// reaper channel keeps the signal: SIGKILL reports exitSignal, while a
// child that ran `exit 137` (the same 128+9 code) reports exitCode 137.
func TestWatchChildExit_Init_SIGKILLDistinctFromExit137(t *testing.T) {
	sr := newInitModeExitExec(t, "kill -9 $$")
	err := runWatchChildExit(t, sr)
	ee := requireExitError(t, err)
	if ee.Code != nil || ee.Signal != "SIGKILL" {
		t.Fatalf("ExitError = {Code:%v Signal:%q}, want nil code and SIGKILL", ee.Code, ee.Signal)
	}
	if got := err.Error(); got != "shell process exited: code=137" {
		t.Fatalf("Error() = %q, want unchanged text", got)
	}
	doc, _ := exitDocAfterClose(t, sr)
	if doc.Status.ExitSignal != "SIGKILL" || doc.Status.ExitCode != nil || doc.Status.ExitedAt.IsZero() {
		t.Fatalf("doc status = %+v, want exitSignal SIGKILL and no exitCode", doc.Status)
	}

	sr137 := newInitModeExitExec(t, "exit 137")
	ee137 := requireExitError(t, runWatchChildExit(t, sr137))
	if ee137.Code == nil || *ee137.Code != 137 || ee137.Signal != "" {
		t.Fatalf("exit 137 ExitError = {Code:%v Signal:%q}, want code 137 and no signal", ee137.Code, ee137.Signal)
	}
}
