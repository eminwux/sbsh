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

package read

import (
	"bytes"
	"context"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	internalterminal "github.com/eminwux/sbsh/internal/terminal"
	"github.com/eminwux/sbsh/internal/terminal/terminalrpc"
	"github.com/eminwux/sbsh/pkg/api"
	"golang.org/x/sys/unix"
)

// startTermServer wires a real net/rpc server backed by the given fake
// TerminalController on a tempdir Unix socket, mirroring the harness in
// pkg/rpcclient/terminal. Returns the socket path the client should dial.
func startTermServer(t *testing.T, core *internalterminal.ControllerTest) string {
	t.Helper()
	sockPath := filepath.Join(t.TempDir(), "term.sock")
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

// fdPair returns a connected socketpair. ours is retained by the test (its
// write side feeds the live stream); theirs is handed to the server to pass
// back over SCM_RIGHTS as the subscriber's read end. The server codec closes
// the fd it sends, so theirs must not be closed by the test.
func fdPair(t *testing.T) (ours, theirs int) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	return fds[0], fds[1]
}

func Test_runRead_Follow_ReplaysThenStreams(t *testing.T) {
	ours, theirs := fdPair(t)

	sock := startTermServer(t, &internalterminal.ControllerTest{
		SubscribeFunc: func(_ *api.SubscribeRequest, resp *api.ResponseWithFD) error {
			resp.JSON = map[string]any{"subscribed": true}
			resp.FDs = []int{theirs}
			return nil
		},
	})

	runPath := t.TempDir()
	capturePath := filepath.Join(runPath, "capture.log")
	if err := os.WriteFile(capturePath, []byte("REPLAY:"), 0o644); err != nil {
		t.Fatalf("write capture: %v", err)
	}
	writeTerminalMetadata(t, runPath, "term1", "alice", capturePath, sock)

	// Queue live bytes in the socketpair buffer and close our end so the
	// follower reads the replay, then the live tail, then EOF and returns.
	if _, err := unix.Write(ours, []byte("LIVE\n")); err != nil {
		t.Fatalf("seed live bytes: %v", err)
	}
	_ = unix.Close(ours)

	var out, errOut bytes.Buffer
	opts := readOpts{runPath: runPath, name: "alice", follow: true}
	if err := runRead(context.Background(), testLogger(), &out, &errOut, opts); err != nil {
		t.Fatalf("runRead(follow): %v", err)
	}
	if !strings.HasPrefix(out.String(), "REPLAY:") {
		t.Errorf("output missing replay prefix: %q", out.String())
	}
	if !strings.Contains(out.String(), "LIVE\n") {
		t.Errorf("output missing live tail: %q", out.String())
	}
}

func Test_runRead_Follow_NoSocketRecorded(t *testing.T) {
	runPath := t.TempDir()
	capturePath := filepath.Join(runPath, "capture.log")
	if err := os.WriteFile(capturePath, []byte("data"), 0o644); err != nil {
		t.Fatalf("write capture: %v", err)
	}
	writeTerminalMetadata(t, runPath, "term1", "alice", capturePath, "")

	opts := readOpts{runPath: runPath, name: "alice", follow: true}
	err := runRead(context.Background(), testLogger(), &bytes.Buffer{}, &bytes.Buffer{}, opts)
	if err == nil {
		t.Fatal("expected error for missing socket in follow mode, got nil")
	}
}
