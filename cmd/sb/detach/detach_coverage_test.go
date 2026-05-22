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
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Test_RunE_PositionalNameResolutionFails drives RunE past argument
// validation and through runDetachCmd's positional-name path, where the
// name fails to resolve to a client ID.
func Test_RunE_PositionalNameResolutionFails(t *testing.T) {
	t.Cleanup(viper.Reset)
	logger := discardLogger()
	cmd := NewDetachCmd()
	cmd.SetContext(context.WithValue(context.Background(), types.CtxLogger, logger))

	t.Setenv(config.SB_ROOT_RUN_PATH.EnvVar(), t.TempDir())
	viper.Set(config.SB_DETACH_SOCKET.ViperKey, "")
	viper.Set(config.SB_DETACH_ID.ViperKey, "")
	viper.Set(config.SB_DETACH_NAME.ViperKey, "")

	err := cmd.RunE(cmd, []string{"no-such-client"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrBuildSocketPath) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrBuildSocketPath, err)
	}
}

// Test_runDetachCmd_NameFlagResolutionFails covers the --name (no positional)
// branch where clientName is taken from the flag and then fails to resolve.
func Test_runDetachCmd_NameFlagResolutionFails(t *testing.T) {
	t.Cleanup(viper.Reset)
	logger := discardLogger()
	cmd := &cobra.Command{}
	cmd.SetContext(context.WithValue(context.Background(), types.CtxLogger, logger))

	t.Setenv(config.SB_ROOT_RUN_PATH.EnvVar(), t.TempDir())
	viper.Set(config.SB_DETACH_SOCKET.ViperKey, "")
	viper.Set(config.SB_DETACH_ID.ViperKey, "")
	viper.Set(config.SB_DETACH_NAME.ViperKey, "some-client-name")

	err := runDetachCmd(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrBuildSocketPath) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrBuildSocketPath, err)
	}
}

// Test_runDetachCmd_SocketDialFails covers the RPC-client path: with an
// explicit --socket pointing at a non-existent socket, the dial fails fast
// and runDetachCmd returns ErrDetachTerminal.
func Test_runDetachCmd_SocketDialFails(t *testing.T) {
	t.Cleanup(viper.Reset)
	logger := discardLogger()
	cmd := &cobra.Command{}
	cmd.SetContext(context.WithValue(context.Background(), types.CtxLogger, logger))

	missing := filepath.Join(t.TempDir(), "missing.sock")
	viper.Set(config.SB_DETACH_SOCKET.ViperKey, missing)
	viper.Set(config.SB_DETACH_ID.ViperKey, "")
	viper.Set(config.SB_DETACH_NAME.ViperKey, "")

	err := runDetachCmd(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrDetachTerminal) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrDetachTerminal, err)
	}
}
