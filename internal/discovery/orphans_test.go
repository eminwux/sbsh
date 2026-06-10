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
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/pkg/api"
)

// backdateDir pushes a directory's mtime past the orphan grace period so
// prune exercises the steady-state (not freshly-created) classification.
func backdateDir(t *testing.T, dir string) {
	t.Helper()
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatalf("chtimes %s: %v", dir, err)
	}
}

// truncateMetadata halves metadata.json in place, reproducing the
// corrupt-file shape from #391's repro.
func truncateMetadata(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "metadata.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := os.WriteFile(path, b[:len(b)/2], 0o644); err != nil {
		t.Fatalf("truncate %s: %v", path, err)
	}
}

// TestScanAndPruneTerminals_ReapsCorruptMetadataDir asserts a directory
// whose metadata.json is truncated mid-write is removed by prune even
// though ScanTerminals can never return it.
func TestScanAndPruneTerminals_ReapsCorruptMetadataDir(t *testing.T) {
	runPath := t.TempDir()
	writeTerminalDoc(t, runPath, "id-corrupt", "alpha", api.Exited, deadPid)
	termDir := filepath.Join(runPath, defaults.TerminalsRunPath, "id-corrupt")
	truncateMetadata(t, termDir)
	backdateDir(t, termDir)

	if err := discovery.ScanAndPruneTerminals(context.Background(), discardLogger(), runPath, io.Discard); err != nil {
		t.Fatalf("ScanAndPruneTerminals: unexpected error: %v", err)
	}

	if _, err := os.Stat(termDir); !os.IsNotExist(err) {
		t.Fatalf("expected corrupt-metadata terminal dir to be pruned, stat err=%v", err)
	}
}

// TestScanAndPruneTerminals_ReapsMissingMetadataDir asserts a directory with
// no metadata.json at all (terminal killed before the first write — the
// #374 window) is removed by prune.
func TestScanAndPruneTerminals_ReapsMissingMetadataDir(t *testing.T) {
	runPath := t.TempDir()
	termDir := filepath.Join(runPath, defaults.TerminalsRunPath, "id-missing")
	if err := os.MkdirAll(termDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(termDir, "log"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	backdateDir(t, termDir)

	var out bytes.Buffer
	if err := discovery.ScanAndPruneTerminals(context.Background(), discardLogger(), runPath, &out); err != nil {
		t.Fatalf("ScanAndPruneTerminals: unexpected error: %v", err)
	}

	if _, err := os.Stat(termDir); !os.IsNotExist(err) {
		t.Fatalf("expected missing-metadata terminal dir to be pruned, stat err=%v", err)
	}
	if !strings.Contains(out.String(), termDir) {
		t.Fatalf("expected prune output to name the reaped dir, got %q", out.String())
	}
}

// TestScanAndPruneTerminals_SkipsOrphanWithLiveSocket asserts an orphan
// directory hosting a unix socket with a live listener is never removed —
// corrupt metadata does not prove the terminal process is gone.
func TestScanAndPruneTerminals_SkipsOrphanWithLiveSocket(t *testing.T) {
	runPath := t.TempDir()
	termDir := filepath.Join(runPath, defaults.TerminalsRunPath, "id-live-sock")
	if err := os.MkdirAll(termDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ln, err := net.Listen("unix", filepath.Join(termDir, "socket"))
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, errAccept := ln.Accept()
			if errAccept != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	backdateDir(t, termDir)

	if err := discovery.ScanAndPruneTerminals(context.Background(), discardLogger(), runPath, io.Discard); err != nil {
		t.Fatalf("ScanAndPruneTerminals: unexpected error: %v", err)
	}

	if _, err := os.Stat(termDir); err != nil {
		t.Fatalf("expected orphan dir with live socket to survive prune, stat err=%v", err)
	}
}

// TestScanAndPruneTerminals_ShieldsFreshOrphanDir asserts a just-created
// metadata-less directory survives prune: the runner MkdirAlls the dir
// before the first metadata.json write lands, so reaping inside the grace
// window would race terminal startup.
func TestScanAndPruneTerminals_ShieldsFreshOrphanDir(t *testing.T) {
	runPath := t.TempDir()
	termDir := filepath.Join(runPath, defaults.TerminalsRunPath, "id-fresh")
	if err := os.MkdirAll(termDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := discovery.ScanAndPruneTerminals(context.Background(), discardLogger(), runPath, io.Discard); err != nil {
		t.Fatalf("ScanAndPruneTerminals: unexpected error: %v", err)
	}

	if _, err := os.Stat(termDir); err != nil {
		t.Fatalf("expected fresh dir to survive prune, stat err=%v", err)
	}
}

// TestScanAndPruneClients_ReapsCorruptMetadataDir asserts a client directory
// whose metadata.json is truncated mid-write is removed by prune even though
// ScanClients can never return it. See #422.
func TestScanAndPruneClients_ReapsCorruptMetadataDir(t *testing.T) {
	runPath := t.TempDir()
	writeClientDoc(t, runPath, "id-corrupt", api.ClientExited, deadPid)
	clientDir := filepath.Join(runPath, defaults.ClientsRunPath, "id-corrupt")
	truncateMetadata(t, clientDir)
	backdateDir(t, clientDir)

	if err := discovery.ScanAndPruneClients(context.Background(), discardLogger(), runPath, io.Discard); err != nil {
		t.Fatalf("ScanAndPruneClients: unexpected error: %v", err)
	}

	if _, err := os.Stat(clientDir); !os.IsNotExist(err) {
		t.Fatalf("expected corrupt-metadata client dir to be pruned, stat err=%v", err)
	}
}

// TestScanAndPruneClients_ReapsMissingMetadataDir asserts a client directory
// with no metadata.json at all (client killed before the first write) is
// removed by prune.
func TestScanAndPruneClients_ReapsMissingMetadataDir(t *testing.T) {
	runPath := t.TempDir()
	clientDir := filepath.Join(runPath, defaults.ClientsRunPath, "id-missing")
	if err := os.MkdirAll(clientDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(clientDir, "log"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	backdateDir(t, clientDir)

	var out bytes.Buffer
	if err := discovery.ScanAndPruneClients(context.Background(), discardLogger(), runPath, &out); err != nil {
		t.Fatalf("ScanAndPruneClients: unexpected error: %v", err)
	}

	if _, err := os.Stat(clientDir); !os.IsNotExist(err) {
		t.Fatalf("expected missing-metadata client dir to be pruned, stat err=%v", err)
	}
	if !strings.Contains(out.String(), clientDir) {
		t.Fatalf("expected prune output to name the reaped dir, got %q", out.String())
	}
}

// TestScanAndPruneClients_SkipsOrphanWithLiveSocket asserts an orphan client
// directory hosting a unix socket with a live listener is never removed —
// corrupt metadata does not prove the client process is gone.
func TestScanAndPruneClients_SkipsOrphanWithLiveSocket(t *testing.T) {
	runPath := t.TempDir()
	clientDir := filepath.Join(runPath, defaults.ClientsRunPath, "id-live-sock")
	if err := os.MkdirAll(clientDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ln, err := net.Listen("unix", filepath.Join(clientDir, "client.sock"))
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, errAccept := ln.Accept()
			if errAccept != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	backdateDir(t, clientDir)

	if err := discovery.ScanAndPruneClients(context.Background(), discardLogger(), runPath, io.Discard); err != nil {
		t.Fatalf("ScanAndPruneClients: unexpected error: %v", err)
	}

	if _, err := os.Stat(clientDir); err != nil {
		t.Fatalf("expected orphan dir with live socket to survive prune, stat err=%v", err)
	}
}

// TestScanAndPruneClients_ShieldsFreshOrphanDir asserts a just-created
// metadata-less client directory survives prune: the client MkdirAlls the
// dir before the first metadata.json write lands, so reaping inside the
// grace window would race client startup.
func TestScanAndPruneClients_ShieldsFreshOrphanDir(t *testing.T) {
	runPath := t.TempDir()
	clientDir := filepath.Join(runPath, defaults.ClientsRunPath, "id-fresh")
	if err := os.MkdirAll(clientDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := discovery.ScanAndPruneClients(context.Background(), discardLogger(), runPath, io.Discard); err != nil {
		t.Fatalf("ScanAndPruneClients: unexpected error: %v", err)
	}

	if _, err := os.Stat(clientDir); err != nil {
		t.Fatalf("expected fresh dir to survive prune, stat err=%v", err)
	}
}

// TestReportOrphanClientDirs asserts the `sb get clients` warning channel: a
// one-line, non-verbose signal when orphan client dirs exist, silence when
// none do.
func TestReportOrphanClientDirs(t *testing.T) {
	ctx := context.Background()

	t.Run("warns when orphans exist", func(t *testing.T) {
		runPath := t.TempDir()
		writeClientDoc(t, runPath, "id-corrupt", api.ClientExited, deadPid)
		clientDir := filepath.Join(runPath, defaults.ClientsRunPath, "id-corrupt")
		truncateMetadata(t, clientDir)
		backdateDir(t, clientDir)

		var out bytes.Buffer
		n, err := discovery.ReportOrphanClientDirs(ctx, discardLogger(), runPath, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 1 {
			t.Fatalf("expected 1 orphan, got %d", n)
		}
		if !strings.Contains(out.String(), "sb prune clients") {
			t.Fatalf("expected warning to name the cleanup command, got %q", out.String())
		}
	})

	t.Run("silent when no orphans", func(t *testing.T) {
		runPath := t.TempDir()
		writeClientDoc(t, runPath, "id-ok", api.ClientReady, os.Getpid())

		var out bytes.Buffer
		n, err := discovery.ReportOrphanClientDirs(ctx, discardLogger(), runPath, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 0 {
			t.Fatalf("expected 0 orphans, got %d", n)
		}
		if out.Len() != 0 {
			t.Fatalf("expected no output, got %q", out.String())
		}
	})
}

// TestReportOrphanTerminalDirs asserts the `sb get` warning channel: a
// one-line, non-verbose signal when orphans exist, silence when none do.
func TestReportOrphanTerminalDirs(t *testing.T) {
	ctx := context.Background()

	t.Run("warns when orphans exist", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-corrupt", "alpha", api.Exited, deadPid)
		termDir := filepath.Join(runPath, defaults.TerminalsRunPath, "id-corrupt")
		truncateMetadata(t, termDir)
		backdateDir(t, termDir)

		var out bytes.Buffer
		n, err := discovery.ReportOrphanTerminalDirs(ctx, discardLogger(), runPath, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 1 {
			t.Fatalf("expected 1 orphan, got %d", n)
		}
		if !strings.Contains(out.String(), "sb prune terminals") {
			t.Fatalf("expected warning to name the cleanup command, got %q", out.String())
		}
	})

	t.Run("silent when no orphans", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-ok", "alpha", api.Ready, os.Getpid())

		var out bytes.Buffer
		n, err := discovery.ReportOrphanTerminalDirs(ctx, discardLogger(), runPath, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 0 {
			t.Fatalf("expected 0 orphans, got %d", n)
		}
		if out.Len() != 0 {
			t.Fatalf("expected no output, got %q", out.String())
		}
	})
}
