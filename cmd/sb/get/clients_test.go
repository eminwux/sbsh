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

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func Test_ErrLoggerNotFound_Client_RunE(t *testing.T) {
	cmd := NewGetClientCmd()
	ctx := context.Background()
	// Don't set CtxLogger, so it will be nil
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	// listClients will check logger and return ErrLoggerNotFound
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrLoggerNotFound, err)
	}
}

func Test_ErrInvalidOutputFormat_Client_List(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := &cobra.Command{}
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	viper.Set(config.SB_GET_CLIENTS_OUTPUT.ViperKey, "bogus")
	defer viper.Set(config.SB_GET_CLIENTS_OUTPUT.ViperKey, "")

	err := listClients(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrInvalidOutputFormat) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidOutputFormat, err)
	}
}

func Test_ErrTooManyArguments_Client(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := NewGetClientCmd()
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	// Too many args
	err := cmd.RunE(cmd, []string{"client-name-1", "client-name-2"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrTooManyArguments) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrTooManyArguments, err)
	}
}

func Test_ErrLoggerNotFound_Client_GetClient(t *testing.T) {
	cmd := &cobra.Command{}
	ctx := context.Background()
	cmd.SetContext(ctx)

	err := getClient(cmd, []string{"client-name"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrLoggerNotFound, err)
	}
}

func Test_ErrInvalidOutputFormat_Client(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := &cobra.Command{}
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	// Set invalid output format
	viper.Set(config.SB_GET_CLIENTS_OUTPUT.ViperKey, "invalid-format")

	err := getClient(cmd, []string{"client-name"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrInvalidOutputFormat) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidOutputFormat, err)
	}

	viper.Set(config.SB_GET_CLIENTS_OUTPUT.ViperKey, "")
}

func Test_ErrGetRunPath_Client(t *testing.T) {
	// This test verifies that ErrGetRunPath is properly defined
	err := errdefs.ErrGetRunPath
	if err == nil {
		t.Fatal("ErrGetRunPath should not be nil")
	}
	if !errors.Is(err, errdefs.ErrGetRunPath) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrGetRunPath, err)
	}
}

func Test_ErrNoClientsFound(t *testing.T) {
	// This test verifies that ErrNoClientsFound is properly defined
	err := errdefs.ErrNoClientsFound
	if err == nil {
		t.Fatal("ErrNoClientsFound should not be nil")
	}
	if !errors.Is(err, errdefs.ErrNoClientsFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrNoClientsFound, err)
	}
}

func Test_ErrClientNotFound(t *testing.T) {
	// This test verifies that ErrClientNotFound is properly defined
	err := errdefs.ErrClientNotFound
	if err == nil {
		t.Fatal("ErrClientNotFound should not be nil")
	}
	if !errors.Is(err, errdefs.ErrClientNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrClientNotFound, err)
	}
}
