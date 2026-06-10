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
// descriptor out of the runtime poller and flips it to blocking mode. Keeping
// the descriptor pollable until the final SetNonblock in restoreParentTerminal
// lets the persistent stdin pump read it efficiently without mode flapping.
// Control hands us the fd without disturbing the poller registration. See
// issues #364 and #399.
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

// escRestoreParentTerm normalizes the parent terminal's output-side DEC private
// mode state on session teardown: leave the alternate-screen buffer, show the
// cursor, reset the cursor style to the terminal default, disable mouse
// tracking under every common encoding, and disable bracketed paste. Symmetric
// with the modes the server-side attach repaint may set (escEnterAltScreen /
// escHideCursor in internal/terminal/terminalrunner/terminal_screen.go) and
// with anything a TUI/shell on the remote side wrote through to the parent
// terminal without a matching reset. Emitted unconditionally per #365 — the
// client cannot rely on the remote side emitting a matching reset on its own;
// resetting a mode that wasn't set is a no-op on every terminal.
const escRestoreParentTerm = "\x1b[?1049l" + // leave the alternate screen buffer
	"\x1b[?25h" + // make the cursor visible
	"\x1b[0 q" + // reset cursor style (DECSCUSR) to terminal default
	"\x1b[?1000l" + // disable X11 mouse reporting (basic)
	"\x1b[?1002l" + // disable cell-motion mouse tracking
	"\x1b[?1003l" + // disable all-motion mouse tracking
	"\x1b[?1006l" + // disable SGR mouse encoding
	"\x1b[?1015l" + // disable urxvt mouse encoding
	"\x1b[?2004l" // disable bracketed paste

// restoreParentTerminal performs a single, idempotent "unblock stdin + restore
// tty" at the session boundary so the parent shell is always handed back a
// usable terminal — on user detach, remote process exit, peer hangup, and
// ctx cancellation alike. It is safe to call from every teardown path; the
// sync.Once guarantees the body runs exactly once. See issue #364.
//
// postDrainMsg, when non-empty, is written to stdout after the inbound
// socket->stdout copier has drained but before the termios raw->cooked restore
// — the safe point at which a teardown-confirmation banner (e.g. the detach
// "Detached" notice) cannot interleave with mid-flush remote bytes yet raw mode
// is still active so its embedded \r\n behaves correctly. The Close()/exit
// paths pass "". See issues #364 and #383.
func (sr *Exec) restoreParentTerminal(postDrainMsg string) {
	sr.restoreOnce.Do(func() {
		sr.logger.Debug("restoreParentTerminal: beginning terminal restore")

		// Cancel the context so both copiers observe shutdown: the socket-side
		// copier is unblocked by CopierManager (it sets read/write deadlines on
		// the UnixConn on ctx.Done), and the stdin->socket copier reads through
		// the persistent pump's context-bound pumpReader, whose Read returns
		// promptly on cancellation regardless of the descriptor's pollability.
		// The pump goroutine itself keeps owning sr.stdin so a byte it already
		// read is handed to the next session, not lost. See issues #364 and #399.
		sr.ctxCancel()

		// Wait for the copier goroutines to drain so nobody is mid-read when we
		// hand the descriptor back to blocking mode — but bound it. Both copiers
		// now exit on ctx-cancel (the stdin->socket copier via pumpReader, the
		// socket->stdout copier via the UnixConn deadline), so this returns
		// promptly on every path. The bound remains a safety net against a
		// wedged socket read so teardown can never hang (#364 e2e timeouts).
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

		// Write any teardown-confirmation banner now: the inbound copier has
		// drained (so this cannot interleave with mid-flush remote bytes) and the
		// termios restore below has not yet run (so raw mode is still active and
		// the banner's embedded \r\n behaves). Emitted before the normalize
		// sequence so it lands while any alt-screen the remote set is still
		// active, matching the pre-#383 relative ordering. See issue #383.
		if postDrainMsg != "" && sr.stdout != nil {
			if _, err := sr.stdout.WriteString(postDrainMsg); err != nil {
				sr.logger.Debug("restoreParentTerminal: write post-drain message failed", "error", err)
			}
		}

		// Normalize the parent terminal's output-side DEC private mode state
		// (alt-screen, cursor visibility, cursor style) before handing the
		// terminal back. Runs after the copier wait so we do not race the
		// socket->stdout copier still draining remote bytes, and before the
		// termios restore — these are escape bytes interpreted by the parent
		// terminal emulator, not line-discipline input, so the active termios
		// mode does not affect them. See issue #365.
		if sr.stdout != nil {
			if _, err := sr.stdout.WriteString(escRestoreParentTerm); err != nil {
				sr.logger.Debug("restoreParentTerminal: write normalize sequence failed", "error", err)
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
// descriptor pollable across MakeRaw/Restore/getsize avoids prematurely
// dropping the persistent stdin pump's poller registration. See issue #364.
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
