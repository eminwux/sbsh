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
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/eminwux/sbsh/pkg/api"
	"github.com/hinshun/vt10x"
	"golang.org/x/sys/unix"
)

// errMockDetach is the sentinel a Detach-RPC-failure mock returns so a test can
// assert Exec.Detach propagates it while still restoring the parent terminal.
var errMockDetach = errors.New("detach rpc failed")

// ttyState reports the descriptor's blocking flag and the ECHO/ICANON termios
// line-discipline flags. It reads them through SyscallConn().Control so the
// inspection itself never disturbs the descriptor's poller registration.
func ttyState(t *testing.T, f *os.File) (bool, bool, bool) {
	t.Helper()
	var nonblock, echo, icanon bool
	rc, err := f.SyscallConn()
	if err != nil {
		t.Fatalf("SyscallConn: %v", err)
	}
	if cerr := rc.Control(func(fd uintptr) {
		tm, e := unix.IoctlGetTermios(int(fd), unix.TCGETS)
		if e != nil {
			t.Errorf("TCGETS: %v", e)
			return
		}
		echo = tm.Lflag&unix.ECHO != 0
		icanon = tm.Lflag&unix.ICANON != 0
		fl, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFL, 0)
		if errno != 0 {
			t.Errorf("F_GETFL: %v", errno)
			return
		}
		nonblock = int(fl)&syscall.O_NONBLOCK != 0
	}); cerr != nil {
		t.Fatalf("Control: %v", cerr)
	}
	return nonblock, echo, icanon
}

// newAttachedExec wires an Exec onto a real pty slave and a connected unix
// socket pair, then drives startConnectionManager so the stdin->socket copier
// is parked in a blocking read on the tty — exactly the state a live attached
// session sits in. The returned tty is the parent-facing terminal whose
// restoration the caller asserts.
func newAttachedExec(t *testing.T) (*Exec, *os.File) {
	t.Helper()
	tty := openTTY(t)
	client, _ := unixSocketPair(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	runPath := t.TempDir()
	sr := &Exec{
		id:         api.ID("client-restore"),
		ctx:        ctx,
		ctxCancel:  cancel,
		logger:     testLogger(),
		events:     make(chan Event, 8),
		metadataMu: sync.RWMutex{},
		stdin:      tty,
		stdout:     tty,
		stderr:     tty,
		ioConn:     client,
		terminal:   &api.AttachedTerminal{Spec: &api.TerminalSpec{ID: api.ID("term-restore")}},
		metadata: api.ClientDoc{
			APIVersion: api.APIVersionV1Beta1,
			Kind:       api.KindClient,
			Spec: api.ClientSpec{
				ID:              api.ID("client-restore"),
				RunPath:         runPath,
				DetachKeystroke: false,
			},
		},
	}

	if err := sr.startConnectionManager(); err != nil {
		t.Fatalf("startConnectionManager: %v", err)
	}

	// Sanity: raw mode is active (echo + canonical input cleared) and the
	// descriptor is still pollable while the copier is parked on the read.
	nb, echo, icanon := ttyState(t, tty)
	if echo || icanon {
		t.Fatalf("after attach: ECHO=%v ICANON=%v; want both cleared (raw mode)", echo, icanon)
	}
	if !nb {
		t.Fatalf("after attach: descriptor unexpectedly blocking; deadline unblock would not work")
	}
	return sr, tty
}

// assertRestored verifies the parent terminal was handed back in a usable
// state: cooked line discipline (ECHO + ICANON set) and a blocking descriptor
// (O_NONBLOCK cleared) — the two conditions a shell needs to recover without a
// manual reset/stty sane.
func assertRestored(t *testing.T, tty *os.File) {
	t.Helper()
	nb, echo, icanon := ttyState(t, tty)
	if nb {
		t.Errorf("stdin descriptor left in O_NONBLOCK after teardown; parent shell read loop would misbehave")
	}
	if !echo {
		t.Errorf("ECHO not restored after teardown; parent shell would not echo keystrokes")
	}
	if !icanon {
		t.Errorf("ICANON not restored after teardown; parent shell line editing would be broken")
	}
}

// TestRestore_OnProcessExit asserts that the spawned-process-exit teardown
// path (EvCmdExited -> Close) restores the parent terminal: blocking stdin and
// cooked line discipline. Regression for issue #364.
func TestRestore_OnProcessExit(t *testing.T) {
	sr, tty := newAttachedExec(t)

	sockPath := filepath.Join(t.TempDir(), "ctrl.sock")
	if err := os.WriteFile(sockPath, []byte{}, 0o600); err != nil {
		t.Fatalf("seed socket file: %v", err)
	}
	sr.metadata.Spec.SockerCtrl = sockPath

	if err := sr.Close(nil); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertRestored(t, tty)
}

// TestRestore_OnDetach asserts that the user-detach teardown path
// (EvDetach -> Detach) restores the parent terminal, independently of the
// Close()/EvError path. Regression for issue #364.
func TestRestore_OnDetach(t *testing.T) {
	sr, tty := newAttachedExec(t)
	sr.terminalClient = &mockTerminalClient{} // detach succeeds by default

	if err := sr.Detach(); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	assertRestored(t, tty)
}

// assertNormalizeEmitted reads everything still in r and checks that the
// output-side normalization escape (leave alt-screen, show cursor, reset
// cursor style) was written by restoreParentTerminal. The caller closes the
// pipe writer before calling so ReadAll terminates on EOF.
func assertNormalizeEmitted(t *testing.T, r *os.File) {
	t.Helper()
	buf, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	out := string(buf)
	for _, want := range []string{
		"\x1b[?1049l", "\x1b[?25h", "\x1b[0 q",
		"\x1b[?1000l", "\x1b[?1002l", "\x1b[?1003l",
		"\x1b[?1006l", "\x1b[?1015l", "\x1b[?2004l",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing normalization escape %q in stdout %q", want, out)
		}
	}
}

// TestRestore_EmitsOutputNormalize_OnProcessExit asserts that the
// spawned-process-exit teardown path writes the output-side normalization
// escape to the parent terminal: leave alt-screen, show cursor, reset cursor
// style. Regression for issue #365.
func TestRestore_EmitsOutputNormalize_OnProcessExit(t *testing.T) {
	sr, _ := newAttachedExec(t)

	r, w, errPipe := os.Pipe()
	if errPipe != nil {
		t.Fatalf("Pipe: %v", errPipe)
	}
	t.Cleanup(func() { _ = r.Close() })
	sr.stdout = w

	sockPath := filepath.Join(t.TempDir(), "ctrl.sock")
	if errSeed := os.WriteFile(sockPath, []byte{}, 0o600); errSeed != nil {
		t.Fatalf("seed socket file: %v", errSeed)
	}
	sr.metadata.Spec.SockerCtrl = sockPath

	if errClose := sr.Close(nil); errClose != nil {
		t.Fatalf("Close: %v", errClose)
	}
	_ = w.Close()
	assertNormalizeEmitted(t, r)
}

// TestRestore_EmitsOutputNormalize_OnDetach asserts the user-detach teardown
// path writes the output-side normalization escape, independently of the
// Close()/EvError path. Regression for issue #365.
func TestRestore_EmitsOutputNormalize_OnDetach(t *testing.T) {
	sr, _ := newAttachedExec(t)
	sr.terminalClient = &mockTerminalClient{}

	r, w, errPipe := os.Pipe()
	if errPipe != nil {
		t.Fatalf("Pipe: %v", errPipe)
	}
	t.Cleanup(func() { _ = r.Close() })
	sr.stdout = w

	if errDetach := sr.Detach(); errDetach != nil {
		t.Fatalf("Detach: %v", errDetach)
	}
	_ = w.Close()
	assertNormalizeEmitted(t, r)
}

// TestRestore_OnDetach_RPCFailure asserts that when the Detach control-socket
// RPC fails, Exec.Detach still restores the parent terminal (cooked line
// discipline + blocking descriptor) and still emits the output normalization
// sequence, rather than returning early and stranding the terminal in raw mode.
// The user has committed to leaving via ^]^]; a server-side RPC failure is not a
// reason to keep the parent terminal broken. The "Detached" banner is suppressed
// on this path because it would be misleading. Regression for #383 Gap 1.
func TestRestore_OnDetach_RPCFailure(t *testing.T) {
	sr, tty := newAttachedExec(t)
	sr.terminalClient = &mockTerminalClient{
		detachFunc: func(_ context.Context, _ *api.ID) error { return errMockDetach },
	}

	r, w, errPipe := os.Pipe()
	if errPipe != nil {
		t.Fatalf("Pipe: %v", errPipe)
	}
	t.Cleanup(func() { _ = r.Close() })
	sr.stdout = w

	err := sr.Detach()
	if !errors.Is(err, errMockDetach) {
		t.Fatalf("Detach: want errMockDetach, got %v", err)
	}
	_ = w.Close()

	// Termios cooked + blocking descriptor restored despite the RPC failure.
	assertRestored(t, tty)

	buf, errRead := io.ReadAll(r)
	if errRead != nil {
		t.Fatalf("ReadAll: %v", errRead)
	}
	out := string(buf)
	for _, want := range []string{"\x1b[?1049l", "\x1b[?25h", "\x1b[0 q"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing normalization escape %q in stdout %q on RPC-failure path", want, out)
		}
	}
	if strings.Contains(out, "Detached") {
		t.Errorf("Detached banner should be suppressed on RPC-failure path, got %q", out)
	}
}

// TestDetach_BannerWrittenBeforeNormalize asserts that on the successful detach
// path the "Detached" banner is emitted from within restoreParentTerminal — at
// the post-drain, pre-termios-restore point — and precedes the output
// normalization sequence. This is the same relative order as before #383, but
// now downstream of the copier drain so the banner cannot interleave with
// mid-flush remote bytes. Regression for #383 Gap 2.
func TestDetach_BannerWrittenBeforeNormalize(t *testing.T) {
	sr, _ := newAttachedExec(t)
	sr.terminalClient = &mockTerminalClient{}

	r, w, errPipe := os.Pipe()
	if errPipe != nil {
		t.Fatalf("Pipe: %v", errPipe)
	}
	t.Cleanup(func() { _ = r.Close() })
	sr.stdout = w

	if err := sr.Detach(); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	_ = w.Close()

	buf, errRead := io.ReadAll(r)
	if errRead != nil {
		t.Fatalf("ReadAll: %v", errRead)
	}
	out := string(buf)
	bannerIdx := strings.Index(out, "Detached")
	normIdx := strings.Index(out, "\x1b[?1049l")
	if bannerIdx < 0 {
		t.Errorf("Detached banner missing on successful detach in stdout %q", out)
	}
	if normIdx < 0 {
		t.Errorf("normalization sequence missing on successful detach in stdout %q", out)
	}
	if bannerIdx >= 0 && normIdx >= 0 && bannerIdx > normIdx {
		t.Errorf("banner should precede normalize sequence; banner@%d normalize@%d in %q", bannerIdx, normIdx, out)
	}
}

// captureDetachOutput swaps stdout for a pipe, runs Detach, and returns every
// byte the teardown wrote — the banner plus the restore sequence.
func captureDetachOutput(t *testing.T, sr *Exec) string {
	t.Helper()
	r, w, errPipe := os.Pipe()
	if errPipe != nil {
		t.Fatalf("Pipe: %v", errPipe)
	}
	t.Cleanup(func() { _ = r.Close() })
	sr.stdout = w

	if err := sr.Detach(); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	_ = w.Close()

	buf, errRead := io.ReadAll(r)
	if errRead != nil {
		t.Fatalf("ReadAll: %v", errRead)
	}
	return string(buf)
}

// TestDetach_CursorStaysAfterBanner asserts the default detach restore leaves
// the cursor on the line after the "Detached" banner instead of yanking it to
// row 1, on a session that never entered the alt screen. The bare ?1049l of
// #365 performed an implicit DECRC with no prior save, which xterm documents
// as restoring the default (home) cursor; the DECSC prefix pins the restore
// to the banner position. Verified by feeding the teardown bytes into a vt10x
// terminal seeded mid-screen — the same parser the repro in #425 used.
func TestDetach_CursorStaysAfterBanner(t *testing.T) {
	sr, _ := newAttachedExec(t)
	sr.terminalClient = &mockTerminalClient{}

	out := captureDetachOutput(t, sr)

	if strings.Contains(out, "\x1b[2J") {
		t.Errorf("default detach must not erase the screen; got %q", out)
	}

	// Parent terminal mid-screen: four lines written, prompt on row 3.
	vt := vt10x.New(vt10x.WithSize(80, 24))
	_, _ = vt.Write([]byte("$ ls\r\nfile1\r\nfile2\r\n$ "))
	_, _ = vt.Write([]byte(out))

	// The banner's two \r\n frames put "Detached" on row 4 and the cursor on
	// row 5 col 0; the restore sequence must keep it there. (vt10x keeps a
	// single saved-cursor slot and swaps buffers on a normal-buffer ?1049l —
	// quirks that do not affect the cursor assertion.)
	cur := vt.Cursor()
	if cur.Y == 0 {
		t.Fatalf("detach restored the cursor to row 1 — the #425 jump; output %q", out)
	}
	if cur.X != 0 || cur.Y != 5 {
		t.Errorf("cursor after detach = (%d,%d), want (0,5) — the line after the banner", cur.X, cur.Y)
	}
}

// TestDetach_OptInClearScreen asserts --clear-on-detach performs a real
// screen erase — the \x1b[2J\x1b[H bytes on client stdout after the normalize
// sequence — not merely a cursor-home.
func TestDetach_OptInClearScreen(t *testing.T) {
	sr, _ := newAttachedExec(t)
	sr.terminalClient = &mockTerminalClient{}
	sr.metadata.Spec.ClearOnDetach = true

	out := captureDetachOutput(t, sr)

	clearIdx := strings.Index(out, "\x1b[2J\x1b[H")
	normIdx := strings.Index(out, "\x1b[?1049l")
	if clearIdx < 0 {
		t.Fatalf("opt-in detach clear must write \\x1b[2J\\x1b[H to stdout; got %q", out)
	}
	if normIdx < 0 || clearIdx < normIdx {
		t.Errorf("clear must follow the normalize sequence; clear@%d normalize@%d in %q", clearIdx, normIdx, out)
	}

	vt := vt10x.New(vt10x.WithSize(80, 24))
	_, _ = vt.Write([]byte("$ ls\r\nfile1\r\nfile2\r\n$ "))
	_, _ = vt.Write([]byte(out))
	if cur := vt.Cursor(); cur.X != 0 || cur.Y != 0 {
		t.Errorf("cursor after opt-in clear = (%d,%d), want home (0,0)", cur.X, cur.Y)
	}
}

// TestDetach_OptInClearScreen_RPCFailure asserts the opt-in clear still runs
// when the Detach RPC fails — the user committed to detaching either way.
func TestDetach_OptInClearScreen_RPCFailure(t *testing.T) {
	sr, _ := newAttachedExec(t)
	sr.terminalClient = &mockTerminalClient{
		detachFunc: func(_ context.Context, _ *api.ID) error { return errMockDetach },
	}
	sr.metadata.Spec.ClearOnDetach = true

	r, w, errPipe := os.Pipe()
	if errPipe != nil {
		t.Fatalf("Pipe: %v", errPipe)
	}
	t.Cleanup(func() { _ = r.Close() })
	sr.stdout = w

	if err := sr.Detach(); !errors.Is(err, errMockDetach) {
		t.Fatalf("Detach: want errMockDetach, got %v", err)
	}
	_ = w.Close()

	buf, errRead := io.ReadAll(r)
	if errRead != nil {
		t.Fatalf("ReadAll: %v", errRead)
	}
	if !strings.Contains(string(buf), "\x1b[2J\x1b[H") {
		t.Errorf("opt-in clear missing on RPC-failure detach; got %q", string(buf))
	}
}

// TestDetach_AltScreenSessionReturnsToNormalBuffer asserts a detach while the
// parent terminal sits in the alternate screen (a TUI session) still returns
// to the normal buffer with its content intact — the DECSC prefix must not
// break the ?1049l buffer switch. The pre-attach cursor restore itself cannot
// be asserted under vt10x: it keeps a single saved-cursor slot where
// xterm/VTE keep one per buffer, so the prefix's save lands in the wrong slot
// only in the emulator. See the escRestoreParentTerm comment.
func TestDetach_AltScreenSessionReturnsToNormalBuffer(t *testing.T) {
	sr, _ := newAttachedExec(t)
	sr.terminalClient = &mockTerminalClient{}

	out := captureDetachOutput(t, sr)

	vt := vt10x.New(vt10x.WithSize(80, 24))
	_, _ = vt.Write([]byte("before-tui\r\n"))
	_, _ = vt.Write([]byte("\x1b[?1049h\x1b[2J\x1b[HTUI APP"))
	_, _ = vt.Write([]byte(out))

	if vt.Mode()&vt10x.ModeAltScreen != 0 {
		t.Errorf("detach left the terminal in the alt screen; output %q", out)
	}
	if !strings.Contains(vt.String(), "before-tui") {
		t.Errorf("normal-buffer content lost across detach; screen %q", vt.String())
	}
	if strings.Contains(vt.String(), "TUI APP") {
		t.Errorf("alt-screen content leaked into the normal buffer; screen %q", vt.String())
	}
}

// TestRestore_Idempotent asserts the restore runs exactly once even when both
// the detach and the close paths fire (the live sequence: user detaches, then
// the conn-close EvError drives Close). The second teardown must be a safe
// no-op and leave the terminal cooked + blocking.
func TestRestore_Idempotent(t *testing.T) {
	sr, tty := newAttachedExec(t)
	sr.terminalClient = &mockTerminalClient{}

	if err := sr.Detach(); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	sockPath := filepath.Join(t.TempDir(), "ctrl.sock")
	if err := os.WriteFile(sockPath, []byte{}, 0o600); err != nil {
		t.Fatalf("seed socket file: %v", err)
	}
	sr.metadata.Spec.SockerCtrl = sockPath
	if err := sr.Close(nil); err != nil {
		t.Fatalf("Close after Detach: %v", err)
	}
	assertRestored(t, tty)
}
