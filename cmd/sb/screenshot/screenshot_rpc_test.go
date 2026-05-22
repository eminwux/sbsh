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

package screenshot

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/rpc"
	"path/filepath"
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

func Test_runScreenshot_ANSIRoundtrip(t *testing.T) {
	sock := startTermServer(t, &internalterminal.ControllerTest{
		ScreenshotFunc: func(_ *api.ScreenshotArgs) (*api.ScreenshotResult, error) {
			return &api.ScreenshotResult{Cols: 80, Rows: 24, Text: "plain-grid", ANSI: "ansi-grid"}, nil
		},
	})
	runPath := t.TempDir()
	writeTerminalMetadata(t, runPath, "term1", "alice", sock)

	var out bytes.Buffer
	opts := screenshotOpts{runPath: runPath, name: "alice", plain: false}
	if err := runScreenshot(context.Background(), testLogger(), &out, opts); err != nil {
		t.Fatalf("runScreenshot: %v", err)
	}
	if out.String() != "ansi-grid" {
		t.Errorf("ANSI screenshot = %q, want %q", out.String(), "ansi-grid")
	}
}

func Test_runScreenshot_PlainGrid(t *testing.T) {
	sock := startTermServer(t, &internalterminal.ControllerTest{
		ScreenshotFunc: func(_ *api.ScreenshotArgs) (*api.ScreenshotResult, error) {
			return &api.ScreenshotResult{Text: "plain-grid", ANSI: "ansi-grid"}, nil
		},
	})
	runPath := t.TempDir()
	writeTerminalMetadata(t, runPath, "term1", "alice", sock)

	var out bytes.Buffer
	opts := screenshotOpts{runPath: runPath, name: "alice", plain: true}
	if err := runScreenshot(context.Background(), testLogger(), &out, opts); err != nil {
		t.Fatalf("runScreenshot: %v", err)
	}
	if out.String() != "plain-grid" {
		t.Errorf("plain screenshot = %q, want %q", out.String(), "plain-grid")
	}
}

func Test_runScreenshot_RPCError(t *testing.T) {
	sock := startTermServer(t, &internalterminal.ControllerTest{
		ScreenshotFunc: func(_ *api.ScreenshotArgs) (*api.ScreenshotResult, error) {
			return nil, errors.New("boom")
		},
	})
	runPath := t.TempDir()
	writeTerminalMetadata(t, runPath, "term1", "alice", sock)

	opts := screenshotOpts{runPath: runPath, name: "alice"}
	if err := runScreenshot(context.Background(), testLogger(), &bytes.Buffer{}, opts); err == nil {
		t.Fatal("expected screenshot RPC error, got nil")
	}
}
