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
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
	"golang.org/x/sys/unix"
)

// newOpenSocketCtrlExec builds a minimal Exec wired with the fields
// OpenSocketCtrl touches: ctx, logger, id, and a metadata block whose
// Spec.RunPath points at a writable temp dir so updateMetadata can
// persist the terminal doc without a real terminal directory layout.
// Tests pass socketFile through metadata.Spec.SocketFile.
func newOpenSocketCtrlExec(t *testing.T, socketFile string) *Exec {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runPath := t.TempDir()
	id := api.ID("opensocketctrl-test")
	if err := os.MkdirAll(filepath.Join(runPath, "terminals", string(id)), 0o700); err != nil {
		t.Fatalf("mkdir terminal dir: %v", err)
	}
	return &Exec{
		ctx:       ctx,
		ctxCancel: cancel,
		logger:    logger,
		id:        id,
		metadata: api.TerminalDoc{
			Spec: api.TerminalSpec{
				RunPath:    runPath,
				SocketFile: socketFile,
			},
		},
		clients:       make(map[api.ID]*ioClient),
		subscribers:   make(map[*subscriberWriter]struct{}),
		closeReqCh:    make(chan error, 1),
		closedCh:      make(chan struct{}),
		childDoneCh:   make(chan struct{}),
		childDoneOnce: &sync.Once{},
		ptyPipes:      &ptyPipes{},
		closePTY:      &sync.Once{},
		closeClosedCh: &sync.Once{},
	}
}

// TestOpenSocketCtrl_RefusesBindOnLivePeer is the first AC for #261.
//
// Pre-fix, OpenSocketCtrl unconditionally `os.Remove`s the socket path
// before binding. If a live peer is already listening on that path
// (operator passed the same --id / --socket-file twice, two starts raced),
// the live inode is silently unlinked: the victim's listener still accepts
// on the inode it holds, but new clients hit ENOENT on dial and attach
// against the older terminal mysteriously breaks.
//
// Post-fix, OpenSocketCtrl probes the path with a short DialTimeout. A
// successful dial means a peer is serving; the bind must refuse with an
// error and the pre-bound listener must remain serviceable.
func TestOpenSocketCtrl_RefusesBindOnLivePeer(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "ctrl.sock")

	peer, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("pre-bind peer listener: %v", err)
	}
	defer peer.Close()

	acceptedCh := make(chan net.Conn, 1)
	go func() {
		c, _ := peer.Accept()
		if c != nil {
			acceptedCh <- c
		}
	}()

	sr := newOpenSocketCtrlExec(t, sockPath)

	err = sr.OpenSocketCtrl()
	if err == nil {
		t.Fatalf("OpenSocketCtrl returned nil; want error refusing bind on live peer")
	}

	// Pre-bound peer must still serve new clients — the bind refusal means
	// the original inode survives, so DialTimeout below succeeds and
	// peer.Accept returns the connection.
	conn, dialErr := net.DialTimeout("unix", sockPath, time.Second)
	if dialErr != nil {
		t.Fatalf("pre-bound peer no longer accepts after OpenSocketCtrl refusal: %v", dialErr)
	}
	defer conn.Close()

	select {
	case accepted := <-acceptedCh:
		_ = accepted.Close()
	case <-time.After(time.Second):
		t.Fatal("pre-bound peer did not accept the probe connection within 1s")
	}
}

// TestOpenSocketCtrl_CleansUpStaleSocket is the second AC for #261.
//
// A stale socket file (an inode left by a prior process that has since
// exited — no peer is accepting) must still be cleaned up and the bind
// must proceed. The probe's DialTimeout returns ECONNREFUSED for a stale
// socket file, which the implementation treats as "no peer; safe to
// unlink".
func TestOpenSocketCtrl_CleansUpStaleSocket(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "ctrl.sock")

	// Create a stale socket file: bind a listener, then close it without
	// removing the inode (simulates a prior process that didn't clean up).
	stale, errListen := net.Listen("unix", sockPath)
	if errListen != nil {
		t.Fatalf("create stale listener: %v", errListen)
	}
	if unixLn, ok := stale.(*net.UnixListener); ok {
		unixLn.SetUnlinkOnClose(false)
	}
	if errClose := stale.Close(); errClose != nil {
		t.Fatalf("close stale listener: %v", errClose)
	}
	if _, errStat := os.Stat(sockPath); errStat != nil {
		t.Fatalf("stale socket inode missing before OpenSocketCtrl: %v", errStat)
	}

	sr := newOpenSocketCtrlExec(t, sockPath)
	t.Cleanup(func() {
		if sr.lnCtrl != nil {
			_ = sr.lnCtrl.Close()
		}
	})

	if errOpen := sr.OpenSocketCtrl(); errOpen != nil {
		t.Fatalf("OpenSocketCtrl returned error on stale-socket cleanup path: %v", errOpen)
	}
	if sr.lnCtrl == nil {
		t.Fatal("OpenSocketCtrl succeeded but lnCtrl is nil; expected bound listener")
	}

	// Listener must actually be serving the path now — dial it.
	conn, dialErr := net.DialTimeout("unix", sockPath, time.Second)
	if dialErr != nil {
		t.Fatalf("new listener does not accept dials after stale cleanup: %v", dialErr)
	}
	_ = conn.Close()
}

// TestListenUnixWithMode_SocketBornAtRequestedMode is the AC for #361.
//
// Pre-fix, OpenSocketCtrl called net.ListenConfig.Listen directly. The
// socket inode was born at (0o666 & ~processUmask) — typically 0o600
// under the common 0o077 daemon umask — and only later chmoded to the
// configured 0o660. A group-member client that dialed in that window
// hit EACCES even though the final post-chmod state would have admitted
// it.
//
// Post-fix, listenUnixWithMode saves+sets umask around the bind so the
// inode is born at the requested mode without any chmod having to run.
// This test pre-sets a hostile umask (0o077, the common daemon default)
// and asserts the inode mode immediately after listenUnixWithMode
// returns — *before* any applySocketPerms chmod could mask the bug.
func TestListenUnixWithMode_SocketBornAtRequestedMode(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "ctrl.sock")

	prev := unix.Umask(0o077)
	defer unix.Umask(prev)

	ln, err := listenUnixWithMode(context.Background(), sockPath, 0o660)
	if err != nil {
		t.Fatalf("listenUnixWithMode: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	info, errStat := os.Stat(sockPath)
	if errStat != nil {
		t.Fatalf("stat socket: %v", errStat)
	}
	if got := info.Mode().Perm(); got != 0o660 {
		t.Fatalf("socket inode mode = 0o%o; want 0o660 (born from umask, no chmod ran)", got)
	}
}

// TestListenUnixWithMode_DefaultModeWhenZero confirms the mode==0
// fallback path: listenUnixWithMode applies defaultSocketMode (0o600)
// when the caller passes 0, matching applySocketPerms' contract so the
// raw spec field can flow through without a guard at the call site.
func TestListenUnixWithMode_DefaultModeWhenZero(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "ctrl.sock")

	prev := unix.Umask(0o022)
	defer unix.Umask(prev)

	ln, err := listenUnixWithMode(context.Background(), sockPath, 0)
	if err != nil {
		t.Fatalf("listenUnixWithMode: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	info, errStat := os.Stat(sockPath)
	if errStat != nil {
		t.Fatalf("stat socket: %v", errStat)
	}
	if got := info.Mode().Perm(); got != defaultSocketMode {
		t.Fatalf("socket inode mode = 0o%o; want 0o%o (default fallback)", got, defaultSocketMode)
	}
}

// TestOpenSocketCtrl_SocketReachableAtConfiguredMode is the end-to-end
// AC for #361. After OpenSocketCtrl returns, the control socket inode
// must be at the configured SocketMode regardless of the daemon's
// process umask. Pre-fix, the listener was born at 0o600 under a 0o077
// umask and only later chmoded to 0o660; post-fix, the umask path
// produces the right mode at bind time and applySocketPerms re-asserts
// it idempotently.
func TestOpenSocketCtrl_SocketReachableAtConfiguredMode(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "ctrl.sock")

	prev := unix.Umask(0o077)
	defer unix.Umask(prev)

	sr := newOpenSocketCtrlExec(t, sockPath)
	sr.metadata.Spec.SocketMode = 0o660
	t.Cleanup(func() {
		if sr.lnCtrl != nil {
			_ = sr.lnCtrl.Close()
		}
	})

	if errOpen := sr.OpenSocketCtrl(); errOpen != nil {
		t.Fatalf("OpenSocketCtrl: %v", errOpen)
	}

	info, errStat := os.Stat(sockPath)
	if errStat != nil {
		t.Fatalf("stat socket: %v", errStat)
	}
	if got := info.Mode().Perm(); got != 0o660 {
		t.Fatalf("socket mode = 0o%o; want 0o660", got)
	}
}
