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
	"os/exec"
	"syscall"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
	"golang.org/x/sys/unix"
)

// exitStatus is the tracked child's wait status as the PID-1 reaper reports
// it on TrackedExitCh. Code keeps the shell-conventional 128+signal
// encoding for a signaled death so log lines and error text stay as they
// were; Signal is non-zero only for a signaled death, which is what tells
// a SIGKILL'd child apart from one that ran `exit 137`.
type exitStatus struct {
	Code   int
	Signal syscall.Signal
}

// ExitError is the terminating error for a workload exit. Serve (and the
// EvCmdExited event) carry it when the tracked child exited, in both
// PID-1 init mode and normal mode; teardown paths that never observed a
// wait status, and non-workload failures such as a PTY error, are plain
// errors. Match it with errors.As.
//
// Exactly one of Code and Signal is set: Code (possibly 0) for a normal
// exit, Signal (e.g. "SIGKILL") for a signaled death, where no POSIX exit
// status exists. Error() returns the same text as before this type existed
// ("shell process exited: code=N", "shell process exited: <wait error>",
// "shell process exited (code 0)"), and Unwrap exposes the underlying
// error, so errors.As(err, **exec.ExitError) keeps working in normal mode.
type ExitError struct {
	Code   *int
	Signal string

	err error
}

func (e *ExitError) Error() string {
	if e.err == nil {
		return "shell process exited"
	}
	return e.err.Error()
}

func (e *ExitError) Unwrap() error { return e.err }

// signalName returns the conventional name of sig ("SIGKILL"), falling
// back to Go's description when the platform table has no name for it.
func signalName(sig syscall.Signal) string {
	if name := unix.SignalName(sig); name != "" {
		return name
	}
	return sig.String()
}

// newTrackedExitError builds the ExitError for an exit the PID-1 reaper
// delivered.
func newTrackedExitError(st exitStatus) *ExitError {
	e := &ExitError{err: fmt.Errorf("shell process exited: code=%d", st.Code)}
	if st.Signal != 0 {
		e.Signal = signalName(st.Signal)
	} else {
		code := st.Code
		e.Code = &code
	}
	return e
}

// newWaitExitError maps cmd.Wait's result to the terminating error. A nil
// result or an *exec.ExitError becomes an *ExitError; any other Wait error
// (no wait status was observed) stays a plain wrapped error.
func newWaitExitError(werr error) error {
	if werr == nil {
		code := 0
		return &ExitError{Code: &code, err: errors.New("shell process exited (code 0)")}
	}
	wrapped := fmt.Errorf("shell process exited: %w", werr)
	var ee *exec.ExitError
	if !errors.As(werr, &ee) {
		return wrapped
	}
	e := &ExitError{err: wrapped}
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		e.Signal = signalName(ws.Signal())
	} else {
		code := ee.ExitCode()
		e.Code = &code
	}
	return e
}

// exitInfo is the workload's exit as watchChildExit observed it. It is
// held on the runner, not in Status, until Close writes State Exited, so a
// concurrent attacher write cannot persist an exitCode next to State
// Ready.
type exitInfo struct {
	code   *int
	signal string
	at     time.Time
}

// recordExit stores e's details for Close to persist. Guarded by
// metadataMu.
func (sr *Exec) recordExit(e *ExitError) {
	info := &exitInfo{signal: e.Signal, at: time.Now().UTC()}
	if e.Code != nil {
		code := *e.Code
		info.code = &code
	}
	sr.metadataMu.Lock()
	sr.exitInfo = info
	sr.metadataMu.Unlock()
}

// applyExitInfoLocked copies the recorded exit details into Status unless
// they are already there, reporting whether Status changed. The caller
// holds metadataMu for writing.
func (sr *Exec) applyExitInfoLocked() bool {
	if sr.exitInfo == nil || !sr.metadata.Status.ExitedAt.IsZero() {
		return false
	}
	if sr.exitInfo.code != nil {
		code := *sr.exitInfo.code
		sr.metadata.Status.ExitCode = &code
	}
	sr.metadata.Status.ExitSignal = sr.exitInfo.signal
	sr.metadata.Status.ExitedAt = sr.exitInfo.at
	return true
}

// markExited commits State Exited, together with any exit details already
// recorded (the natural-exit case), and persists it. Unlike
// updateTerminalState it commits in memory even when the write fails:
// nothing advances past Exited, so the #373 strand risk does not apply,
// and Metadata() and the status observer must still see the terminal as
// exited when the closing uid cannot write the directory (#345).
func (sr *Exec) markExited() error {
	sr.metadataMu.Lock()
	sr.metadata.Status.State = api.Exited
	sr.applyExitInfoLocked()
	doc := sr.metadata
	sr.metadataMu.Unlock()

	err := sr.persistMetadata(doc)
	if err != nil {
		sr.noteMetadataWriteErr("markExited", err)
	}
	sr.notifyStatus()
	return err
}

// persistLateExitInfo writes exit details that arrived after markExited —
// the Stop/ctx paths, where Exited is written before shutdownChild signals
// the child. No-op when there is nothing new.
func (sr *Exec) persistLateExitInfo() {
	sr.metadataMu.Lock()
	changed := sr.metadata.Status.State == api.Exited && sr.applyExitInfoLocked()
	doc := sr.metadata
	sr.metadataMu.Unlock()
	if !changed {
		return
	}
	if err := sr.persistMetadata(doc); err != nil {
		sr.noteMetadataWriteErr("persistLateExitInfo", err)
	}
	sr.notifyStatus()
}
