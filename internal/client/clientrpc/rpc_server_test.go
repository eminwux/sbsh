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

package clientrpc_test

import (
	"errors"
	"testing"

	"github.com/eminwux/sbsh/internal/client"
	"github.com/eminwux/sbsh/internal/client/clientrpc"
	"github.com/eminwux/sbsh/pkg/api"
)

func TestClientControllerRPC_Ping_Roundtrip(t *testing.T) {
	t.Parallel()
	core := &client.ControllerTest{
		PingFunc: func(in *api.PingMessage) (*api.PingMessage, error) {
			if in == nil || in.Message != "PING" {
				return nil, errors.New("unexpected ping message")
			}
			return &api.PingMessage{Message: "PONG"}, nil
		},
	}
	rpc := &clientrpc.ClientControllerRPC{Core: core}

	out := &api.PingMessage{}
	if err := rpc.Ping(&api.PingMessage{Message: "PING"}, out); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if out.Message != "PONG" {
		t.Fatalf("Ping reply = %q; want %q", out.Message, "PONG")
	}
}

func TestClientControllerRPC_Ping_PropagatesError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("boom")
	core := &client.ControllerTest{
		PingFunc: func(_ *api.PingMessage) (*api.PingMessage, error) { return nil, wantErr },
	}
	rpc := &clientrpc.ClientControllerRPC{Core: core}

	out := &api.PingMessage{}
	if err := rpc.Ping(&api.PingMessage{Message: "PING"}, out); !errors.Is(err, wantErr) {
		t.Fatalf("Ping err = %v; want %v", err, wantErr)
	}
}

func TestClientControllerRPC_Metadata_Roundtrip(t *testing.T) {
	t.Parallel()
	want := &api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata:   api.ClientMetadata{Name: "alpha"},
		Spec:       api.ClientSpec{ID: "c-1"},
		Status:     api.ClientStatus{Pid: 4242, State: api.ClientReady},
	}
	core := &client.ControllerTest{
		MetadataFunc: func() (*api.ClientDoc, error) { return want, nil },
	}
	rpc := &clientrpc.ClientControllerRPC{Core: core}

	out := &api.ClientDoc{}
	if err := rpc.Metadata(api.Empty{}, out); err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if out.Metadata.Name != "alpha" || out.Spec.ID != "c-1" || out.Status.Pid != 4242 ||
		out.Status.State != api.ClientReady {
		t.Fatalf("Metadata reply = %+v; want %+v", out, want)
	}
}

func TestClientControllerRPC_State_Roundtrip(t *testing.T) {
	t.Parallel()
	state := api.ClientAttached
	core := &client.ControllerTest{
		StateFunc: func() (*api.ClientStatusMode, error) { return &state, nil },
	}
	rpc := &clientrpc.ClientControllerRPC{Core: core}

	out := api.ClientInitializing
	if err := rpc.State(api.Empty{}, &out); err != nil {
		t.Fatalf("State: %v", err)
	}
	if out != api.ClientAttached {
		t.Fatalf("State reply = %v; want %v", out, api.ClientAttached)
	}
}

func TestClientControllerRPC_Stop_PassesArgs(t *testing.T) {
	t.Parallel()
	var got *api.StopArgs
	core := &client.ControllerTest{
		StopFunc: func(args *api.StopArgs) error {
			got = args
			return nil
		},
	}
	rpc := &clientrpc.ClientControllerRPC{Core: core}

	if err := rpc.Stop(&api.StopArgs{Reason: "bye"}, &api.Empty{}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got == nil || got.Reason != "bye" {
		t.Fatalf("Stop got = %+v; want Reason=%q", got, "bye")
	}
}
