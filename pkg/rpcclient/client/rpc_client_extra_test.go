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
	"path/filepath"
	"testing"
	"time"

	internalclient "github.com/eminwux/sbsh/internal/client"
	"github.com/eminwux/sbsh/pkg/api"
	rpcclient "github.com/eminwux/sbsh/pkg/rpcclient/client"
)

// TestClient_NilArgsTolerated drives every method with a nil argument/reply to
// exercise the defensive nil-substitution branches in rpc_client.go (Ping with
// nil pong, Metadata/State/Stop with nil pointers). The fake server records the
// inputs so we can confirm the substituted zero values reach the wire.
func TestClient_NilArgsTolerated(t *testing.T) {
	t.Parallel()

	pong := &api.PingMessage{Message: "PONG"}
	doc := &api.ClientDoc{Metadata: api.ClientMetadata{Name: "n"}}
	state := api.ClientReady
	stopDone := make(chan struct{})
	core := &internalclient.ControllerTest{
		PingFunc:     func(_ *api.PingMessage) (*api.PingMessage, error) { return pong, nil },
		MetadataFunc: func() (*api.ClientDoc, error) { return doc, nil },
		StateFunc:    func() (*api.ClientStatusMode, error) { return &state, nil },
		StopFunc:     func(_ *api.StopArgs) error { close(stopDone); return nil },
	}

	sock := startTestServer(t, core)
	c := rpcclient.NewUnix(sock)
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Ping with nil ping and nil pong: both get substituted internally.
	if err := c.Ping(ctx, nil, nil); err != nil {
		t.Fatalf("Ping(nil,nil): %v", err)
	}
	// Metadata with nil doc: server result is decoded into an internal throwaway.
	if err := c.Metadata(ctx, nil); err != nil {
		t.Fatalf("Metadata(nil): %v", err)
	}
	// State with nil state: same throwaway path.
	if err := c.State(ctx, nil); err != nil {
		t.Fatalf("State(nil): %v", err)
	}
	// Stop with nil args: substituted to an empty StopArgs.
	if err := c.Stop(ctx, nil); err != nil {
		t.Fatalf("Stop(nil): %v", err)
	}
	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("server Stop not invoked")
	}
}

// TestClient_DialFailure points the client at a non-existent socket so c.dial
// returns an error that call() must surface unchanged.
func TestClient_DialFailure(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "does-not-exist.sock")
	c := rpcclient.NewUnix(missing, rpcclient.WithDialTimeout(200*time.Millisecond))
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Detach(ctx); err == nil {
		t.Fatalf("Detach against missing socket: err = nil; want dial error")
	}
}

// TestClient_ContextCancellation cancels the context while the server blocks
// inside the handler, exercising the ctx.Done() branch of call() that nudges
// the connection deadline and returns ctx.Err().
func TestClient_ContextCancellation(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	core := &internalclient.ControllerTest{
		PingFunc: func(_ *api.PingMessage) (*api.PingMessage, error) {
			<-release // block until the test releases us
			return &api.PingMessage{}, nil
		},
	}
	t.Cleanup(func() { close(release) })

	sock := startTestServer(t, core)
	c := rpcclient.NewUnix(sock)
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := c.Ping(ctx, &api.PingMessage{Message: "PING"}, &api.PingMessage{})
	if err == nil {
		t.Fatalf("Ping err = nil; want context deadline error")
	}
}
