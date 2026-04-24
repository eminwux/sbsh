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

package client_test

import (
	"context"
	"errors"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"path/filepath"
	"sync"
	"testing"
	"time"

	internalclient "github.com/eminwux/sbsh/internal/client"
	"github.com/eminwux/sbsh/internal/client/clientrpc"
	"github.com/eminwux/sbsh/pkg/api"
	rpcclient "github.com/eminwux/sbsh/pkg/rpcclient/client"
)

// startTestServer wires a real net/rpc JSON server backed by the given
// fake controller, listening on a tempdir Unix socket. The server runs
// until the test ends. Returns the socket path so callers can dial it
// with rpcclient.NewUnix.
func startTestServer(t *testing.T, core *internalclient.ControllerTest) string {
	t.Helper()

	dir := t.TempDir()
	sockPath := filepath.Join(dir, "client.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := rpc.NewServer()
	if err := srv.RegisterName(api.ClientService, &clientrpc.ClientControllerRPC{Core: core}); err != nil {
		_ = ln.Close()
		t.Fatalf("RegisterName: %v", err)
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
			go srv.ServeCodec(jsonrpc.NewServerCodec(conn))
		}
	}()

	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})

	return sockPath
}

func TestClient_Ping_Roundtrip(t *testing.T) {
	t.Parallel()
	var gotIn *api.PingMessage
	core := &internalclient.ControllerTest{
		PingFunc: func(in *api.PingMessage) (*api.PingMessage, error) {
			gotIn = in
			return &api.PingMessage{Message: "PONG"}, nil
		},
	}

	sock := startTestServer(t, core)
	c := rpcclient.NewUnix(sock)
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out := &api.PingMessage{}
	if err := c.Ping(ctx, &api.PingMessage{Message: "PING"}, out); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if out.Message != "PONG" {
		t.Fatalf("Ping reply = %q; want PONG", out.Message)
	}
	if gotIn == nil || gotIn.Message != "PING" {
		t.Fatalf("server saw %+v; want PING", gotIn)
	}
}

func TestClient_Ping_PropagatesError(t *testing.T) {
	t.Parallel()
	core := &internalclient.ControllerTest{
		PingFunc: func(_ *api.PingMessage) (*api.PingMessage, error) {
			return nil, errors.New("nope")
		},
	}
	sock := startTestServer(t, core)
	c := rpcclient.NewUnix(sock)
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Ping(ctx, &api.PingMessage{Message: "PING"}, &api.PingMessage{}); err == nil {
		t.Fatalf("Ping err = nil; want non-nil")
	}
}

func TestClient_Metadata_Roundtrip(t *testing.T) {
	t.Parallel()
	want := &api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata: api.ClientMetadata{
			Name:   "rpc-test",
			Labels: map[string]string{"k": "v"},
		},
		Spec:   api.ClientSpec{ID: "c-rpc", LogFile: "/tmp/log"},
		Status: api.ClientStatus{Pid: 999, State: api.ClientReady},
	}
	core := &internalclient.ControllerTest{
		MetadataFunc: func() (*api.ClientDoc, error) { return want, nil },
	}
	sock := startTestServer(t, core)
	c := rpcclient.NewUnix(sock)
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got := &api.ClientDoc{}
	if err := c.Metadata(ctx, got); err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if got.Metadata.Name != "rpc-test" || got.Spec.ID != "c-rpc" ||
		got.Status.Pid != 999 || got.Status.State != api.ClientReady {
		t.Fatalf("Metadata reply = %+v; want %+v", got, want)
	}
	if got.Metadata.Labels["k"] != "v" {
		t.Fatalf("Metadata labels not preserved: %+v", got.Metadata.Labels)
	}
}

func TestClient_State_Roundtrip(t *testing.T) {
	t.Parallel()
	state := api.ClientAttached
	core := &internalclient.ControllerTest{
		StateFunc: func() (*api.ClientStatusMode, error) { return &state, nil },
	}
	sock := startTestServer(t, core)
	c := rpcclient.NewUnix(sock)
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got := api.ClientInitializing
	if err := c.State(ctx, &got); err != nil {
		t.Fatalf("State: %v", err)
	}
	if got != api.ClientAttached {
		t.Fatalf("State reply = %v; want %v", got, api.ClientAttached)
	}
}

func TestClient_Stop_PassesReason(t *testing.T) {
	t.Parallel()
	var gotArgs *api.StopArgs
	done := make(chan struct{})
	core := &internalclient.ControllerTest{
		StopFunc: func(args *api.StopArgs) error {
			gotArgs = args
			close(done)
			return nil
		},
	}
	sock := startTestServer(t, core)
	c := rpcclient.NewUnix(sock)
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Stop(ctx, &api.StopArgs{Reason: "test"}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("server Stop not invoked")
	}
	if gotArgs == nil || gotArgs.Reason != "test" {
		t.Fatalf("Stop args = %+v; want Reason=%q", gotArgs, "test")
	}
}

func TestClient_Detach_StillWorks(t *testing.T) {
	t.Parallel()
	called := make(chan struct{})
	core := &internalclient.ControllerTest{
		DetachFunc: func() error {
			close(called)
			return nil
		},
	}
	sock := startTestServer(t, core)
	c := rpcclient.NewUnix(sock)
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Detach(ctx); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatalf("server Detach not invoked")
	}
}
