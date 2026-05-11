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

package server_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
	"github.com/eminwux/sbsh/pkg/builder"
	publog "github.com/eminwux/sbsh/pkg/logging"
	rpcclient "github.com/eminwux/sbsh/pkg/rpcclient/terminal"
	"github.com/eminwux/sbsh/pkg/terminal/server"
)

// TestServer_PingWriteSubscribeStop exercises the public facade
// end-to-end against the wire protocol: dial via pkg/rpcclient/terminal,
// run Ping, push bytes through Write, register a Subscribe stream
// (FD-passing via SCM_RIGHTS), then drive a graceful Stop.
//
// The test deliberately uses the public surface only — no peeking at
// internal runner state — to mirror how an out-of-tree consumer would
// exercise the facade.
func TestServer_PingWriteSubscribeStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h := startTestServer(ctx, t)
	defer h.cleanup()

	client := rpcclient.NewUnix(h.socketPath, h.logger)
	defer client.Close()

	t.Run("Ping", func(t *testing.T) { runPing(ctx, t, client) })

	clientID := api.ID("subscriber-1")
	subConn, subErr := client.Subscribe(ctx, &api.SubscribeRequest{ClientID: clientID}, nil)
	if subErr != nil {
		t.Fatalf("Subscribe: %v", subErr)
	}
	defer subConn.Close()

	t.Run("Write", func(t *testing.T) { runWrite(ctx, t, client) })
	t.Run("SubscribeReceivesPTYBytes", func(t *testing.T) { runSubscribeRead(t, subConn) })
	t.Run("Stop", func(t *testing.T) { runStop(t, h) })
}

// TestServer_DrivenByBuildTerminalSpec locks in the contract that an
// out-of-tree caller can build a TerminalSpec via pkg/builder, pre-create
// the per-terminal log file with pkg/logging.NewFileLogger, and then
// drive pkg/terminal/server.Server.Serve to the api.Ready state — without
// the `spec.LogFile = ""` post-step that issue #204 documented as the
// workaround. The runner re-chmods spec.LogFile during StartTerminal;
// before pkg/logging existed, callers depending only on public surfaces
// hit chmod ENOENT here.
func TestServer_DrivenByBuildTerminalSpec(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	spec, err := builder.BuildTerminalSpec(ctx, logger, tmp,
		builder.WithCommand([]string{"/bin/sh"}),
	)
	if err != nil {
		t.Fatalf("BuildTerminalSpec: %v", err)
	}
	if spec.LogFile == "" {
		t.Fatal("BuildTerminalSpec produced an empty LogFile; expected a derived path")
	}

	fl, err := publog.NewFileLogger(spec.LogFile, spec.LogLevel)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	t.Cleanup(func() { _ = fl.File.Close() })

	listener, listenErr := net.Listen("unix", spec.SocketFile)
	if listenErr != nil {
		t.Fatalf("net.Listen: %v", listenErr)
	}

	spec.ShutdownGrace = 500 * time.Millisecond

	srv, newErr := server.New(spec, logger)
	if newErr != nil {
		t.Fatalf("server.New: %v", newErr)
	}

	serveErrCh := make(chan error, 1)
	go func() { serveErrCh <- srv.Serve(ctx, listener) }()

	if waitErr := waitReady(ctx, srv, 10*time.Second); waitErr != nil {
		t.Fatalf("waitReady: %v", waitErr)
	}

	if stopErr := srv.Stop(errors.New("test cleanup")); stopErr != nil {
		t.Fatalf("Stop: %v", stopErr)
	}
	select {
	case <-serveErrCh:
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return within 10s after Stop")
	}
}

// TestServer_New_RejectsInvalidSpec covers the constructor guards so
// downstream consumers get a clear error rather than a panic deep in
// the runner.
func TestServer_New_RejectsInvalidSpec(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if _, err := server.New(nil, logger); err == nil {
		t.Fatal("New(nil, ...) returned no error")
	}
	if _, err := server.New(&api.TerminalSpec{}, logger); err == nil {
		t.Fatal("New(spec with empty Command) returned no error")
	}
}

// TestServer_Serve_RejectsNilListener guards against the easiest
// caller mistake: passing nil.
func TestServer_Serve_RejectsNilListener(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, newErr := server.New(&api.TerminalSpec{Command: "/bin/sh"}, logger)
	if newErr != nil {
		t.Fatalf("New: %v", newErr)
	}
	if err := srv.Serve(context.Background(), nil); err == nil {
		t.Fatal("Serve(nil listener) returned no error")
	}
}

// testHarness bundles the resources a single integration test needs so
// the test functions stay readable and gocognit stays happy.
type testHarness struct {
	srv        *server.Server
	logger     *slog.Logger
	socketPath string
	serveErrCh chan error
	cleanup    func()
}

func startTestServer(ctx context.Context, t *testing.T) *testHarness {
	t.Helper()

	tmp := t.TempDir()
	socketPath := filepath.Join(tmp, "ctrl.sock")
	capturePath := filepath.Join(tmp, "capture.log")

	listener, listenErr := net.Listen("unix", socketPath)
	if listenErr != nil {
		t.Fatalf("net.Listen: %v", listenErr)
	}

	spec := &api.TerminalSpec{
		ID:          api.ID("test-server-1"),
		Name:        "test-server-1",
		Labels:      map[string]string{},
		Command:     "/bin/sh",
		CommandArgs: []string{},
		EnvInherit:  true,
		RunPath:     tmp,
		SocketFile:  socketPath,
		CaptureFile: capturePath,
		// LogFile intentionally empty; the runner skips perm-apply for it.

		// Keep the SIGTERM → SIGKILL grace tight so Stop returns
		// quickly even when /bin/sh ignores SIGTERM under PTY.
		ShutdownGrace: 500 * time.Millisecond,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	srv, newErr := server.New(spec, logger)
	if newErr != nil {
		t.Fatalf("server.New: %v", newErr)
	}

	serveErrCh := make(chan error, 1)
	go func() { serveErrCh <- srv.Serve(ctx, listener) }()

	if waitErr := waitReady(ctx, srv, 10*time.Second); waitErr != nil {
		t.Fatalf("waitReady: %v", waitErr)
	}

	cleanup := func() {
		_ = srv.Stop(errors.New("test cleanup"))
		select {
		case <-serveErrCh:
		case <-time.After(5 * time.Second):
			t.Logf("Serve did not return within 5s of cleanup Stop")
		}
	}

	return &testHarness{
		srv:        srv,
		logger:     logger,
		socketPath: socketPath,
		serveErrCh: serveErrCh,
		cleanup:    cleanup,
	}
}

func runPing(ctx context.Context, t *testing.T, client rpcclient.Client) {
	t.Helper()
	var pong api.PingMessage
	if err := client.Ping(ctx, &api.PingMessage{Message: "PING"}, &pong); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if pong.Message != "PONG" {
		t.Fatalf("Ping reply = %q, want %q", pong.Message, "PONG")
	}
}

func runWrite(ctx context.Context, t *testing.T, client rpcclient.Client) {
	t.Helper()
	if err := client.Write(ctx, &api.WriteRequest{Data: []byte("echo hi\n")}); err != nil {
		t.Fatalf("Write: %v", err)
	}
}

func runSubscribeRead(t *testing.T, conn net.Conn) {
	t.Helper()
	// The PTY echoes input; the shell also runs "echo hi" and prints
	// "hi". Either is enough — we just need to see *some* bytes
	// arrive on the subscribe stream to verify FD passing wired up
	// and the multiwriter is fanning out to subscribers.
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("subscribe read: %v (n=%d)", err, n)
	}
	if n == 0 {
		t.Fatal("subscribe read returned 0 bytes")
	}
	t.Logf("subscribe read %d bytes: %q", n, buf[:n])
}

func runStop(t *testing.T, h *testHarness) {
	t.Helper()
	stopReason := errors.New("test stop")
	if err := h.srv.Stop(stopReason); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case serveErr := <-h.serveErrCh:
		if serveErr == nil || serveErr.Error() == "" {
			t.Fatalf("Serve returned %v; expected stop reason to propagate", serveErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return within 10s after Stop")
	}
	// Drain the cleanup-side Stop so the deferred cleanup is a no-op
	// instead of waiting on a Serve that already returned.
	h.cleanup = func() {}
}

// waitReady polls Metadata until the runner reports api.Ready or the
// deadline expires. Returns the most recent error if the deadline hits
// first.
func waitReady(ctx context.Context, srv *server.Server, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		md, err := srv.Metadata()
		if err == nil && md != nil && md.Status.State == api.Ready {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("timed out waiting for api.Ready")
}
