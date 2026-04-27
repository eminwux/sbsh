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
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

func writeTerminalDoc(t *testing.T, runPath, id, name string, state api.TerminalStatusMode, pid int) {
	t.Helper()
	dir := filepath.Join(runPath, defaults.TerminalsRunPath, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	doc := api.TerminalDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminal,
		Metadata:   api.TerminalMetadata{Name: name},
		Spec: api.TerminalSpec{
			ID:      api.ID(id),
			Name:    name,
			RunPath: runPath,
		},
		Status: api.TerminalStatus{
			State:           state,
			Pid:             pid,
			TerminalRunPath: dir,
		},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), b, 0o644); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestVerifyTerminalNameAvailable(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()
	livePid := os.Getpid()
	// PID 0 is reserved on Unix; isProcessAlive treats it as "unknown / alive",
	// so a defunct PID like a very high number signals a dead process.
	const deadPid = 0x7fffffff

	t.Run("empty name is always available", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-1", "alpha", api.Ready, livePid)
		if err := discovery.VerifyTerminalNameAvailable(ctx, logger, runPath, ""); err != nil {
			t.Fatalf("expected nil for empty name, got: %v", err)
		}
	})

	t.Run("empty runPath is always available", func(t *testing.T) {
		if err := discovery.VerifyTerminalNameAvailable(ctx, logger, "", "alpha"); err != nil {
			t.Fatalf("expected nil for empty runPath, got: %v", err)
		}
	})

	t.Run("no terminals", func(t *testing.T) {
		runPath := t.TempDir()
		if err := discovery.VerifyTerminalNameAvailable(ctx, logger, runPath, "alpha"); err != nil {
			t.Fatalf("expected nil, got: %v", err)
		}
	})

	t.Run("active terminal blocks reuse", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-1", "alpha", api.Ready, livePid)
		err := discovery.VerifyTerminalNameAvailable(ctx, logger, runPath, "alpha")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errdefs.ErrTerminalNameInUse) {
			t.Fatalf("expected ErrTerminalNameInUse, got: %v", err)
		}
	})

	t.Run("active terminal does not block a different name", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-1", "alpha", api.Ready, livePid)
		if err := discovery.VerifyTerminalNameAvailable(ctx, logger, runPath, "beta"); err != nil {
			t.Fatalf("expected nil for non-colliding name, got: %v", err)
		}
	})

	t.Run("exited terminal allows reuse", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-1", "alpha", api.Exited, livePid)
		if err := discovery.VerifyTerminalNameAvailable(ctx, logger, runPath, "alpha"); err != nil {
			t.Fatalf("expected nil for exited terminal, got: %v", err)
		}
	})

	t.Run("stale active terminal is reconciled to exited and allows reuse", func(t *testing.T) {
		runPath := t.TempDir()
		// State says Ready, but the PID is gone — Reconcile should flip it to
		// Exited under the hood, so the name becomes available.
		writeTerminalDoc(t, runPath, "id-1", "alpha", api.Ready, deadPid)
		if err := discovery.VerifyTerminalNameAvailable(ctx, logger, runPath, "alpha"); err != nil {
			t.Fatalf("expected nil after stale-state reconciliation, got: %v", err)
		}
	})

	t.Run("collision picks the active match when an exited terminal shares the name", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-old", "alpha", api.Exited, livePid)
		writeTerminalDoc(t, runPath, "id-new", "alpha", api.Ready, livePid)
		err := discovery.VerifyTerminalNameAvailable(ctx, logger, runPath, "alpha")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errdefs.ErrTerminalNameInUse) {
			t.Fatalf("expected ErrTerminalNameInUse, got: %v", err)
		}
	})
}
