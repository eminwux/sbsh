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

package supervisorrunner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

// mockTerminalClient is a mock implementation of terminal.Client for testing.
type mockTerminalClient struct {
	stateFunc func(ctx context.Context, state *api.TerminalStatusMode) error
	closeFunc func() error
}

func (m *mockTerminalClient) Ping(_ context.Context, _ *api.PingMessage, _ *api.PingMessage) error {
	return nil
}

func (m *mockTerminalClient) Resize(_ context.Context, _ *api.ResizeArgs) error {
	return nil
}

func (m *mockTerminalClient) Detach(_ context.Context, _ *api.ID) error {
	return nil
}

func (m *mockTerminalClient) Attach(_ context.Context, _ *api.ID, _ any) (net.Conn, error) {
	return nil, errors.New("not implemented in mock")
}

func (m *mockTerminalClient) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func (m *mockTerminalClient) Metadata(_ context.Context, _ *api.TerminalDoc) error {
	return nil
}

func (m *mockTerminalClient) State(ctx context.Context, state *api.TerminalStatusMode) error {
	if m.stateFunc != nil {
		return m.stateFunc(ctx, state)
	}
	return errors.New("stateFunc not set")
}

func Test_WaitReady_WithMultipleStates(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sr := &Exec{
		ctx:    ctx,
		logger: logger,
		terminal: &api.SupervisedTerminal{
			Spec: &api.TerminalSpec{
				ID:   api.ID("test-terminal"),
				Name: "test",
			},
		},
		metadata: api.SupervisorDoc{
			APIVersion: api.APIVersionV1Beta1,
			Kind:       api.KindSupervisor,
			Metadata: api.SupervisorMetadata{
				Name:        "test-supervisor",
				Labels:      make(map[string]string),
				Annotations: make(map[string]string),
			},
			Spec: api.SupervisorSpec{
				Name: "test-supervisor",
			},
		},
	}

	// Test with Starting state - should succeed
	currentState := api.Starting
	sr.terminalClient = &mockTerminalClient{
		stateFunc: func(_ context.Context, state *api.TerminalStatusMode) error {
			*state = currentState
			return nil
		},
	}

	err := sr.waitReady(api.Starting, api.Ready)
	if err != nil {
		t.Fatalf("waitReady(Starting, Ready) with Starting state should succeed, got error: %v", err)
	}

	// Test with Ready state - should succeed
	currentState = api.Ready
	err = sr.waitReady(api.Starting, api.Ready)
	if err != nil {
		t.Fatalf("waitReady(Starting, Ready) with Ready state should succeed, got error: %v", err)
	}

	// Test with Initializing state - should timeout
	currentState = api.Initializing
	done := make(chan error, 1)
	go func() {
		done <- sr.waitReady(api.Starting, api.Ready)
	}()

	select {
	case errWait := <-done:
		if errWait == nil {
			t.Fatal("waitReady(Starting, Ready) with Initializing state should timeout or fail")
		}
		if !errors.Is(errWait, context.DeadlineExceeded) {
			// Check if error message contains timeout context
			errStr := fmt.Sprintf("%v", errWait)
			if errStr != "context deadline exceeded" &&
				errStr != "context done before ready: context deadline exceeded" {
				t.Fatalf("Expected timeout error, got: %v", errWait)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("waitReady should have timed out within 3 seconds")
	}
}

func Test_WaitReady_StartingState(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sr := &Exec{
		ctx:    ctx,
		logger: logger,
		terminal: &api.SupervisedTerminal{
			Spec: &api.TerminalSpec{
				ID:   api.ID("test-terminal"),
				Name: "test",
			},
		},
		metadata: api.SupervisorDoc{
			APIVersion: api.APIVersionV1Beta1,
			Kind:       api.KindSupervisor,
			Metadata: api.SupervisorMetadata{
				Name:        "test-supervisor",
				Labels:      make(map[string]string),
				Annotations: make(map[string]string),
			},
			Spec: api.SupervisorSpec{
				Name: "test-supervisor",
			},
		},
	}

	sr.terminalClient = &mockTerminalClient{
		stateFunc: func(_ context.Context, state *api.TerminalStatusMode) error {
			*state = api.Starting
			return nil
		},
	}

	err := sr.waitReady(api.Starting)
	if err != nil {
		t.Fatalf("waitReady(Starting) should succeed when terminal is in Starting state, got error: %v", err)
	}
}

func Test_WaitReady_StartingOrReady(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sr := &Exec{
		ctx:    ctx,
		logger: logger,
		terminal: &api.SupervisedTerminal{
			Spec: &api.TerminalSpec{
				ID:   api.ID("test-terminal"),
				Name: "test",
			},
		},
		metadata: api.SupervisorDoc{
			APIVersion: api.APIVersionV1Beta1,
			Kind:       api.KindSupervisor,
			Metadata: api.SupervisorMetadata{
				Name:        "test-supervisor",
				Labels:      make(map[string]string),
				Annotations: make(map[string]string),
			},
			Spec: api.SupervisorSpec{
				Name: "test-supervisor",
			},
		},
	}

	// Test with Starting state
	sr.terminalClient = &mockTerminalClient{
		stateFunc: func(_ context.Context, state *api.TerminalStatusMode) error {
			*state = api.Starting
			return nil
		},
	}

	err := sr.waitReady(api.Starting, api.Ready)
	if err != nil {
		t.Fatalf("waitReady(Starting, Ready) with Starting state should succeed, got error: %v", err)
	}

	// Test with Ready state
	sr.terminalClient = &mockTerminalClient{
		stateFunc: func(_ context.Context, state *api.TerminalStatusMode) error {
			*state = api.Ready
			return nil
		},
	}

	err = sr.waitReady(api.Starting, api.Ready)
	if err != nil {
		t.Fatalf("waitReady(Starting, Ready) with Ready state should succeed, got error: %v", err)
	}
}

func Test_WaitReady_Timeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sr := &Exec{
		ctx:    ctx,
		logger: logger,
		terminal: &api.SupervisedTerminal{
			Spec: &api.TerminalSpec{
				ID:   api.ID("test-terminal"),
				Name: "test",
			},
		},
		metadata: api.SupervisorDoc{
			APIVersion: api.APIVersionV1Beta1,
			Kind:       api.KindSupervisor,
			Metadata: api.SupervisorMetadata{
				Name:        "test-supervisor",
				Labels:      make(map[string]string),
				Annotations: make(map[string]string),
			},
			Spec: api.SupervisorSpec{
				Name: "test-supervisor",
			},
		},
	}

	// Terminal state never reaches the expected states
	sr.terminalClient = &mockTerminalClient{
		stateFunc: func(_ context.Context, state *api.TerminalStatusMode) error {
			*state = api.Initializing // Terminal stuck in Initializing
			return nil
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- sr.waitReady(api.Starting, api.Ready)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("waitReady should have timed out")
		}
		// Should get a timeout error
		if !errors.Is(err, context.DeadlineExceeded) {
			// Check error message for timeout
			errStr := fmt.Sprintf("%v", err)
			if errStr != "context deadline exceeded" &&
				errStr != "context done before ready: context deadline exceeded" {
				t.Fatalf("Expected timeout error, got: %v", err)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("waitReady should have timed out within 3 seconds")
	}
}

func Test_WaitReady_StateTransition(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sr := &Exec{
		ctx:    ctx,
		logger: logger,
		terminal: &api.SupervisedTerminal{
			Spec: &api.TerminalSpec{
				ID:   api.ID("test-terminal"),
				Name: "test",
			},
		},
		metadata: api.SupervisorDoc{
			APIVersion: api.APIVersionV1Beta1,
			Kind:       api.KindSupervisor,
			Metadata: api.SupervisorMetadata{
				Name:        "test-supervisor",
				Labels:      make(map[string]string),
				Annotations: make(map[string]string),
			},
			Spec: api.SupervisorSpec{
				Name: "test-supervisor",
			},
		},
	}

	// Simulate state transition: Initializing -> Starting
	stateSequence := []api.TerminalStatusMode{api.Initializing, api.Starting}
	callCount := 0

	sr.terminalClient = &mockTerminalClient{
		stateFunc: func(_ context.Context, state *api.TerminalStatusMode) error {
			if callCount < len(stateSequence) {
				*state = stateSequence[callCount]
				callCount++
				// Add a small delay to simulate real RPC call
				time.Sleep(10 * time.Millisecond)
			} else {
				*state = api.Starting
			}
			return nil
		},
	}

	err := sr.waitReady(api.Starting, api.Ready)
	if err != nil {
		t.Fatalf("waitReady should succeed when state transitions to Starting, got error: %v", err)
	}
	if callCount < 2 {
		t.Errorf("Expected at least 2 state checks, got %d", callCount)
	}
}
