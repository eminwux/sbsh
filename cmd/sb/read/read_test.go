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

package read

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
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
// discovery.FindTerminalByName resolves it. capturePath/socket may be empty to
// exercise the missing-field error paths.
func writeTerminalMetadata(t *testing.T, runPath, id, name, capturePath, socket string) {
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
		Status: api.TerminalStatus{
			State:       api.Ready,
			CaptureFile: capturePath,
			SocketFile:  socket,
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
}

func Test_NewReadCmd_Metadata(t *testing.T) {
	cmd := NewReadCmd()
	if cmd == nil {
		t.Fatal("NewReadCmd returned nil")
	}
	if cmd.Use != "read <name>" {
		t.Errorf("Use = %q", cmd.Use)
	}
	if cmd.Flags().Lookup("follow") == nil {
		t.Error("--follow flag not registered")
	}
}

func Test_RunE_ErrLoggerNotFound(t *testing.T) {
	cmd := NewReadCmd()
	cmd.SetContext(context.Background())

	err := cmd.RunE(cmd, []string{"some-terminal"})
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected ErrLoggerNotFound, got: %v", err)
	}
}

func Test_RunE_ErrNoTerminalIdentifier(t *testing.T) {
	cmd := NewReadCmd()
	ctx := context.WithValue(context.Background(), types.CtxLogger, testLogger())
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, []string{""})
	if !errors.Is(err, errdefs.ErrNoTerminalIdentifier) {
		t.Fatalf("expected ErrNoTerminalIdentifier, got: %v", err)
	}
}

func Test_runRead_TerminalNotFound(t *testing.T) {
	runPath := t.TempDir()
	opts := readOpts{runPath: runPath, name: "ghost"}

	err := runRead(context.Background(), testLogger(), io.Discard, io.Discard, opts)
	if !errors.Is(err, errdefs.ErrTerminalNotFound) {
		t.Fatalf("expected ErrTerminalNotFound, got: %v", err)
	}
}

func Test_runRead_NoCaptureFileRecorded(t *testing.T) {
	runPath := t.TempDir()
	writeTerminalMetadata(t, runPath, "term1", "alice", "", "")
	opts := readOpts{runPath: runPath, name: "alice"}

	err := runRead(context.Background(), testLogger(), io.Discard, io.Discard, opts)
	if err == nil {
		t.Fatal("expected error for missing capture file, got nil")
	}
	if errors.Is(err, errdefs.ErrTerminalNotFound) {
		t.Fatalf("terminal should resolve; got not-found: %v", err)
	}
}

func Test_runRead_NoFollow_DumpsCapture(t *testing.T) {
	runPath := t.TempDir()
	capturePath := filepath.Join(runPath, "capture.log")
	want := []byte("hello from the capture log\n")
	if err := os.WriteFile(capturePath, want, 0o644); err != nil {
		t.Fatalf("write capture: %v", err)
	}
	writeTerminalMetadata(t, runPath, "term1", "alice", capturePath, "")
	opts := readOpts{runPath: runPath, name: "alice", follow: false}

	var out bytes.Buffer
	if err := runRead(context.Background(), testLogger(), &out, io.Discard, opts); err != nil {
		t.Fatalf("runRead returned error: %v", err)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Errorf("dumped capture = %q, want %q", out.Bytes(), want)
	}
}

func Test_dumpCapture_RawPassthrough(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.log")
	want := []byte("raw bytes\x1b[0m\n")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("write capture: %v", err)
	}

	var out bytes.Buffer
	if err := dumpCapture(path, &out); err != nil {
		t.Fatalf("dumpCapture returned error: %v", err)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Errorf("dumpCapture wrote %q, want %q", out.Bytes(), want)
	}
}

func Test_dumpCapture_AbsentFileNoOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "never-written.log")

	var out bytes.Buffer
	if err := dumpCapture(path, &out); err != nil {
		t.Fatalf("dumpCapture on absent file should not error, got: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("expected no output for absent capture, got %q", out.Bytes())
	}
}

// errWriter fails on every Write to exercise the write-error path.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

func Test_dumpCapture_WriteError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.log")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("write capture: %v", err)
	}

	if err := dumpCapture(path, errWriter{}); err == nil {
		t.Fatal("expected write error, got nil")
	}
}

func Test_streamLive_NormalEOF(t *testing.T) {
	client, server := net.Pipe()
	go func() {
		_, _ = server.Write([]byte("live output\n"))
		_ = server.Close()
	}()

	var out bytes.Buffer
	if err := streamLive(client, &out, io.Discard); err != nil {
		t.Fatalf("streamLive returned error: %v", err)
	}
	if out.String() != "live output\n" {
		t.Errorf("streamLive wrote %q", out.String())
	}
}

func Test_streamLive_LaggedSentinel(t *testing.T) {
	client, server := net.Pipe()
	go func() {
		_, _ = server.Write(append([]byte("some output "), laggedNotice...))
		_ = server.Close()
	}()

	var out, errOut bytes.Buffer
	err := streamLive(client, &out, &errOut)
	if !errors.Is(err, errdefs.ErrSubscriberLagged) {
		t.Fatalf("expected ErrSubscriberLagged, got: %v", err)
	}
	if errOut.Len() == 0 {
		t.Error("expected a lagged notice on stderr")
	}
}

func Test_streamLive_LaggedSentinelSplitAcrossReads(t *testing.T) {
	half := len(laggedNotice) / 2
	client, server := net.Pipe()
	go func() {
		// net.Pipe is unbuffered: each Write pairs with a Read, so the
		// sentinel arrives split across two streamLive read iterations.
		_, _ = server.Write(laggedNotice[:half])
		_, _ = server.Write(laggedNotice[half:])
		_ = server.Close()
	}()

	var out, errOut bytes.Buffer
	err := streamLive(client, &out, &errOut)
	if !errors.Is(err, errdefs.ErrSubscriberLagged) {
		t.Fatalf("expected ErrSubscriberLagged on split sentinel, got: %v", err)
	}
}

func Test_streamLive_WriteError(t *testing.T) {
	client, server := net.Pipe()
	go func() {
		_, _ = server.Write([]byte("x"))
		_ = server.Close()
	}()

	err := streamLive(client, errWriter{}, io.Discard)
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
}
