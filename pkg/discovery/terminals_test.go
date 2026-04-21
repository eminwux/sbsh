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

package discovery_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/eminwux/sbsh/pkg/discovery"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func writeTerminalDoc(t *testing.T, runPath, id, name string) {
	t.Helper()
	dir := filepath.Join(runPath, defaults.TerminalsRunPath, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	doc := api.TerminalDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminal,
		Metadata:   api.TerminalMetadata{Name: name},
		Spec:       api.TerminalSpec{ID: api.ID(id), Name: name},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), b, 0o644); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}
}

func writeInvalidTerminalJSON(t *testing.T, runPath, id string) {
	t.Helper()
	dir := filepath.Join(runPath, defaults.TerminalsRunPath, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestScanTerminals_EmptyRunPath(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	runPath := t.TempDir()

	got, err := discovery.ScanTerminals(ctx, logger, runPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 terminals, got %d", len(got))
	}
}

func TestScanTerminals_MissingRunPath(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	runPath := filepath.Join(t.TempDir(), "does-not-exist")

	got, err := discovery.ScanTerminals(ctx, logger, runPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 terminals, got %d", len(got))
	}
}

func TestScanTerminals_InvalidJSON(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	runPath := t.TempDir()
	writeInvalidTerminalJSON(t, runPath, "bad-id")

	_, err := discovery.ScanTerminals(ctx, logger, runPath)
	if err == nil {
		t.Fatal("expected error on invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error, got: %v", err)
	}
}

func TestScanTerminals_SortedByIDThenName(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	runPath := t.TempDir()

	// Deliberately insert out of order.
	writeTerminalDoc(t, runPath, "id-b", "beta")
	writeTerminalDoc(t, runPath, "id-a", "alpha")
	writeTerminalDoc(t, runPath, "id-c", "charlie")

	got, err := discovery.ScanTerminals(ctx, logger, runPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 terminals, got %d", len(got))
	}
	wantIDs := []string{"id-a", "id-b", "id-c"}
	for i, w := range wantIDs {
		if string(got[i].Spec.ID) != w {
			t.Fatalf("sort mismatch at %d: want %q, got %q", i, w, got[i].Spec.ID)
		}
	}
}

func TestFindTerminalByID(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	runPath := t.TempDir()
	writeTerminalDoc(t, runPath, "id-1", "one")
	writeTerminalDoc(t, runPath, "id-2", "two")

	got, err := discovery.FindTerminalByID(ctx, logger, runPath, "id-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || string(got.Spec.ID) != "id-2" {
		t.Fatalf("want id-2, got %+v", got)
	}
}

func TestFindTerminalByID_NotFound(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	runPath := t.TempDir()
	writeTerminalDoc(t, runPath, "id-1", "one")

	_, err := discovery.FindTerminalByID(ctx, logger, runPath, "missing")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected error to mention %q, got: %v", "missing", err)
	}
}

func TestFindTerminalByName(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	runPath := t.TempDir()
	writeTerminalDoc(t, runPath, "id-1", "one")
	writeTerminalDoc(t, runPath, "id-2", "two")

	got, err := discovery.FindTerminalByName(ctx, logger, runPath, "two")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Spec.Name != "two" {
		t.Fatalf("want name=two, got %+v", got)
	}
}

func TestFindTerminalByName_NotFound(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	runPath := t.TempDir()
	writeTerminalDoc(t, runPath, "id-1", "one")

	_, err := discovery.FindTerminalByName(ctx, logger, runPath, "ghost")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("expected error to mention %q, got: %v", "ghost", err)
	}
}
