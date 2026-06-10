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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/pkg/discovery"
)

// backdateDir pushes a directory's mtime past the orphan grace period so
// tests exercise the steady-state (not freshly-created) classification.
func backdateDir(t *testing.T, dir string) {
	t.Helper()
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatalf("chtimes %s: %v", dir, err)
	}
}

func TestOrphanTerminalDirs(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	t.Run("missing terminals root yields no orphans", func(t *testing.T) {
		got, err := discovery.OrphanTerminalDirs(ctx, logger, t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected no orphans, got %v", got)
		}
	})

	t.Run("healthy dirs are not orphans", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-ok", "alpha")
		backdateDir(t, filepath.Join(runPath, defaults.TerminalsRunPath, "id-ok"))

		got, err := discovery.OrphanTerminalDirs(ctx, logger, runPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected no orphans, got %v", got)
		}
	})

	t.Run("missing and corrupt metadata classify as orphans", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-ok", "alpha")
		writeInvalidTerminalJSON(t, runPath, "id-corrupt")
		missingDir := filepath.Join(runPath, defaults.TerminalsRunPath, "id-missing")
		if err := os.MkdirAll(missingDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(missingDir, "log"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write log: %v", err)
		}
		for _, id := range []string{"id-ok", "id-corrupt", "id-missing"} {
			backdateDir(t, filepath.Join(runPath, defaults.TerminalsRunPath, id))
		}

		got, err := discovery.OrphanTerminalDirs(ctx, logger, runPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]bool{
			filepath.Join(runPath, defaults.TerminalsRunPath, "id-corrupt"): true,
			filepath.Join(runPath, defaults.TerminalsRunPath, "id-missing"): true,
		}
		if len(got) != len(want) {
			t.Fatalf("expected %d orphans, got %v", len(want), got)
		}
		for _, dir := range got {
			if !want[dir] {
				t.Fatalf("unexpected orphan %s (got %v)", dir, got)
			}
		}
	})

	t.Run("recently modified dirs are shielded by the grace period", func(t *testing.T) {
		runPath := t.TempDir()
		freshDir := filepath.Join(runPath, defaults.TerminalsRunPath, "id-fresh")
		if err := os.MkdirAll(freshDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		got, err := discovery.OrphanTerminalDirs(ctx, logger, runPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected fresh dir to be shielded, got %v", got)
		}
	})

	t.Run("stray files under terminals root are ignored", func(t *testing.T) {
		runPath := t.TempDir()
		root := filepath.Join(runPath, defaults.TerminalsRunPath)
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "stray"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write stray: %v", err)
		}

		got, err := discovery.OrphanTerminalDirs(ctx, logger, runPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected stray file to be ignored, got %v", got)
		}
	})
}
