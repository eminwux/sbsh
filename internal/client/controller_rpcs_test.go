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

package client

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/client/clientrunner"
	"github.com/eminwux/sbsh/pkg/api"
)

func newControllerForRPCTest(t *testing.T, runner *clientrunner.Test) *Controller {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	c := NewClientController(context.Background(), logger).(*Controller)
	if runner != nil {
		c.sr = runner
	}
	return c
}

func TestController_Ping_Pong(t *testing.T) {
	t.Parallel()
	c := newControllerForRPCTest(t, nil)
	out, err := c.Ping(&api.PingMessage{Message: "PING"})
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if out.Message != "PONG" {
		t.Fatalf("Ping reply = %q; want PONG", out.Message)
	}
}

func TestController_Ping_RejectsUnknownMessage(t *testing.T) {
	t.Parallel()
	c := newControllerForRPCTest(t, nil)
	if _, err := c.Ping(&api.PingMessage{Message: "huh"}); err == nil {
		t.Fatalf("Ping err = nil; want non-nil for unknown message")
	}
}

func TestController_Metadata_DelegatesToRunner(t *testing.T) {
	t.Parallel()
	want := &api.ClientDoc{
		Metadata: api.ClientMetadata{Name: "delegate"},
		Spec:     api.ClientSpec{ID: "c-9"},
		Status:   api.ClientStatus{State: api.ClientReady, Pid: 7},
	}
	runner := &clientrunner.Test{
		MetadataFunc: func() (*api.ClientDoc, error) { return want, nil },
	}
	c := newControllerForRPCTest(t, runner)
	got, err := c.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if got != want {
		t.Fatalf("Metadata = %p; want %p", got, want)
	}
}

func TestController_Metadata_NoRunner(t *testing.T) {
	t.Parallel()
	c := newControllerForRPCTest(t, nil)
	if _, err := c.Metadata(); err == nil {
		t.Fatalf("Metadata err = nil; want runner-not-init error")
	}
}

func TestController_State_DelegatesToRunner(t *testing.T) {
	t.Parallel()
	state := api.ClientAttached
	runner := &clientrunner.Test{
		StateFunc: func() (*api.ClientStatusMode, error) { return &state, nil },
	}
	c := newControllerForRPCTest(t, runner)
	got, err := c.State()
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if *got != api.ClientAttached {
		t.Fatalf("State = %v; want %v", *got, api.ClientAttached)
	}
}

func TestController_State_NoRunner(t *testing.T) {
	t.Parallel()
	c := newControllerForRPCTest(t, nil)
	if _, err := c.State(); err == nil {
		t.Fatalf("State err = nil; want runner-not-init error")
	}
}

func TestController_Stop_TriggersAsyncClose(t *testing.T) {
	t.Parallel()
	closed := make(chan error, 1)
	runner := &clientrunner.Test{
		CloseFunc: func(reason error) error {
			select {
			case closed <- reason:
			default:
			}
			return nil
		},
	}
	c := newControllerForRPCTest(t, runner)

	if err := c.Stop(&api.StopArgs{Reason: "stop-reason"}); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	select {
	case reason := <-closed:
		if reason == nil || !strings.Contains(reason.Error(), "stop-reason") {
			t.Fatalf("Close reason = %v; want to contain %q", reason, "stop-reason")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Close was not triggered by Stop")
	}
}

func TestController_Stop_NilArgsUsesDefaultReason(t *testing.T) {
	t.Parallel()
	closed := make(chan error, 1)
	runner := &clientrunner.Test{
		CloseFunc: func(reason error) error {
			select {
			case closed <- reason:
			default:
			}
			return nil
		},
	}
	c := newControllerForRPCTest(t, runner)

	if err := c.Stop(nil); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case reason := <-closed:
		if reason == nil {
			t.Fatalf("expected non-nil close reason on Stop(nil)")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Close was not triggered by Stop(nil)")
	}
}

