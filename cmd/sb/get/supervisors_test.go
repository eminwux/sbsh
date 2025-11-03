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

package get

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func Test_ErrLoggerNotFound_Supervisor_RunE(t *testing.T) {
	cmd := NewGetSupervisorCmd()
	ctx := context.Background()
	// Don't set CtxLogger, so it will be nil
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	// listSupervisors will check logger and return ErrLoggerNotFound
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrLoggerNotFound, err)
	}
}

func Test_ErrInvalidFlag_Supervisor_Output(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := NewGetSupervisorCmd()
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	// Set output flag when listing (no args)
	cmd.SetArgs([]string{"--output", "json"})
	_ = cmd.Execute()

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrInvalidFlag) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidFlag, err)
	}
}

func Test_ErrTooManyArguments_Supervisor(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := NewGetSupervisorCmd()
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

func Test_ErrLoggerNotFound_Supervisor_GetSupervisor(t *testing.T) {
	cmd := &cobra.Command{}
	ctx := context.Background()
	cmd.SetContext(ctx)

	err := getSupervisor(cmd, []string{"supervisor-name"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrLoggerNotFound, err)
	}
}

func Test_ErrInvalidOutputFormat_Supervisor(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := &cobra.Command{}
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	// Set invalid output format
	viper.Set(outputFormatSupervisorInput, "invalid-format")

	err := getSupervisor(cmd, []string{"supervisor-name"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrInvalidOutputFormat) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidOutputFormat, err)
	}

	viper.Set(outputFormatSupervisorInput, "")
}

func Test_ErrGetRunPath_Supervisor(t *testing.T) {
	// This test verifies that ErrGetRunPath is properly defined
	err := errdefs.ErrGetRunPath
	if err == nil {
		t.Fatal("ErrGetRunPath should not be nil")
	}
	if !errors.Is(err, errdefs.ErrGetRunPath) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrGetRunPath, err)
	}
}

func Test_ErrNoSupervisorsFound(t *testing.T) {
	// This test verifies that ErrNoSupervisorsFound is properly defined
	err := errdefs.ErrNoSupervisorsFound
	if err == nil {
		t.Fatal("ErrNoSupervisorsFound should not be nil")
	}
	if !errors.Is(err, errdefs.ErrNoSupervisorsFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrNoSupervisorsFound, err)
	}
}

func Test_ErrSupervisorNotFound(t *testing.T) {
	// This test verifies that ErrSupervisorNotFound is properly defined
	err := errdefs.ErrSupervisorNotFound
	if err == nil {
		t.Fatal("ErrSupervisorNotFound should not be nil")
	}
	if !errors.Is(err, errdefs.ErrSupervisorNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrSupervisorNotFound, err)
	}
}
