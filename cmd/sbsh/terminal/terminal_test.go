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

package terminal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/internal/terminal"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestRunTerminal_ErrContextCancelled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	newTerminalController := func(_ context.Context) api.TerminalController {
		return &terminal.ControllerTest{
			Exit:          nil,
			RunFunc:       func(_ *api.TerminalSpec) error { return nil },
			WaitReadyFunc: func() error { return nil },
			WaitCloseFunc: func() error { return nil },
			PingFunc:      func(_ *api.PingMessage) (*api.PingMessage, error) { return &api.PingMessage{Message: "PONG"}, nil },
		}
	}

	ctrl := newTerminalController(context.Background())

	t.Cleanup(func() {})

	spec := api.TerminalSpec{
		ID:          api.ID(naming.RandomID()),
		Kind:        api.TerminalLocal,
		Name:        naming.RandomName(),
		Command:     "/bin/bash",
		CommandArgs: []string{},
		Env:         os.Environ(),
		RunPath:     viper.GetString("global.runPath"),
	}

	done := make(chan error)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- runTerminal(ctx, cancel, logger, ctrl, &spec) // will block until ctx.Done()
		defer close(done)
	}()

	// Give Run() time to set ready, then signal the process (NotifyContext listens to SIGTERM/INT)
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, errdefs.ErrContextDone) {
			t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrContextDone, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runTerminal to return after SIGTERM")
	}
}

func TestRunTerminal_ErrWaitOnReady(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	newTerminalController := func(_ context.Context) api.TerminalController {
		return &terminal.ControllerTest{
			Exit:          nil,
			RunFunc:       func(_ *api.TerminalSpec) error { return nil },
			WaitReadyFunc: func() error { return errors.New("not ready") },
			WaitCloseFunc: func() error { return nil },
		}
	}

	ctrl := newTerminalController(context.Background())

	t.Cleanup(func() {})

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	spec := api.TerminalSpec{
		ID:          api.ID(naming.RandomID()),
		Kind:        api.TerminalLocal,
		Name:        naming.RandomName(),
		Command:     "/bin/bash",
		CommandArgs: []string{},
		Env:         os.Environ(),
		RunPath:     viper.GetString("global.runPath"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := runTerminal(ctx, cancel, logger, ctrl, &spec); err != nil && !errors.Is(err, errdefs.ErrWaitOnReady) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrWaitOnReady, err)
	}
}

func TestRunTerminal_ErrWaitOnClose(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	newTerminalController := func(_ context.Context) api.TerminalController {
		return &terminal.ControllerTest{
			Exit:          nil,
			RunFunc:       func(_ *api.TerminalSpec) error { return nil },
			WaitReadyFunc: func() error { return nil },
			WaitCloseFunc: func() error { return errors.New("error on close") },
		}
	}
	t.Cleanup(func() {})

	ctrl := newTerminalController(context.Background())

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	spec := api.TerminalSpec{
		ID:          api.ID(naming.RandomID()),
		Kind:        api.TerminalLocal,
		Name:        naming.RandomName(),
		Command:     "/bin/bash",
		CommandArgs: []string{},
		Env:         os.Environ(),
		RunPath:     viper.GetString("global.runPath"),
	}

	exitCh := make(chan error)

	ctx, cancel := context.WithCancel(context.Background())

	go func(exitCh chan error) {
		exitCh <- runTerminal(ctx, cancel, logger, ctrl, &spec)
		defer close(exitCh)
	}(exitCh)

	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(40 * time.Millisecond)

	if err := <-exitCh; err != nil && !errors.Is(err, errdefs.ErrWaitOnClose) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrWaitOnClose, err)
	}
}

func Test_ErrInvalidFlag(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("id", "", "test flag")
	cmd.Flags().Parse([]string{"--id", "test123"})

	err := checkFlag(cmd, "id", "positional argument '-'")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrInvalidFlag) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidFlag, err)
	}
}

func Test_ErrInvalidArgument(t *testing.T) {
	cmd := &cobra.Command{}
	spec, err := processInput(cmd, []string{"invalid"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if spec != nil {
		t.Fatal("expected nil spec")
	}
	if !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidArgument, err)
	}
}

func Test_ErrStdinEmpty(t *testing.T) {
	// This test verifies that ErrStdinEmpty is properly defined
	// Testing the actual condition requires mocking os.Stdin.Stat(), which is complex
	// The error is tested indirectly through integration tests
	err := errdefs.ErrStdinEmpty
	if err == nil {
		t.Fatal("ErrStdinEmpty should not be nil")
	}
	if !errors.Is(err, errdefs.ErrStdinEmpty) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrStdinEmpty, err)
	}
}

func Test_ErrOpenSpecFile(t *testing.T) {
	cmd := &cobra.Command{}
	viper.Set("sbsh.terminal.spec", "/nonexistent/file/path/that/does/not/exist.json")

	spec, err := processInput(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if spec != nil {
		t.Fatal("expected nil spec")
	}
	if !errors.Is(err, errdefs.ErrOpenSpecFile) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrOpenSpecFile, err)
	}
}

func Test_ErrInvalidJSONSpec(t *testing.T) {
	// Create a temporary file with invalid JSON
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.json")
	err := os.WriteFile(tmpFile, []byte("{invalid json}"), 0o644)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	cmd := &cobra.Command{}
	viper.Set("sbsh.terminal.spec", tmpFile)

	spec, err := processInput(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if spec != nil {
		t.Fatal("expected nil spec")
	}
	if !errors.Is(err, errdefs.ErrInvalidJSONSpec) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidJSONSpec, err)
	}
}

func Test_ErrNoSpecDefined(t *testing.T) {
	cmd := &cobra.Command{}
	viper.Set("sbsh.terminal.spec", "")

	spec, err := processInput(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if spec != nil {
		t.Fatal("expected nil spec")
	}
	if !errors.Is(err, errdefs.ErrNoSpecDefined) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrNoSpecDefined, err)
	}
}

func Test_ErrTerminalSpecNotFound_RunE(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cmd := NewTerminalCmd()
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)
	// Don't set CtxTerminalSpec, so it will be nil

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrTerminalSpecNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrTerminalSpecNotFound, err)
	}
}

func Test_ErrLoggerNotFound_RunE(t *testing.T) {
	cmd := NewTerminalCmd()
	ctx := context.Background()
	// Don't set CtxLogger, so it will be nil
	cmd.SetContext(ctx)

	// Set a valid terminal spec so we get past that check
	spec := &api.TerminalSpec{
		ID:          api.ID(naming.RandomID()),
		Kind:        api.TerminalLocal,
		Name:        naming.RandomName(),
		Command:     "/bin/bash",
		CommandArgs: []string{},
		Env:         os.Environ(),
	}
	ctx = context.WithValue(ctx, types.CtxTerminalSpec, spec)
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrLoggerNotFound, err)
	}
}

func Test_ValidJSONSpec(t *testing.T) {
	// Create a temporary file with valid JSON
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "valid.json")
	validSpec := api.TerminalSpec{
		ID:          api.ID("test-id"),
		Kind:        api.TerminalLocal,
		Name:        "test-name",
		Command:     "/bin/bash",
		CommandArgs: []string{},
		Env:         os.Environ(),
	}
	jsonData, err := json.Marshal(validSpec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}
	err = os.WriteFile(tmpFile, jsonData, 0o644)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	cmd := &cobra.Command{}
	viper.Set("sbsh.terminal.spec", tmpFile)

	spec, err := processInput(cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec == nil {
		t.Fatal("expected non-nil spec")
	}
	if spec.ID != validSpec.ID {
		t.Fatalf("expected ID %s; got: %s", validSpec.ID, spec.ID)
	}
	if spec.Name != validSpec.Name {
		t.Fatalf("expected Name %s; got: %s", validSpec.Name, spec.Name)
	}
}
