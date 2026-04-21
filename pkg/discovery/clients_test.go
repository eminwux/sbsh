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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/eminwux/sbsh/pkg/discovery"
)

func writeClientDoc(t *testing.T, runPath, id, name string) {
	t.Helper()
	dir := filepath.Join(runPath, defaults.ClientsRunPath, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	doc := api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata:   api.ClientMetadata{Name: name},
		Spec:       api.ClientSpec{ID: api.ID(id)},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), b, 0o644); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}
}

func writeInvalidClientJSON(t *testing.T, runPath, id string) {
	t.Helper()
	dir := filepath.Join(runPath, defaults.ClientsRunPath, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte("{garbage"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestScanClients_EmptyRunPath(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	runPath := t.TempDir()

	got, err := discovery.ScanClients(ctx, logger, runPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 clients, got %d", len(got))
	}
}

func TestScanClients_MissingRunPath(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	runPath := filepath.Join(t.TempDir(), "nope")

	got, err := discovery.ScanClients(ctx, logger, runPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 clients, got %d", len(got))
	}
}

func TestScanClients_InvalidJSON(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	runPath := t.TempDir()
	writeInvalidClientJSON(t, runPath, "bad")

	_, err := discovery.ScanClients(ctx, logger, runPath)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
}

func TestScanClients_Sorted(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	runPath := t.TempDir()

	writeClientDoc(t, runPath, "c-z", "zeta")
	writeClientDoc(t, runPath, "c-a", "alpha")
	writeClientDoc(t, runPath, "c-m", "mu")

	got, err := discovery.ScanClients(ctx, logger, runPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantIDs := []string{"c-a", "c-m", "c-z"}
	if len(got) != len(wantIDs) {
		t.Fatalf("want %d clients, got %d", len(wantIDs), len(got))
	}
	for i, w := range wantIDs {
		if string(got[i].Spec.ID) != w {
			t.Fatalf("sort mismatch at %d: want %q, got %q", i, w, got[i].Spec.ID)
		}
	}
}

func TestFindClientByName(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	runPath := t.TempDir()
	writeClientDoc(t, runPath, "c-1", "one")
	writeClientDoc(t, runPath, "c-2", "two")

	got, err := discovery.FindClientByName(ctx, logger, runPath, "two")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Metadata.Name != "two" {
		t.Fatalf("want name=two, got %+v", got)
	}
}

func TestFindClientByName_NotFound(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	runPath := t.TempDir()
	writeClientDoc(t, runPath, "c-1", "one")

	_, err := discovery.FindClientByName(ctx, logger, runPath, "missing")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected error to mention %q, got: %v", "missing", err)
	}
}
