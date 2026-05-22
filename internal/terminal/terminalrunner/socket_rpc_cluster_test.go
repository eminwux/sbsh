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

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/terminal/terminalrpc"
	"github.com/eminwux/sbsh/pkg/api"
	"golang.org/x/sys/unix"
)

// newWiredExec builds an Exec with a live PTY fan-out (multiOutW) and an
// os.Pipe standing in for the PTY stdin path, so the socket/RPC-wiring
// functions that register clients on the fan-out (CreateNewClient ->
// handleClient -> cleanupClient) run end to end without a real PTY. Metadata
// writes target a real terminals/<id> dir so updateTerminalAttachers succeeds
// quietly.
func newWiredExec(t *testing.T) *Exec {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runPath := t.TempDir()
	id := api.ID("wired-test")
	if err := os.MkdirAll(filepath.Join(runPath, "terminals", string(id)), 0o700); err != nil {
		t.Fatalf("mkdir terminal dir: %v", err)
	}
	pipeInR, pipeInW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = pipeInR.Close()
		_ = pipeInW.Close()
	})
	sr := &Exec{
		ctx:       ctx,
		ctxCancel: cancel,
		logger:    logger,
		id:        id,
		metadata: api.TerminalDoc{
			Spec: api.TerminalSpec{RunPath: runPath},
		},
		clients:       make(map[api.ID]*ioClient),
		subscribers:   make(map[*subscriberWriter]struct{}),
		closeReqCh:    make(chan error, 1),
		closedCh:      make(chan struct{}),
		childDoneCh:   make(chan struct{}),
		childDoneOnce: &sync.Once{},
		ptyPipes: &ptyPipes{
			pipeInR:   pipeInR,
			pipeInW:   pipeInW,
			multiOutW: NewDynamicMultiWriter(logger),
		},
		closePTY:      &sync.Once{},
		closeClosedCh: &sync.Once{},
	}
	sr.gates.StdinOpen = true
	sr.gates.OutputOn = true
	return sr
}

// applySocketPerms with an explicit mode chmods the inode to that mode and a
// zero mode falls back to defaultSocketMode. A non-nil gid set to the caller's
// own gid exercises the chown arm without requiring privilege.
func TestApplySocketPerms(t *testing.T) {
	sr := newWiredExec(t)

	t.Run("explicit mode", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "s.sock")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		if err := sr.applySocketPerms(path, 0o660, nil); err != nil {
			t.Fatalf("applySocketPerms: %v", err)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if got := fi.Mode().Perm(); got != 0o660 {
			t.Errorf("mode = %o, want 660", got)
		}
	})

	t.Run("zero mode falls back to default", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "s.sock")
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		if err := sr.applySocketPerms(path, 0, nil); err != nil {
			t.Fatalf("applySocketPerms: %v", err)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if got := fi.Mode().Perm(); got != defaultSocketMode.Perm() {
			t.Errorf("mode = %o, want %o (default)", got, defaultSocketMode.Perm())
		}
	})

	t.Run("chmod error on missing path", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist.sock")
		if err := sr.applySocketPerms(missing, 0o600, nil); err == nil {
			t.Fatal("applySocketPerms on missing path returned nil; want chmod error")
		}
	})

	t.Run("gid chown arm", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "s.sock")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		gid := os.Getgid()
		if err := sr.applySocketPerms(path, 0o600, &gid); err != nil {
			t.Fatalf("applySocketPerms with gid: %v", err)
		}
	})
}

// applyArtifactPerms mirrors applySocketPerms for capture/log artifacts.
func TestApplyArtifactPerms(t *testing.T) {
	sr := newWiredExec(t)

	t.Run("explicit mode", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "capture.raw")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		if err := sr.applyArtifactPerms("capture", path, 0o640, nil); err != nil {
			t.Fatalf("applyArtifactPerms: %v", err)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if got := fi.Mode().Perm(); got != 0o640 {
			t.Errorf("mode = %o, want 640", got)
		}
	})

	t.Run("zero mode falls back to default", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "capture.raw")
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		if err := sr.applyArtifactPerms("capture", path, 0, nil); err != nil {
			t.Fatalf("applyArtifactPerms: %v", err)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if got := fi.Mode().Perm(); got != defaultArtifactMode.Perm() {
			t.Errorf("mode = %o, want %o (default)", got, defaultArtifactMode.Perm())
		}
	})

	t.Run("chmod error on missing path", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nope.raw")
		if err := sr.applyArtifactPerms("capture", missing, 0o600, nil); err == nil {
			t.Fatal("applyArtifactPerms on missing path returned nil; want chmod error")
		}
	})

	t.Run("gid chown arm", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "log.txt")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		gid := os.Getgid()
		if err := sr.applyArtifactPerms("logfile", path, 0o600, &gid); err != nil {
			t.Fatalf("applyArtifactPerms with gid: %v", err)
		}
	})
}

// OpenSocketCtrl fails fast when the very first metadata write cannot land
// because the terminals/<id> directory does not exist — exercising the
// early updateMetadata error return before any listener is bound.
func TestOpenSocketCtrl_MetadataWriteFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// RunPath points at a temp dir, but the terminals/<id> subdir is
	// deliberately NOT created, so WriteMetadata's CreateTemp fails.
	sr := &Exec{
		ctx:       ctx,
		ctxCancel: cancel,
		logger:    logger,
		id:        api.ID("missing-dir"),
		metadata: api.TerminalDoc{
			Spec: api.TerminalSpec{
				RunPath:    t.TempDir(),
				SocketFile: filepath.Join(t.TempDir(), "ctrl.sock"),
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
	if err := sr.OpenSocketCtrl(); err == nil {
		t.Fatal("OpenSocketCtrl returned nil with an unwritable metadata dir; want error")
	}
	if sr.lnCtrl != nil {
		t.Error("OpenSocketCtrl bound a listener despite the early metadata failure")
	}
}

// CreateNewClient allocates a socketpair, registers the server side on the
// fan-out via handleClient, and returns the client FD. Closing the client FD
// drives the reader-side detach through cleanupClient, which removes the
// client from the registry. This covers CreateNewClient, handleClient, and
// cleanupClient end to end.
func TestCreateNewClient_RegistersAndCleansUp(t *testing.T) {
	sr := newWiredExec(t)

	id := api.ID("attach-1")
	cliFD, err := sr.CreateNewClient(&id, false)
	if err != nil {
		t.Fatalf("CreateNewClient: %v", err)
	}
	if cliFD < 0 {
		t.Fatalf("CreateNewClient returned invalid fd %d", cliFD)
	}

	// addClient runs synchronously inside CreateNewClient before handleClient
	// is spawned, so the client is registered the moment we return.
	if _, ok := sr.getClient(id); !ok {
		t.Fatal("client not registered after CreateNewClient")
	}

	// Closing the client end makes the server-side reader observe EOF, which
	// funnels through detach -> cleanupClient -> removeClient.
	if err := unix.Close(cliFD); err != nil {
		t.Fatalf("close client fd: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := sr.getClient(id); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("client not cleaned up within 2s after client-side close")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// cleanupClient's early-failure fallback covers the path where handleClient
// never wired an outWriter (e.g. an early cast failure): cleanupClient must
// close client.conn directly rather than relying on a drain goroutine.
func TestCleanupClient_EarlyFailureClosesConn(t *testing.T) {
	sr := newWiredExec(t)

	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	id := api.ID("early-fail")
	cl := &ioClient{id: &id, conn: serverSide}
	sr.addClient(cl)

	sr.cleanupClient(cl)

	if _, ok := sr.getClient(id); ok {
		t.Fatal("cleanupClient did not remove the client from the registry")
	}
	// The fallback closed serverSide directly; a write must now fail.
	if _, err := serverSide.Write([]byte("x")); err == nil {
		t.Error("cleanupClient left the conn open on the early-failure path")
	}
}

// getClientList, removeClient, and getClient are the small registry helpers.
func TestClientRegistryHelpers(t *testing.T) {
	sr := newWiredExec(t)

	idA := api.ID("a")
	idB := api.ID("b")
	clA := &ioClient{id: &idA}
	clB := &ioClient{id: &idB}
	sr.addClient(clA)
	sr.addClient(clB)

	if got := len(sr.getClientList()); got != 2 {
		t.Fatalf("getClientList len = %d, want 2", got)
	}

	if _, ok := sr.getClient(idA); !ok {
		t.Error("getClient(a) = false, want true")
	}

	sr.removeClient(clA)
	if _, ok := sr.getClient(idA); ok {
		t.Error("getClient(a) after removeClient = true, want false")
	}
	if got := len(sr.getClientList()); got != 1 {
		t.Errorf("getClientList len after remove = %d, want 1", got)
	}
}

// WritePTY covers each guard plus the success and write-failure paths.
func TestWritePTY(t *testing.T) {
	t.Run("empty data is a no-op", func(t *testing.T) {
		sr := newWiredExec(t)
		if err := sr.WritePTY(nil); err != nil {
			t.Errorf("WritePTY(nil) = %v, want nil", err)
		}
	})

	t.Run("stdin closed", func(t *testing.T) {
		sr := newWiredExec(t)
		sr.gates.StdinOpen = false
		if err := sr.WritePTY([]byte("x")); err != errdefs.ErrTerminalStdinClosed {
			t.Errorf("WritePTY with stdin closed = %v, want ErrTerminalStdinClosed", err)
		}
	})

	t.Run("nil ptmx", func(t *testing.T) {
		sr := newWiredExec(t)
		sr.ptmx = nil
		if err := sr.WritePTY([]byte("x")); err != errdefs.ErrTerminalStdinClosed {
			t.Errorf("WritePTY with nil ptmx = %v, want ErrTerminalStdinClosed", err)
		}
	})

	t.Run("success increments bytesIn", func(t *testing.T) {
		sr := newWiredExec(t)
		pr, pw, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe: %v", err)
		}
		defer pr.Close()
		defer pw.Close()
		sr.ptmx = pw

		// Drain the read end so the write cannot block on a full pipe buffer.
		done := make(chan struct{})
		go func() {
			defer close(done)
			buf := make([]byte, 64)
			_, _ = pr.Read(buf)
		}()

		data := []byte("hello")
		if err := sr.WritePTY(data); err != nil {
			t.Fatalf("WritePTY: %v", err)
		}
		<-done
		sr.obsMu.RLock()
		got := sr.bytesIn
		sr.obsMu.RUnlock()
		if got != uint64(len(data)) {
			t.Errorf("bytesIn = %d, want %d", got, len(data))
		}
	})

	t.Run("write failure on broken pipe", func(t *testing.T) {
		sr := newWiredExec(t)
		pr, pw, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe: %v", err)
		}
		// Closing the read end makes writes to pw fail with EPIPE.
		_ = pr.Close()
		sr.ptmx = pw
		defer pw.Close()

		if err := sr.WritePTY([]byte("data")); err == nil {
			t.Error("WritePTY to a broken pipe returned nil; want write error")
		}
	})
}

// Subscribe rejects the registration when the PTY fan-out is not wired
// (terminal not running) and tears down the freshly allocated socketpair
// rather than leaking the fds.
func TestSubscribe_NotRunning(t *testing.T) {
	sr := newWiredExec(t)
	// Clear the fan-out so Subscribe's "terminal not running" guard fires.
	sr.ptyPipesMu.Lock()
	sr.ptyPipes.multiOutW = nil
	sr.ptyPipesMu.Unlock()

	var resp api.ResponseWithFD
	err := sr.Subscribe(&api.SubscribeRequest{ClientID: api.ID("sub-1")}, &resp)
	if err == nil {
		t.Fatal("Subscribe with nil multiOutW returned nil; want terminal-not-running error")
	}
	if len(resp.FDs) != 0 {
		t.Errorf("Subscribe returned FDs on the not-running path: %v", resp.FDs)
	}
}

// Subscribe's early-shutdown guard rejects a registration that arrives after
// the runner's context has been canceled, returning errTerminalClosing and
// registering no subscriber.
func TestSubscribe_EarlyShutdown(t *testing.T) {
	sr := newWiredExec(t)
	sr.ctxCancel() // flip the context before the RPC lands

	var resp api.ResponseWithFD
	err := sr.Subscribe(&api.SubscribeRequest{ClientID: api.ID("sub-2")}, &resp)
	if err != errTerminalClosing {
		t.Fatalf("Subscribe after ctx cancel = %v, want errTerminalClosing", err)
	}
	sr.subsMu.Lock()
	n := len(sr.subscribers)
	sr.subsMu.Unlock()
	if n != 0 {
		t.Errorf("Subscribe registered %d subscribers on the shutdown path; want 0", n)
	}
}

// StartServer rejects an extra handler whose Name collides with the built-in
// service name and reports the failure on both readyCh and doneCh without
// entering the accept loop.
func TestStartServer_ExtraHandlerCollision(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sr, cleanup := newStartServerTestExec(t, logger)
	defer cleanup()

	readyCh := make(chan error, 1)
	doneCh := make(chan error, 1)
	svc := &terminalrpc.TerminalControllerRPC{Core: nil}
	collide := terminalrpc.ExtraHandler{Name: api.TerminalService, Receiver: svc}

	go sr.StartServer(sr.ctx, svc, readyCh, doneCh, collide)

	select {
	case err := <-readyCh:
		if err == nil {
			t.Fatal("StartServer accepted a colliding extra-handler name; want registration error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartServer did not report the collision on readyCh within 2s")
	}

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("StartServer did not signal doneCh after the collision")
	}
}

// StartServer's accept loop accepts a real Unix connection, casts it, and
// hands it to ServeCodec on its own goroutine. Dialing the listener exercises
// the accept+cast+serve happy path; ctx cancel then drives the graceful exit.
func TestStartServer_AcceptsConnection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sr, cleanup := newStartServerTestExec(t, logger)
	defer cleanup()

	readyCh := make(chan error, 1)
	doneCh := make(chan error, 1)
	svc := &terminalrpc.TerminalControllerRPC{Core: nil}

	go sr.StartServer(sr.ctx, svc, readyCh, doneCh)

	select {
	case err := <-readyCh:
		if err != nil {
			t.Fatalf("StartServer ready returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartServer did not signal ready within 2s")
	}

	conn, err := net.Dial("unix", sr.lnCtrl.Addr().String())
	if err != nil {
		t.Fatalf("dial control socket: %v", err)
	}
	// Give the accept loop a moment to accept and spawn ServeCodec.
	time.Sleep(20 * time.Millisecond)
	_ = conn.Close()

	sr.ctxCancel()
	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("graceful exit delivered non-nil err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartServer did not exit within 2s after ctx cancel")
	}
}
