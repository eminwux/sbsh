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

package logging

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/cmd/types"
	"github.com/spf13/cobra"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"nonsense", slog.LevelInfo},
	}
	for _, tc := range cases {
		if got := ParseLevel(tc.in); got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestNewNoopLogger_NeverEnabledAndNoPanic(t *testing.T) {
	l := NewNoopLogger()
	if l == nil {
		t.Fatal("NewNoopLogger() = nil")
	}
	if l.Enabled(context.Background(), slog.LevelError) {
		t.Errorf("noop logger reports Enabled = true; want false at every level")
	}
	// Logging through the noop handler must be a safe no-op.
	l.Info("dropped", "k", "v")
	l.Error("dropped", "k", "v")
}

func TestNoopHandler_Methods(t *testing.T) {
	h := noopHandler{}
	ctx := context.Background()
	if h.Enabled(ctx, slog.LevelDebug) {
		t.Errorf("Enabled = true; want false")
	}
	if err := h.Handle(ctx, slog.Record{}); err != nil {
		t.Errorf("Handle error = %v; want nil", err)
	}
	if got := h.WithAttrs([]slog.Attr{slog.String("a", "b")}); got == nil {
		t.Errorf("WithAttrs = nil; want a handler")
	}
	if got := h.WithGroup("g"); got == nil {
		t.Errorf("WithGroup = nil; want a handler")
	}
}

func TestSetupFileLogger_RejectsEmptyArgs(t *testing.T) {
	cmd := &cobra.Command{}
	cases := []struct {
		name     string
		cmd      *cobra.Command
		logfile  string
		loglevel string
	}{
		{"nil cmd", nil, "x.log", "info"},
		{"empty logfile", cmd, "", "info"},
		{"empty loglevel", cmd, "x.log", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := SetupFileLogger(tc.cmd, tc.logfile, tc.loglevel); err == nil {
				t.Errorf("SetupFileLogger(%v, %q, %q) = nil; want error", tc.cmd, tc.logfile, tc.loglevel)
			}
		})
	}
}

func TestSetupFileLogger_PopulatesContext(t *testing.T) {
	logfile := filepath.Join(t.TempDir(), "sub", "test.log")
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	if err := SetupFileLogger(cmd, logfile, "debug"); err != nil {
		t.Fatalf("SetupFileLogger error: %v", err)
	}

	ctx := cmd.Context()
	if ctx.Value(types.CtxLogger) == nil {
		t.Errorf("CtxLogger not set in context")
	}
	if ctx.Value(types.CtxLevelVar) == nil {
		t.Errorf("CtxLevelVar not set in context")
	}
	if ctx.Value(types.CtxHandler) == nil {
		t.Errorf("CtxHandler not set in context")
	}
	if _, ok := ctx.Value(types.CtxLogger).(*slog.Logger); !ok {
		t.Errorf("CtxLogger is not a *slog.Logger")
	}
}
