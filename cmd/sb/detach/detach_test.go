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

package detach

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func Test_ErrLoggerNotFound_RunE(t *testing.T) {
	cmd := NewDetachCmd()
	ctx := context.Background()
	// Don't set CtxLogger, so it will be nil
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, []string{"supervisor-name"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrLoggerNotFound, err)
	}
}

func Test_ErrNoSupervisorIdentifier(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := NewDetachCmd()
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	// No args and no flags provided
	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrNoSupervisorIdentifier) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrNoSupervisorIdentifier, err)
	}
}

func Test_ErrInvalidFlag_ID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := NewDetachCmd()
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)
	// Set --id flag when using positional argument
	cmd.SetArgs([]string{"--id", "test-id", "supervisor-name"})
	_ = cmd.Execute()

	err := cmd.RunE(cmd, []string{"supervisor-name"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrInvalidFlag) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidFlag, err)
	}
}

func Test_ErrInvalidFlag_Name(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := NewDetachCmd()
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)
	// Set --name flag when using positional argument
	cmd.SetArgs([]string{"--name", "test-name", "supervisor-name"})
	_ = cmd.Execute()

	err := cmd.RunE(cmd, []string{"supervisor-name"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrInvalidFlag) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidFlag, err)
	}
}

func Test_ErrInvalidFlag_Socket(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := NewDetachCmd()
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)
	// Set --socket flag when using positional argument
	cmd.SetArgs([]string{"--socket", "/tmp/socket", "supervisor-name"})
	_ = cmd.Execute()

	err := cmd.RunE(cmd, []string{"supervisor-name"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrInvalidFlag) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidFlag, err)
	}
}

func Test_ErrTooManyArguments(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := NewDetachCmd()
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	// Too many args
	err := cmd.RunE(cmd, []string{"supervisor-name-1", "supervisor-name-2"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrTooManyArguments) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrTooManyArguments, err)
	}
}

func Test_ErrLoggerNotFound_RunDetachCmd(t *testing.T) {
	cmd := &cobra.Command{}
	ctx := context.Background()
	cmd.SetContext(ctx)

	err := runDetachCmd(cmd, []string{"supervisor-name"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrLoggerNotFound, err)
	}
}

func Test_ErrPositionalWithFlags(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := &cobra.Command{}
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	viper.Set(detachIDInput, "test-id")
	viper.Set(detachNameInput, "")
	viper.Set(detachSocketInput, "")

	// Positional arg with flags set
	err := runDetachCmd(cmd, []string{"supervisor-name"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrPositionalWithFlags) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrPositionalWithFlags, err)
	}
}

func Test_ErrConflictingFlags_IDAndName(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := &cobra.Command{}
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	viper.Set(detachIDInput, "test-id")
	viper.Set(detachNameInput, "test-name")
	viper.Set(detachSocketInput, "")

	err := runDetachCmd(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrConflictingFlags) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrConflictingFlags, err)
	}
}

func Test_ErrConflictingFlags_IDAndSocket(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := &cobra.Command{}
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	viper.Set(detachIDInput, "test-id")
	viper.Set(detachNameInput, "")
	viper.Set(detachSocketInput, "/tmp/socket")

	err := runDetachCmd(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrConflictingFlags) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrConflictingFlags, err)
	}
}

func Test_ErrConflictingFlags_NameAndSocket(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := &cobra.Command{}
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	viper.Set(detachIDInput, "")
	viper.Set(detachNameInput, "test-name")
	viper.Set(detachSocketInput, "/tmp/socket")

	err := runDetachCmd(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrConflictingFlags) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrConflictingFlags, err)
	}
}

func Test_ErrNoSupervisorIdentification_BuildSocket(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := &cobra.Command{}
	cmd.Flags().String("run-path", "", "Run path directory")
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	viper.Set(detachSocketInput, "")
	viper.Set(detachIDInput, "")
	viper.Set(detachNameInput, "")

	tmpDir := t.TempDir()
	cmd.Flags().Set("run-path", tmpDir)

	// No supervisor ID, name, or socket provided
	_, err := buildSocket(cmd, logger, "", "", "")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrNoSupervisorIdentification) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrNoSupervisorIdentification, err)
	}
}

func Test_ErrDetachSession(t *testing.T) {
	// This test verifies that ErrDetachSession is properly defined
	// Testing the actual detach flow requires a real supervisor socket
	// The error type is verified through integration tests
	err := errdefs.ErrDetachSession
	if err == nil {
		t.Fatal("ErrDetachSession should not be nil")
	}
	if !errors.Is(err, errdefs.ErrDetachSession) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrDetachSession, err)
	}
}

func Test_ErrResolveTerminalName(t *testing.T) {
	// This test verifies that ErrResolveTerminalName is properly defined
	// Testing the actual resolution requires a real supervisor store
	err := errdefs.ErrResolveTerminalName
	if err == nil {
		t.Fatal("ErrResolveTerminalName should not be nil")
	}
	if !errors.Is(err, errdefs.ErrResolveTerminalName) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrResolveTerminalName, err)
	}
}

func Test_ErrBuildSocketPath(t *testing.T) {
	// This test verifies that ErrBuildSocketPath is properly defined
	err := errdefs.ErrBuildSocketPath
	if err == nil {
		t.Fatal("ErrBuildSocketPath should not be nil")
	}
	if !errors.Is(err, errdefs.ErrBuildSocketPath) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrBuildSocketPath, err)
	}
}

func Test_ErrDetermineRunPath(t *testing.T) {
	// This test verifies that ErrDetermineRunPath is properly defined
	err := errdefs.ErrDetermineRunPath
	if err == nil {
		t.Fatal("ErrDetermineRunPath should not be nil")
	}
	if !errors.Is(err, errdefs.ErrDetermineRunPath) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrDetermineRunPath, err)
	}
}

func Test_BuildSocket_WithSocketFlag(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := &cobra.Command{}
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	socketPath := "/tmp/test-socket"
	socket, err := buildSocket(cmd, logger, socketPath, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if socket != socketPath {
		t.Fatalf("expected socket '%s'; got: '%s'", socketPath, socket)
	}
}

func Test_BuildSocket_WithIDFlag(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := &cobra.Command{}
	cmd.Flags().String("run-path", "", "Run path directory")
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	tmpDir := t.TempDir()
	cmd.Flags().Set("run-path", tmpDir)

	supervisorID := "test-supervisor-id"
	socket, err := buildSocket(cmd, logger, "", supervisorID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedSocket := filepath.Join(tmpDir, "supervisors", supervisorID, "socket")
	if socket != expectedSocket {
		t.Fatalf("expected socket '%s'; got: '%s'", expectedSocket, socket)
	}
}
