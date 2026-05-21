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
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

func newIOExec(t *testing.T) *Exec {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Exec{
		id:         api.ID("client-io"),
		ctx:        ctx,
		ctxCancel:  cancel,
		logger:     testLogger(),
		events:     make(chan Event, 8),
		metadataMu: sync.RWMutex{},
		metadata: api.ClientDoc{
			APIVersion: api.APIVersionV1Beta1,
			Kind:       api.KindClient,
			Spec:       api.ClientSpec{ID: api.ID("client-io"), RunPath: t.TempDir()},
		},
		terminal: &api.AttachedTerminal{Spec: &api.TerminalSpec{ID: api.ID("term-io")}},
	}
}

// TestAttach_Success drives the attach() helper: the terminal client returns a
// connection, the state advances to ClientAttached, and ioConn is stored.
func TestAttach_Success(t *testing.T) {
	sr := newIOExec(t)
	// attach() persists metadata, which requires the per-client run dir that
	// CreateMetadata establishes during normal startup.
	if err := sr.CreateMetadata(); err != nil {
		t.Fatalf("CreateMetadata: %v", err)
	}
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })

	var gotReq *api.AttachRequest
	sr.terminalClient = &mockTerminalClient{
		attachFunc: func(_ context.Context, req *api.AttachRequest, _ any) (net.Conn, error) {
			gotReq = req
			return clientConn, nil
		},
	}
	sr.metadata.Spec.FullCapture = true

	if err := sr.attach(); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if gotReq == nil || gotReq.ClientID != sr.id || !gotReq.FullCapture {
		t.Fatalf("attach request = %+v; want ClientID=%q FullCapture=true", gotReq, sr.id)
	}
	if sr.ioConn != clientConn {
		t.Fatal("attach did not store the returned connection in ioConn")
	}
	if st, _ := sr.State(); *st != api.ClientAttached {
		t.Errorf("state = %v; want ClientAttached", *st)
	}
}

// TestAttach_Error covers the branch where the terminal client fails to attach.
func TestAttach_Error(t *testing.T) {
	sr := newIOExec(t)
	sr.terminalClient = &mockTerminalClient{
		attachFunc: func(_ context.Context, _ *api.AttachRequest, _ any) (net.Conn, error) {
			return nil, errors.New("attach refused")
		},
	}
	if err := sr.attach(); err == nil {
		t.Fatal("attach returned nil error when terminal client failed")
	}
}

// TestForwardResize_InitialResize verifies the initial resize is forwarded for
// a real tty stdin, then tears down the WINCH watcher via context cancel.
func TestForwardResize_InitialResize(t *testing.T) {
	sr := newIOExec(t)
	sr.stdin = openTTY(t)

	resized := make(chan *api.ResizeArgs, 1)
	sr.terminalClient = &mockTerminalClient{
		resizeFunc: func(_ context.Context, args *api.ResizeArgs) error {
			select {
			case resized <- args:
			default:
			}
			return nil
		},
	}

	if err := sr.forwardResize(); err != nil {
		t.Fatalf("forwardResize: %v", err)
	}
	select {
	case <-resized:
	case <-time.After(2 * time.Second):
		t.Fatal("initial resize was not forwarded")
	}
	sr.ctxCancel() // stop the WINCH watcher goroutine
}

// TestForwardResize_InitialError covers the branch where the initial resize
// RPC fails.
func TestForwardResize_InitialError(t *testing.T) {
	sr := newIOExec(t)
	sr.stdin = openTTY(t)
	sr.terminalClient = &mockTerminalClient{
		resizeFunc: func(_ context.Context, _ *api.ResizeArgs) error { return errors.New("resize fail") },
	}
	if err := sr.forwardResize(); err == nil {
		t.Fatal("forwardResize returned nil error when initial resize failed")
	}
}

// TestStartConnectionManager wires a real unix socketpair as ioConn and pumps
// the dual-copier loop, exercising both the detach-keystroke-enabled help
// banner and the manager teardown on context cancel.
func TestStartConnectionManager(t *testing.T) {
	sr := newIOExec(t)
	sr.stdin = openTTY(t)

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	t.Cleanup(func() { _ = devNull.Close() })
	sr.stdout = devNull
	sr.metadata.Spec.DetachKeystroke = true

	client, _ := unixSocketPair(t)
	sr.ioConn = client

	if errStart := sr.startConnectionManager(); errStart != nil {
		t.Fatalf("startConnectionManager: %v", errStart)
	}
	// Let the copier goroutines settle, then tear down via context cancel.
	time.Sleep(50 * time.Millisecond)
	sr.ctxCancel()
}

// TestStartConnectionManager_NonUnixConn covers the guard that rejects an
// ioConn that is not a *net.UnixConn.
func TestStartConnectionManager_NonUnixConn(t *testing.T) {
	sr := newIOExec(t)
	sr.stdin = openTTY(t)
	c1, c2 := net.Pipe()
	t.Cleanup(func() { _ = c1.Close(); _ = c2.Close() })
	sr.ioConn = c1 // net.Pipe conn is not a *net.UnixConn

	if err := sr.startConnectionManager(); err == nil {
		t.Fatal("startConnectionManager accepted a non-UnixConn ioConn")
	}
}
