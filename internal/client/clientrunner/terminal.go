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

package clientrunner

import (
	"log/slog"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// toBashUIMode: set terminal to RAW, update flags.
func (sr *Exec) toBashUIMode() error {
	sr.logger.Debug("toBashUIMode: switching to raw mode")
	lastTermState, err := toRawMode(sr.logger, sr.stdin)
	if err != nil {
		sr.logger.Error("toBashUIMode: failed to set raw mode", "error", err)
		return err
	}

	sr.uiMode = UIBash
	sr.lastTermState = lastTermState
	sr.logger.Info("toBashUIMode: switched to bash UI mode")
	return nil
}

// toExitShell: restore the terminal to its pre-attach (cooked) state.
//
// The restore ioctl is issued through SyscallConn().Control rather than
// term.Restore(int(stdin.Fd()), …) on purpose: os.File.Fd() pulls the
// descriptor out of the runtime poller and flips it to blocking mode, which
// would defeat the deadline-based unblock of the stdin copier in
// restoreParentTerminal. Control hands us the fd without disturbing the
// poller registration. See issue #364.
func (sr *Exec) toExitShell() error {
	sr.logger.Debug("toExitShell: switching to cooked mode")
	if sr.lastTermState != nil && sr.stdin != nil {
		if err := withRawFd(sr.stdin, func(fd int) error {
			return term.Restore(fd, sr.lastTermState)
		}); err != nil {
			sr.logger.Error("toExitShell: failed to restore terminal state", "error", err)
			return err
		}
	}

	sr.uiMode = UIExitShell
	sr.logger.Info("toExitShell: switched to exit shell UI mode")
	return nil
}

// copierWaitTimeout bounds how long restoreParentTerminal waits for the copier
// goroutines to drain. The socket->stdout copier unblocks near-instantly via
// the ctx-cancel deadline on the UnixConn; the ceiling exists only so an
// uninterruptible stdin read (see restoreParentTerminal) cannot wedge teardown.
const copierWaitTimeout = 500 * time.Millisecond

// restoreParentTerminal performs a single, idempotent "unblock stdin + restore
// tty" at the session boundary so the parent shell is always handed back a
// usable terminal — on user detach, remote process exit, peer hangup, and
// ctx cancellation alike. It is safe to call from every teardown path; the
// sync.Once guarantees the body runs exactly once. See issue #364.
func (sr *Exec) restoreParentTerminal() {
	sr.restoreOnce.Do(func() {
		sr.logger.Debug("restoreParentTerminal: beginning terminal restore")

		// Cancel the context so the socket-side copier is unblocked by
		// CopierManager (it sets read/write deadlines on the UnixConn on
		// ctx.Done) and the copier loops observe shutdown.
		sr.ctxCancel()

		// Best-effort unblock of the stdin->socket copier parked in
		// sr.stdin.Read(). This works only when the descriptor is pollable — a
		// freshly-opened *os.File over the tty. The real attach path reads
		// os.Stdin, which the Go runtime constructs non-pollable (it is not
		// registered with the runtime poller regardless of how the fd is later
		// accessed), so SetReadDeadline is a no-op there and the parked read
		// cannot be interrupted. The bounded wait below is what keeps that case
		// from wedging teardown.
		if sr.stdin != nil {
			if err := sr.stdin.SetReadDeadline(time.Now()); err != nil {
				sr.logger.Debug("restoreParentTerminal: SetReadDeadline failed", "error", err)
			}
		}

		// Wait for the copier goroutines to drain so nobody is mid-read when we
		// hand the descriptor back to blocking mode — but bound it. The
		// socket->stdout copier exits promptly on ctx-cancel; the stdin->socket
		// copier may be parked in an uninterruptible os.Stdin.Read() (see above)
		// that only process exit reaps. Blocking unconditionally on it hangs
		// every teardown path (#364 e2e detach/exit timeouts). The leaked reader
		// is harmless: the process exits immediately after restore.
		if sr.copier != nil {
			done := make(chan struct{})
			go func() {
				_ = sr.copier.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(copierWaitTimeout):
				sr.logger.Debug("restoreParentTerminal: copier wait timed out; proceeding with restore")
			}
		}

		// Restore termios raw->cooked (still via SyscallConn, no .Fd()).
		_ = sr.toExitShell()

		// Return the descriptor to blocking mode, clearing the O_NONBLOCK leak
		// that otherwise breaks the parent shell's read loop (golang/go#29137).
		// .Fd() additionally removes it from the poller, which is the correct
		// final state for a descriptor shared with the parent shell.
		if sr.stdin != nil {
			if err := syscall.SetNonblock(int(sr.stdin.Fd()), false); err != nil {
				sr.logger.Debug("restoreParentTerminal: SetNonblock failed", "error", err)
			}
		}

		sr.logger.Info("restoreParentTerminal: parent terminal restored")
	})
}

func (sr *Exec) initTerminal() error {
	sr.logger.Debug("initTerminal: client writing to terminal")

	sr.logger.Info("initTerminal: terminal initialized")
	return nil
}

func (sr *Exec) writeTerminal(input string) error {
	sr.logger.Debug("writeTerminal: writing to terminal", "input_len", len(input))
	const maxRetries = 10
	const retrySleepMs = 100

	for i := range len(input) {
		var err error
		for attempt := range maxRetries {
			_, err = sr.ioConn.Write([]byte{input[i]})
			if err == nil {
				break
			}
			sr.logger.Warn("writeTerminal: write failed, retrying", "index", i, "attempt", attempt+1, "error", err)
			time.Sleep(retrySleepMs * time.Millisecond)
		}
		if err != nil {
			sr.logger.Error("writeTerminal: failed to write byte after retries", "index", i, "error", err)
			return err
		}
		time.Sleep(time.Microsecond)
	}
	sr.logger.Info("writeTerminal: finished writing to terminal")
	return nil
}

func toRawMode(logger *slog.Logger, stdin *os.File) (*term.State, error) {
	var state *term.State
	err := withRawFd(stdin, func(fd int) error {
		var merr error
		state, merr = term.MakeRaw(fd)
		return merr
	})
	if err != nil {
		logger.Error("toRawMode: failed to set raw mode", "error", err)
		return nil, err
	}
	logger.Info("toRawMode: terminal set to raw mode")
	return state, nil
}

// withRawFd invokes fn with stdin's underlying descriptor obtained through
// SyscallConn().Control, which — unlike os.File.Fd() — leaves the descriptor
// registered with the runtime poller and in non-blocking mode. Keeping the
// descriptor pollable is what lets restoreParentTerminal interrupt the parked
// stdin read via SetReadDeadline. See issue #364.
func withRawFd(f *os.File, fn func(fd int) error) error {
	rc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var inner error
	if cerr := rc.Control(func(fd uintptr) { inner = fn(int(fd)) }); cerr != nil {
		return cerr
	}
	return inner
}

// ttyGetsize reports the terminal window size for f without calling
// os.File.Fd() (which would drop the descriptor from the runtime poller); see
// withRawFd. It mirrors pty.Getsize's (rows, cols) result shape.
func ttyGetsize(f *os.File) (int, int, error) {
	var rows, cols int
	gerr := withRawFd(f, func(fd int) error {
		ws, ierr := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
		if ierr != nil {
			return ierr
		}
		rows, cols = int(ws.Row), int(ws.Col)
		return nil
	})
	return rows, cols, gerr
}
