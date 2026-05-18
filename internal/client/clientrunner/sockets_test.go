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
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

// newOpenSocketCtrlExec builds a minimal Exec wired with the fields
// OpenSocketCtrl touches on the client-runner side: ctx, logger, and a
// metadata block whose Spec.SockerCtrl points at the test's socket path.
// Mirrors the helper used by terminalrunner's TestOpenSocketCtrl_* cases.
func newOpenSocketCtrlExec(t *testing.T, socketCtrl string) *Exec {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Exec{
		ctx:       ctx,
		ctxCancel: cancel,
		logger:    logger,
		metadata: api.ClientDoc{
			Spec: api.ClientSpec{
				SockerCtrl: socketCtrl,
			},
		},
		metadataMu: sync.RWMutex{},
	}
}

// TestOpenSocketCtrl_RefusesBindOnLivePeer is the first AC for #266.
//
// Pre-fix, clientrunner.OpenSocketCtrl unconditionally `os.Remove`s the
// socket path before binding. If a live peer is already listening on that
// path (operator passed the same --id / --socket-file twice, two starts
// raced), the live inode is silently unlinked: the victim's listener still
// accepts on the inode it holds, but new clients hit ENOENT on dial and
// the older client's control socket mysteriously breaks.
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

// TestOpenSocketCtrl_CleansUpStaleSocket is the second AC for #266.
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
