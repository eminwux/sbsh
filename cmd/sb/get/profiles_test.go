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

func Test_ErrLoggerNotFound_Profile_RunE(t *testing.T) {
	cmd := NewGetProfilesCmd()
	ctx := context.Background()
	// Don't set CtxLogger, so it will be nil
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	// listProfiles will check logger and return ErrLoggerNotFound
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrLoggerNotFound, err)
	}
}

func Test_ErrInvalidFlag_Profile_Output(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := NewGetProfilesCmd()
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

func Test_ErrTooManyArguments_Profile(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := NewGetProfilesCmd()
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	// Too many args
	err := cmd.RunE(cmd, []string{"profile-name-1", "profile-name-2"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrTooManyArguments) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrTooManyArguments, err)
	}
}

func Test_ErrLoggerNotFound_Profile_GetProfile(t *testing.T) {
	cmd := &cobra.Command{}
	ctx := context.Background()
	cmd.SetContext(ctx)

	err := getProfile(cmd, []string{"profile-name"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrLoggerNotFound, err)
	}
}

func Test_ErrInvalidOutputFormat_Profile(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cmd := &cobra.Command{}
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	// Set invalid output format
	viper.Set(config.SB_PROFILES_OUTPUT.ViperKey, "invalid-format")

	err := getProfile(cmd, []string{"profile-name"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrInvalidOutputFormat) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrInvalidOutputFormat, err)
	}

	viper.Set(config.SB_PROFILES_OUTPUT.ViperKey, "")
}
