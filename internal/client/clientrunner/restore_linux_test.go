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
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"github.com/eminwux/sbsh/pkg/api"
	"golang.org/x/sys/unix"
)

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
