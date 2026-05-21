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

//go:build unix

package terminal

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"path/filepath"
	"sync"
	"testing"
	"time"

	internalterminal "github.com/eminwux/sbsh/internal/terminal"
	"github.com/eminwux/sbsh/internal/terminal/terminalrpc"
	"github.com/eminwux/sbsh/pkg/api"
	"golang.org/x/sys/unix"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// startTermServer wires a real net/rpc server backed by the given fake
// TerminalController, listening on a tempdir Unix socket and serving the
// project's FD-aware unixJSONServerCodec — the same codec the production
// control server uses. This exercises the real wire format (including
// SCM_RIGHTS FD passing for Attach/Subscribe) against the client codec.
func startTermServer(t *testing.T, core *internalterminal.ControllerTest) string {
	t.Helper()

	dir := t.TempDir()
	sockPath := filepath.Join(dir, "term.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := rpc.NewServer()
	if errReg := srv.RegisterName(api.TerminalService, &terminalrpc.TerminalControllerRPC{Core: core}); errReg != nil {
		_ = ln.Close()
		t.Fatalf("RegisterName: %v", errReg)
	}

	logger := testLogger()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, errAccept := ln.Accept()
			if errAccept != nil {
				return
			}
			uconn, ok := conn.(*net.UnixConn)
			if !ok {
				_ = conn.Close()
				continue
			}
			go srv.ServeCodec(terminalrpc.NewUnixJSONServerCodec(uconn, logger))
		}
	}()

	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})

	return sockPath
}

func newTestClient(t *testing.T, core *internalterminal.ControllerTest) Client {
	t.Helper()
	sock := startTermServer(t, core)
	c := NewUnix(sock, testLogger())
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func ctx2s(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestTerminal_Ping_Roundtrip(t *testing.T) {
	t.Parallel()
	var gotIn *api.PingMessage
	core := &internalterminal.ControllerTest{
		PingFunc: func(in *api.PingMessage) (*api.PingMessage, error) {
			gotIn = in
			return &api.PingMessage{Message: "PONG"}, nil
		},
	}
	c := newTestClient(t, core)

	out := &api.PingMessage{}
	if err := c.Ping(ctx2s(t), &api.PingMessage{Message: "PING"}, out); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if out.Message != "PONG" {
		t.Fatalf("Ping reply = %q; want PONG", out.Message)
	}
	if gotIn == nil || gotIn.Message != "PING" {
		t.Fatalf("server saw %+v; want PING", gotIn)
	}
}

func TestTerminal_Ping_PropagatesError(t *testing.T) {
	t.Parallel()
	core := &internalterminal.ControllerTest{
		PingFunc: func(_ *api.PingMessage) (*api.PingMessage, error) {
			return nil, errors.New("nope")
		},
	}
	c := newTestClient(t, core)
	if err := c.Ping(ctx2s(t), &api.PingMessage{}, &api.PingMessage{}); err == nil {
		t.Fatalf("Ping err = nil; want non-nil")
	}
}

func TestTerminal_Resize(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	core := &internalterminal.ControllerTest{ResizeFunc: func() { close(done) }}
	c := newTestClient(t, core)

	if err := c.Resize(ctx2s(t), &api.ResizeArgs{Cols: 80, Rows: 24}); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("server Resize not invoked")
	}
}

func TestTerminal_Detach(t *testing.T) {
	t.Parallel()
	var gotID *api.ID
	done := make(chan struct{})
	core := &internalterminal.ControllerTest{
		DetachFunc: func(id *api.ID) error { gotID = id; close(done); return nil },
	}
	c := newTestClient(t, core)

	id := api.ID("term-1")
	if err := c.Detach(ctx2s(t), &id); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("server Detach not invoked")
	}
	if gotID == nil || *gotID != "term-1" {
		t.Fatalf("server saw id %v; want term-1", gotID)
	}
}

func TestTerminal_Metadata_Roundtrip(t *testing.T) {
	t.Parallel()
	want := &api.TerminalDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminal,
		Metadata:   api.TerminalMetadata{Name: "t-rpc"},
		Spec:       api.TerminalSpec{ID: "t-rpc", Command: "bash"},
		Status:     api.TerminalStatus{Pid: 123, State: api.Ready},
	}
	core := &internalterminal.ControllerTest{
		MetadataFunc: func() (*api.TerminalDoc, error) { return want, nil },
	}
	c := newTestClient(t, core)

	got := &api.TerminalDoc{}
	if err := c.Metadata(ctx2s(t), got); err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if got.Metadata.Name != "t-rpc" || got.Spec.Command != "bash" ||
		got.Status.Pid != 123 || got.Status.State != api.Ready {
		t.Fatalf("Metadata reply = %+v; want %+v", got, want)
	}
}

func TestTerminal_Metadata_PropagatesError(t *testing.T) {
	t.Parallel()
	core := &internalterminal.ControllerTest{
		MetadataFunc: func() (*api.TerminalDoc, error) { return nil, errors.New("boom") },
	}
	c := newTestClient(t, core)
	if err := c.Metadata(ctx2s(t), &api.TerminalDoc{}); err == nil {
		t.Fatalf("Metadata err = nil; want wrapped error")
	}
}

func TestTerminal_State_Roundtrip(t *testing.T) {
	t.Parallel()
	state := api.Ready
	core := &internalterminal.ControllerTest{
		StateFunc: func() (*api.TerminalStatusMode, error) { return &state, nil },
	}
	c := newTestClient(t, core)

	got := api.Initializing
	if err := c.State(ctx2s(t), &got); err != nil {
		t.Fatalf("State: %v", err)
	}
	if got != api.Ready {
		t.Fatalf("State reply = %v; want %v", got, api.Ready)
	}
}

func TestTerminal_State_PropagatesError(t *testing.T) {
	t.Parallel()
	core := &internalterminal.ControllerTest{
		StateFunc: func() (*api.TerminalStatusMode, error) { return nil, errors.New("boom") },
	}
	c := newTestClient(t, core)
	got := api.Initializing
	if err := c.State(ctx2s(t), &got); err == nil {
		t.Fatalf("State err = nil; want wrapped error")
	}
}

func TestTerminal_Stop(t *testing.T) {
	t.Parallel()
	var gotArgs *api.StopArgs
	done := make(chan struct{})
	core := &internalterminal.ControllerTest{
		StopFunc: func(args *api.StopArgs) error { gotArgs = args; close(done); return nil },
	}
	c := newTestClient(t, core)

	if err := c.Stop(ctx2s(t), &api.StopArgs{Reason: "test"}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("server Stop not invoked")
	}
	if gotArgs == nil || gotArgs.Reason != "test" {
		t.Fatalf("Stop args = %+v; want Reason=test", gotArgs)
	}
}

func TestTerminal_Write(t *testing.T) {
	t.Parallel()
	var gotReq *api.WriteRequest
	done := make(chan struct{})
	core := &internalterminal.ControllerTest{
		WriteFunc: func(req *api.WriteRequest) error { gotReq = req; close(done); return nil },
	}
	c := newTestClient(t, core)

	if err := c.Write(ctx2s(t), &api.WriteRequest{Data: []byte("hi")}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("server Write not invoked")
	}
	if gotReq == nil || string(gotReq.Data) != "hi" {
		t.Fatalf("Write req = %+v; want Data=hi", gotReq)
	}
}

func TestTerminal_Write_NilReqTolerated(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	core := &internalterminal.ControllerTest{
		WriteFunc: func(_ *api.WriteRequest) error { close(done); return nil },
	}
	c := newTestClient(t, core)

	if err := c.Write(ctx2s(t), nil); err != nil {
		t.Fatalf("Write(nil): %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("server Write not invoked")
	}
}

func TestTerminal_Screenshot_Roundtrip(t *testing.T) {
	t.Parallel()
	want := &api.ScreenshotResult{Cols: 80, Rows: 24, Text: "hello", ANSI: "hello"}
	core := &internalterminal.ControllerTest{
		ScreenshotFunc: func(_ *api.ScreenshotArgs) (*api.ScreenshotResult, error) { return want, nil },
	}
	c := newTestClient(t, core)

	got := &api.ScreenshotResult{}
	if err := c.Screenshot(ctx2s(t), nil, got); err != nil { // nil args exercises substitution
		t.Fatalf("Screenshot: %v", err)
	}
	if got.Cols != 80 || got.Rows != 24 || got.Text != "hello" {
		t.Fatalf("Screenshot reply = %+v; want %+v", got, want)
	}
}

func TestTerminal_Screenshot_PropagatesError(t *testing.T) {
	t.Parallel()
	core := &internalterminal.ControllerTest{
		ScreenshotFunc: func(_ *api.ScreenshotArgs) (*api.ScreenshotResult, error) {
			return nil, errors.New("boom")
		},
	}
	c := newTestClient(t, core)
	if err := c.Screenshot(ctx2s(t), &api.ScreenshotArgs{}, &api.ScreenshotResult{}); err == nil {
		t.Fatalf("Screenshot err = nil; want wrapped error")
	}
}

// fdPair returns a connected socketpair as two *os.File-free fds. ours is kept
// by the test (write side for assertions); theirs is handed to the server to
// pass back over SCM_RIGHTS. The server codec closes the fd it sends, so theirs
// must not be closed by the test.
func fdPair(t *testing.T) (int, int) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	return fds[0], fds[1]
}

func TestTerminal_Attach_ReturnsConnAndResult(t *testing.T) {
	t.Parallel()
	ours, theirs := fdPair(t)
	t.Cleanup(func() { _ = unix.Close(ours) })

	var gotReq *api.AttachRequest
	core := &internalterminal.ControllerTest{
		AttachFunc: func(req *api.AttachRequest, resp *api.ResponseWithFD) error {
			gotReq = req
			resp.JSON = map[string]any{"attached": true}
			resp.FDs = []int{theirs}
			return nil
		},
	}
	c := newTestClient(t, core)

	var out map[string]any
	conn, err := c.Attach(ctx2s(t), &api.AttachRequest{ClientID: "cli-1", FullCapture: true}, &out)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if conn == nil {
		t.Fatalf("Attach returned nil conn")
	}
	if gotReq == nil || gotReq.ClientID != "cli-1" || !gotReq.FullCapture {
		t.Fatalf("server saw req %+v; want ClientID=cli-1 FullCapture=true", gotReq)
	}
	if v, _ := out["attached"].(bool); !v {
		t.Fatalf("Attach result = %+v; want attached=true", out)
	}

	// Prove the returned conn is the live end of the passed fd: write from our
	// retained side and read it back through the net.Conn.
	if _, errW := unix.Write(ours, []byte("ping")); errW != nil {
		t.Fatalf("write to fd: %v", errW)
	}
	buf := make([]byte, 4)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, errR := io.ReadFull(conn, buf); errR != nil {
		t.Fatalf("read from conn: %v", errR)
	}
	if string(buf) != "ping" {
		t.Fatalf("conn read = %q; want ping", buf)
	}
}

func TestTerminal_Attach_NoFD_IsError(t *testing.T) {
	t.Parallel()
	core := &internalterminal.ControllerTest{
		AttachFunc: func(_ *api.AttachRequest, resp *api.ResponseWithFD) error {
			resp.JSON = map[string]any{"attached": true} // success but no FDs
			return nil
		},
	}
	c := newTestClient(t, core)

	var out map[string]any
	if _, err := c.Attach(ctx2s(t), &api.AttachRequest{ClientID: "cli-1"}, &out); err == nil {
		t.Fatalf("Attach without FD: err = nil; want 'did not send IO fd'")
	}
}

func TestTerminal_Attach_PropagatesError(t *testing.T) {
	t.Parallel()
	core := &internalterminal.ControllerTest{
		AttachFunc: func(_ *api.AttachRequest, _ *api.ResponseWithFD) error {
			return errors.New("attach denied")
		},
	}
	c := newTestClient(t, core)

	var out map[string]any
	if _, err := c.Attach(ctx2s(t), &api.AttachRequest{ClientID: "cli-1"}, &out); err == nil {
		t.Fatalf("Attach err = nil; want propagated error")
	}
}

func TestTerminal_Subscribe_ReturnsConnAndResult(t *testing.T) {
	t.Parallel()
	ours, theirs := fdPair(t)
	t.Cleanup(func() { _ = unix.Close(ours) })

	var gotReq *api.SubscribeRequest
	core := &internalterminal.ControllerTest{
		SubscribeFunc: func(req *api.SubscribeRequest, resp *api.ResponseWithFD) error {
			gotReq = req
			resp.JSON = map[string]any{"subscribed": true}
			resp.FDs = []int{theirs}
			return nil
		},
	}
	c := newTestClient(t, core)

	var out map[string]any
	conn, err := c.Subscribe(ctx2s(t), &api.SubscribeRequest{ClientID: "sub-1"}, &out)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if conn == nil {
		t.Fatalf("Subscribe returned nil conn")
	}
	if gotReq == nil || gotReq.ClientID != "sub-1" {
		t.Fatalf("server saw req %+v; want ClientID=sub-1", gotReq)
	}
	if v, _ := out["subscribed"].(bool); !v {
		t.Fatalf("Subscribe result = %+v; want subscribed=true", out)
	}

	if _, errW := unix.Write(ours, []byte("data")); errW != nil {
		t.Fatalf("write to fd: %v", errW)
	}
	buf := make([]byte, 4)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, errR := io.ReadFull(conn, buf); errR != nil {
		t.Fatalf("read from conn: %v", errR)
	}
	if string(buf) != "data" {
		t.Fatalf("conn read = %q; want data", buf)
	}
}

func TestTerminal_Subscribe_NilReqAndNoFD(t *testing.T) {
	t.Parallel()
	core := &internalterminal.ControllerTest{
		SubscribeFunc: func(_ *api.SubscribeRequest, resp *api.ResponseWithFD) error {
			resp.JSON = map[string]any{"subscribed": true} // no FDs
			return nil
		},
	}
	c := newTestClient(t, core)

	// nil req exercises the defensive substitution; no FD exercises the error path.
	if _, err := c.Subscribe(ctx2s(t), nil, &map[string]any{}); err == nil {
		t.Fatalf("Subscribe without FD: err = nil; want 'did not send IO fd'")
	}
}

func TestTerminal_Close_IsNoop(t *testing.T) {
	t.Parallel()
	c := NewUnix(filepath.Join(t.TempDir(), "x.sock"), testLogger())
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// --- callWithCodec branch coverage via the unexported client (internal pkg) ---

func TestCallWithCodec_DialFailureExhaustsRetries(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("dial boom")
	c := &client{
		logger: testLogger(),
		dial:   func(_ context.Context) (net.Conn, error) { return nil, wantErr },
	}
	ctx := ctx2s(t)
	err := c.call(ctx, c.logger, api.TerminalMethodPing, &api.PingMessage{}, &api.PingMessage{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("call err = %v; want %v after exhausting retries", err, wantErr)
	}
}

func TestCallWithCodec_ContextDoneDuringRetryDelay(t *testing.T) {
	t.Parallel()
	c := &client{
		logger: testLogger(),
		dial:   func(_ context.Context) (net.Conn, error) { return nil, errors.New("dial boom") },
	}
	// 100ms deadline: attempt 0 (delay 0) dials and fails immediately; attempt 1
	// waits 200ms, during which the ctx expires, hitting the ctx.Done() branch
	// inside the retry-delay select.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := c.call(ctx, c.logger, api.TerminalMethodPing, &api.PingMessage{}, &api.PingMessage{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("call err = %v; want context.DeadlineExceeded", err)
	}
}

func TestCallWithCodec_ContextDoneDuringCall(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	core := &internalterminal.ControllerTest{
		PingFunc: func(_ *api.PingMessage) (*api.PingMessage, error) {
			<-release
			return &api.PingMessage{}, nil
		},
	}
	t.Cleanup(func() { close(release) })
	c := newTestClient(t, core)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if err := c.Ping(ctx, &api.PingMessage{Message: "PING"}, &api.PingMessage{}); err == nil {
		t.Fatalf("Ping err = nil; want context error")
	}
}

// TestCall_NonUnixConnBranch drives call() over an in-memory net.Pipe. Because
// a pipe conn is not a *net.UnixConn, this exercises the generic LoggingConn
// branch of call() (the production NewUnix path always yields a *net.UnixConn,
// so that branch is otherwise unreachable). net.Pipe avoids the loopback
// networking the sandbox cannot reliably provide.
func TestCall_NonUnixConnBranch(t *testing.T) {
	t.Parallel()
	serverConn, clientConn := net.Pipe()

	srv := rpc.NewServer()
	core := &internalterminal.ControllerTest{
		PingFunc: func(_ *api.PingMessage) (*api.PingMessage, error) {
			return &api.PingMessage{Message: "PONG"}, nil
		},
	}
	if err := srv.RegisterName(api.TerminalService, &terminalrpc.TerminalControllerRPC{Core: core}); err != nil {
		t.Fatalf("RegisterName: %v", err)
	}
	go srv.ServeCodec(jsonrpc.NewServerCodec(serverConn))
	t.Cleanup(func() { _ = serverConn.Close() })

	var once sync.Once
	c := &client{
		logger: testLogger(),
		dial: func(_ context.Context) (net.Conn, error) {
			var conn net.Conn
			err := errors.New("pipe already consumed")
			once.Do(func() { conn, err = clientConn, nil })
			return conn, err
		},
	}

	out := &api.PingMessage{}
	if err := c.Ping(ctx2s(t), &api.PingMessage{Message: "PING"}, out); err != nil {
		t.Fatalf("Ping over pipe: %v", err)
	}
	if out.Message != "PONG" {
		t.Fatalf("Ping reply = %q; want PONG", out.Message)
	}
}

func TestWithDialTimeout_Applied(t *testing.T) {
	t.Parallel()
	var opts unixOpts
	WithDialTimeout(7 * time.Second)(&opts)
	if opts.DialTimeout != 7*time.Second {
		t.Fatalf("DialTimeout = %v; want 7s", opts.DialTimeout)
	}
}
