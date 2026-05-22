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

package screenshot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// writeTerminalMetadata writes a terminal metadata.json under runPath so
// discovery.FindTerminalByName resolves it. socket may be empty to exercise the
// "no control socket recorded" path.
func writeTerminalMetadata(t *testing.T, runPath, id, name, socket string) {
	t.Helper()
	dir := filepath.Join(runPath, defaults.TerminalsRunPath, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir terminal dir: %v", err)
	}
	doc := api.TerminalDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminal,
		Metadata:   api.TerminalMetadata{Name: name},
		Spec:       api.TerminalSpec{ID: api.ID(id), Name: name, RunPath: runPath},
		Status:     api.TerminalStatus{State: api.Ready, SocketFile: socket},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
}

func Test_NewScreenshotCmd_Metadata(t *testing.T) {
	cmd := NewScreenshotCmd()
	if cmd == nil {
		t.Fatal("NewScreenshotCmd returned nil")
	}
	if cmd.Use != "screenshot <name>" {
		t.Errorf("Use = %q", cmd.Use)
	}
	if cmd.Flags().Lookup("plain") == nil {
		t.Error("--plain flag not registered")
	}
}

func Test_RunE_ErrLoggerNotFound(t *testing.T) {
	cmd := NewScreenshotCmd()
	cmd.SetContext(context.Background())

	err := cmd.RunE(cmd, []string{"some-terminal"})
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected ErrLoggerNotFound, got: %v", err)
	}
}

func Test_RunE_ErrNoTerminalIdentifier(t *testing.T) {
	cmd := NewScreenshotCmd()
	ctx := context.WithValue(context.Background(), types.CtxLogger, testLogger())
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, []string{""})
	if !errors.Is(err, errdefs.ErrNoTerminalIdentifier) {
		t.Fatalf("expected ErrNoTerminalIdentifier, got: %v", err)
	}
}

func Test_runScreenshot_TerminalNotFound(t *testing.T) {
	runPath := t.TempDir()
	opts := screenshotOpts{runPath: runPath, name: "ghost"}

	err := runScreenshot(context.Background(), testLogger(), io.Discard, opts)
	if !errors.Is(err, errdefs.ErrTerminalNotFound) {
		t.Fatalf("expected ErrTerminalNotFound, got: %v", err)
	}
}

func Test_runScreenshot_NoSocketRecorded(t *testing.T) {
	runPath := t.TempDir()
	writeTerminalMetadata(t, runPath, "term1", "alice", "")
	opts := screenshotOpts{runPath: runPath, name: "alice"}

	err := runScreenshot(context.Background(), testLogger(), io.Discard, opts)
	if err == nil {
		t.Fatal("expected error for missing socket, got nil")
	}
	if errors.Is(err, errdefs.ErrTerminalNotFound) {
		t.Fatalf("terminal should resolve; got not-found: %v", err)
	}
}
