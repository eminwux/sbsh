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

package write

import (
	"bytes"
	"context"
	"net"
	"net/rpc"
	"sync"
	"testing"

	internalterminal "github.com/eminwux/sbsh/internal/terminal"
	"github.com/eminwux/sbsh/internal/terminal/terminalrpc"
	"github.com/eminwux/sbsh/pkg/api"
)

// startTermServer wires a real net/rpc server backed by the given fake
// TerminalController on a tempdir Unix socket, mirroring the harness in
// pkg/rpcclient/terminal. Returns the socket path the client should dial.
func startTermServer(t *testing.T, core *internalterminal.ControllerTest) string {
	t.Helper()
	sockPath := t.TempDir() + "/term.sock"
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := rpc.NewServer()
	if errReg := srv.RegisterName(api.TerminalService, &terminalrpc.TerminalControllerRPC{Core: core}); errReg != nil {
		_ = ln.Close()
		t.Fatalf("RegisterName: %v", errReg)
	}
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
			go srv.ServeCodec(terminalrpc.NewUnixJSONServerCodec(uconn, testLogger()))
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
	return sockPath
}

func Test_RunWrite_RPCSuccess(t *testing.T) {
	var got []byte
	sock := startTermServer(t, &internalterminal.ControllerTest{
		WriteFunc: func(req *api.WriteRequest) error {
			got = append(got, req.Data...)
			return nil
		},
	})

	runPath := t.TempDir()
	writeTerminalMetadata(t, runPath, "term1", "alice", sock)
	opts := writeOpts{runPath: runPath, name: "alice", payload: []byte("hello")}

	if err := runWrite(context.Background(), testLogger(), opts); err != nil {
		t.Fatalf("runWrite: %v", err)
	}
	if !bytes.Equal(got, []byte("hello")) {
		t.Errorf("server received %q, want %q", got, "hello")
	}
}
