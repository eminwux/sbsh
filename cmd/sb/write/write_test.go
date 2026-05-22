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

package write

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/viper"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// writeTerminalMetadata writes a terminal metadata.json under runPath so
// discovery.FindTerminalByName resolves it. socket may be empty to exercise
// the missing-socket error path.
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
	if writeErr := os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o644); writeErr != nil {
		t.Fatalf("write metadata: %v", writeErr)
	}
}

func Test_NewWriteCmd_Metadata(t *testing.T) {
	cmd := NewWriteCmd()
	if cmd == nil {
		t.Fatal("NewWriteCmd returned nil")
	}
	if cmd.Use != "write <name> [text|-]" {
		t.Errorf("Use = %q", cmd.Use)
	}
	if cmd.Flags().Lookup("raw") == nil {
		t.Error("--raw flag not registered")
	}
}

func Test_RunE_ErrLoggerNotFound(t *testing.T) {
	cmd := NewWriteCmd()
	cmd.SetContext(context.Background())

	err := cmd.RunE(cmd, []string{"box", "hi"})
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected ErrLoggerNotFound, got: %v", err)
	}
}

func Test_ResolveWriteOpts(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		raw         bool
		wantErr     error
		wantPayload []byte
	}{
		{name: "no args", args: nil, wantErr: errdefs.ErrNoTerminalIdentifier},
		{name: "empty name", args: []string{""}, wantErr: errdefs.ErrNoTerminalIdentifier},
		{name: "name only yields empty payload", args: []string{"box"}, wantPayload: nil},
		{name: "caret parsed", args: []string{"box", "hi^M"}, wantPayload: append([]byte("hi"), 0x0D)},
		{name: "invalid caret", args: []string{"box", "^!"}, wantErr: errdefs.ErrInvalidArgument},
		{name: "raw verbatim", args: []string{"box", `^C\n`}, raw: true, wantPayload: []byte(`^C\n`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()
			_ = NewWriteCmd() // bind flags into viper
			if tc.raw {
				viper.Set(config.SB_WRITE_RAW.ViperKey, true)
			}
			opts, err := resolveWriteOpts(tc.args)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(opts.payload, tc.wantPayload) {
				t.Fatalf("payload = %v, want %v", opts.payload, tc.wantPayload)
			}
		})
	}
}

func Test_RunWrite_EmptyPayloadNoOp(t *testing.T) {
	err := runWrite(context.Background(), testLogger(), writeOpts{name: "box"})
	if err != nil {
		t.Fatalf("expected nil for empty payload, got: %v", err)
	}
}

func Test_RunWrite_TerminalNotFound(t *testing.T) {
	opts := writeOpts{runPath: t.TempDir(), name: "ghost", payload: []byte("x")}
	err := runWrite(context.Background(), testLogger(), opts)
	if !errors.Is(err, errdefs.ErrTerminalNotFound) {
		t.Fatalf("expected ErrTerminalNotFound, got: %v", err)
	}
}

func Test_RunWrite_NoSocketRecorded(t *testing.T) {
	runPath := t.TempDir()
	writeTerminalMetadata(t, runPath, "term1", "alice", "")
	opts := writeOpts{runPath: runPath, name: "alice", payload: []byte("x")}

	err := runWrite(context.Background(), testLogger(), opts)
	if err == nil {
		t.Fatal("expected error for missing socket, got nil")
	}
	if errors.Is(err, errdefs.ErrTerminalNotFound) {
		t.Fatalf("terminal should resolve; got not-found: %v", err)
	}
}
