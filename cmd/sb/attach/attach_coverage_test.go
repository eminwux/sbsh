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

package attach

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/spf13/viper"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func Test_resolveTerminalSocket_SocketFlag(t *testing.T) {
	t.Cleanup(viper.Reset)
	logger := discardLogger()
	cmd := NewAttachCmd()
	cmd.SetContext(context.WithValue(context.Background(), types.CtxLogger, logger))

	viper.Set(config.SB_ATTACH_SOCKET.ViperKey, "/tmp/explicit.sock")

	got, err := resolveTerminalSocket(cmd, logger, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/explicit.sock" {
		t.Fatalf("expected /tmp/explicit.sock; got %q", got)
	}
}

func Test_resolveTerminalSocket_IDFlag(t *testing.T) {
	t.Cleanup(viper.Reset)
	logger := discardLogger()
	cmd := NewAttachCmd()
	cmd.SetContext(context.WithValue(context.Background(), types.CtxLogger, logger))

	viper.Set(config.SB_ATTACH_SOCKET.ViperKey, "")
	viper.Set(config.SB_ROOT_RUN_PATH.ViperKey, "/run")
	viper.Set(config.SB_ATTACH_ID.ViperKey, "term-123")
	viper.Set(config.SB_ATTACH_NAME.ViperKey, "")

	got, err := resolveTerminalSocket(cmd, logger, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/run", defaults.TerminalsRunPath, "term-123", "socket")
	if got != want {
		t.Fatalf("expected %q; got %q", want, got)
	}
}

// Test_RunE_PositionalNameResolutionFails drives RunE past argument
// validation and into run(), where the positional name fails to resolve to
// a terminal ID. This exercises the RunE debug-logging path without reaching
// the live attach loop (which would call os.Exit on a bad socket).
func Test_RunE_PositionalNameResolutionFails(t *testing.T) {
	t.Cleanup(viper.Reset)
	logger := discardLogger()
	cmd := NewAttachCmd()
	cmd.SetContext(context.WithValue(context.Background(), types.CtxLogger, logger))

	tmp := t.TempDir()
	viper.Set(config.SB_ATTACH_SOCKET.ViperKey, "")
	viper.Set(config.SB_ROOT_RUN_PATH.ViperKey, tmp)
	viper.Set(config.SB_ATTACH_ID.ViperKey, "")
	viper.Set(config.SB_ATTACH_NAME.ViperKey, "")

	err := cmd.RunE(cmd, []string{"no-such-terminal"})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrResolveTerminalName) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrResolveTerminalName, err)
	}
}

func Test_FlagCompletionFuncs(t *testing.T) {
	t.Setenv(config.SB_ROOT_RUN_PATH.EnvVar(), t.TempDir())
	cmd := NewAttachCmd()

	for _, name := range []string{"id", "name"} {
		fn, ok := cmd.GetFlagCompletionFunc(name)
		if !ok {
			t.Fatalf("no completion func registered for --%s", name)
		}
		// The closures fail silently on an empty run dir; we only need them
		// to execute so their bodies are exercised.
		_, directive := fn(cmd, nil, "")
		_ = directive
	}
}
