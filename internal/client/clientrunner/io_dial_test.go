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
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/pkg/api"
)

// fakeTerminalCtrl is a minimal jsonrpc service that answers the terminal
// control Ping the client runner issues over the terminal's control socket.
type fakeTerminalCtrl struct{}

func (fakeTerminalCtrl) Ping(_ *api.PingMessage, out *api.PingMessage) error {
	*out = api.PingMessage{Message: "PONG"}
	return nil
}

// serveFakeTerminalCtrl binds a unix socket and serves a jsonrpc terminal
// control endpoint exposing Ping. It returns the socket path.
func serveFakeTerminalCtrl(t *testing.T) string {
	t.Helper()
	sockPath := filepath.Join(t.TempDir(), "term-ctrl.sock")
	ln, errListen := net.Listen("unix", sockPath)
	if errListen != nil {
		t.Fatalf("listen terminal ctrl socket: %v", errListen)
	}
	t.Cleanup(func() { _ = ln.Close() })

	srv := rpc.NewServer()
	if errReg := srv.RegisterName(api.TerminalService, &fakeTerminalCtrl{}); errReg != nil {
		t.Fatalf("register fake terminal ctrl: %v", errReg)
	}
	go func() {
		for {
			conn, errAccept := ln.Accept()
			if errAccept != nil {
				return
			}
			go srv.ServeCodec(jsonrpc.NewServerCodec(conn))
		}
	}()
	return sockPath
}

// TestDialTerminalCtrlSocket_Success exercises the real connection path: the
// runner dials the terminal control socket and pings it successfully.
func TestDialTerminalCtrlSocket_Success(t *testing.T) {
	sockPath := serveFakeTerminalCtrl(t)

	sr := newIOExec(t)
	sr.terminal = &api.AttachedTerminal{
		Spec: &api.TerminalSpec{ID: api.ID("term-io"), SocketFile: sockPath},
	}

	if err := sr.dialTerminalCtrlSocket(); err != nil {
		t.Fatalf("dialTerminalCtrlSocket: %v", err)
	}
}

// TestDialTerminalCtrlSocket_PingFailure covers the error branch when no peer
// is serving the terminal control socket.
func TestDialTerminalCtrlSocket_PingFailure(t *testing.T) {
	sr := newIOExec(t)
	sr.terminal = &api.AttachedTerminal{
		Spec: &api.TerminalSpec{
			ID:         api.ID("term-io"),
			SocketFile: filepath.Join(t.TempDir(), "does-not-exist.sock"),
		},
	}

	if err := sr.dialTerminalCtrlSocket(); err == nil {
		t.Fatal("dialTerminalCtrlSocket returned nil error with no peer serving")
	}
}
