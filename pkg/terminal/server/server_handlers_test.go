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
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
	rpcclient "github.com/eminwux/sbsh/pkg/rpcclient/terminal"
	"github.com/eminwux/sbsh/pkg/terminal/server"
)

// SetupStatusArgs / SetupStatusReply are the request/response shapes for
// the dummy custom verb. net/rpc requires the arg and reply types to be
// exported, so these live at package scope with exported fields.
type SetupStatusArgs struct {
	Want string `json:"Want"`
}

type SetupStatusReply struct {
	Status string `json:"Status"`
	Echo   string `json:"Echo"`
}

// setupHandler is a stand-in for kuketty's container-setup-status verb:
// a custom JSON-RPC service registered alongside the built-in protocol.
type setupHandler struct{}

func (setupHandler) GetSetupStatus(args SetupStatusArgs, reply *SetupStatusReply) error {
	reply.Status = "ready"
	reply.Echo = args.Want
	return nil
}

// TestServer_CustomHandler_DispatchAndIsolation registers a custom verb
// via WithHandlers and asserts (1) it dispatches over the same listener
// as the built-in protocol and (2) the built-in attach/control protocol
// (Ping) is unaffected on the same socket — the AC's dispatch + isolation
// requirement.
func TestServer_CustomHandler_DispatchAndIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tmp := t.TempDir()
	socketPath := filepath.Join(tmp, "ctrl.sock")

	listener, listenErr := net.Listen("unix", socketPath)
	if listenErr != nil {
		t.Fatalf("net.Listen: %v", listenErr)
	}

	spec := &api.TerminalSpec{
		ID:            api.ID("test-custom-handler"),
		Name:          "test-custom-handler",
		Labels:        map[string]string{},
		Command:       "/bin/sh",
		CommandArgs:   []string{},
		EnvInherit:    true,
		RunPath:       tmp,
		SocketFile:    socketPath,
		CaptureFile:   filepath.Join(tmp, "capture.log"),
		ShutdownGrace: 500 * time.Millisecond,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, newErr := server.New(spec, logger,
		server.WithHandlers(server.Handler{Name: "SetupStatus", Receiver: setupHandler{}}),
	)
	if newErr != nil {
		t.Fatalf("server.New: %v", newErr)
	}

	serveErrCh := make(chan error, 1)
	go func() { serveErrCh <- srv.Serve(ctx, listener) }()
	defer func() {
		_ = srv.Stop(errors.New("test cleanup"))
		select {
		case <-serveErrCh:
		case <-time.After(5 * time.Second):
			t.Logf("Serve did not return within 5s of cleanup Stop")
		}
	}()

	if waitErr := waitReady(ctx, srv, 10*time.Second); waitErr != nil {
		t.Fatalf("waitReady: %v", waitErr)
	}

	// Custom verb dispatches on the control socket.
	var reply SetupStatusReply
	if err := rawCall(t, socketPath, "SetupStatus.GetSetupStatus",
		SetupStatusArgs{Want: "phase1"}, &reply); err != nil {
		t.Fatalf("custom verb call: %v", err)
	}
	if reply.Status != "ready" || reply.Echo != "phase1" {
		t.Fatalf("custom verb reply = %+v, want {ready phase1}", reply)
	}

	// Built-in protocol is unaffected on the same socket.
	client := rpcclient.NewUnix(socketPath, logger)
	defer client.Close()
	var pong api.PingMessage
	if err := client.Ping(ctx, &api.PingMessage{Message: "PING"}, &pong); err != nil {
		t.Fatalf("built-in Ping after custom registration: %v", err)
	}
	if pong.Message != "PONG" {
		t.Fatalf("Ping reply = %q, want PONG", pong.Message)
	}

	// An unknown method on the custom service still surfaces an error
	// rather than corrupting the built-in dispatch.
	var discard SetupStatusReply
	if err := rawCall(t, socketPath, "SetupStatus.Nonexistent",
		SetupStatusArgs{}, &discard); err == nil {
		t.Fatal("unknown custom method returned no error")
	}
}

// TestServer_New_RejectsBadHandlers locks in the registration-time guards
// from WithHandlers/New: a Name colliding with the built-in service, an
// empty Name, a nil Receiver, and a duplicate Name are all rejected
// before Serve.
func TestServer_New_RejectsBadHandlers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	spec := &api.TerminalSpec{Command: "/bin/sh"}

	cases := []struct {
		name     string
		handlers []server.Handler
	}{
		{"collides with built-in", []server.Handler{{Name: api.TerminalService, Receiver: setupHandler{}}}},
		{"empty name", []server.Handler{{Name: "", Receiver: setupHandler{}}}},
		{"nil receiver", []server.Handler{{Name: "SetupStatus", Receiver: nil}}},
		{"duplicate name", []server.Handler{
			{Name: "SetupStatus", Receiver: setupHandler{}},
			{Name: "SetupStatus", Receiver: setupHandler{}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := server.New(spec, logger, server.WithHandlers(tc.handlers...)); err == nil {
				t.Fatalf("New with %s handler returned no error", tc.name)
			}
		})
	}
}

// rawCall issues a single JSON-RPC request over a fresh connection to the
// control socket and decodes the reply. It speaks the same wire shape the
// built-in codec uses ({id, method, params:[arg]} → {id, result, error})
// so the test exercises real on-socket dispatch without depending on the
// typed client, which only knows the built-in methods.
func rawCall(t *testing.T, socketPath, method string, arg, reply any) error {
	t.Helper()

	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	if derr := conn.SetDeadline(time.Now().Add(5 * time.Second)); derr != nil {
		return derr
	}

	req := struct {
		ID     uint64 `json:"id"`
		Method string `json:"method"`
		Params [1]any `json:"params"`
	}{ID: 1, Method: method, Params: [1]any{arg}}
	if encErr := json.NewEncoder(conn).Encode(&req); encErr != nil {
		return encErr
	}

	var resp struct {
		ID     uint64           `json:"id"`
		Result *json.RawMessage `json:"result"`
		Error  *string          `json:"error"`
	}
	if decErr := json.NewDecoder(conn).Decode(&resp); decErr != nil {
		return decErr
	}
	if resp.Error != nil {
		return errors.New(*resp.Error)
	}
	if resp.Result == nil || reply == nil {
		return nil
	}
	return json.Unmarshal(*resp.Result, reply)
}
