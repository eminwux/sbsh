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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/pkg/api"
)

const sampleProfile = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: alpha
spec:
  runTarget: local
  shell:
    cmd: /bin/bash
    cmdArgs:
      - -l
    env:
      FOO: bar
`

func writeProfileFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// writeNamedClientDoc writes a client metadata.json with a Metadata.Name set,
// which client lookups resolve against (writeClientDoc leaves the name empty).
func writeNamedClientDoc(t *testing.T, runPath, id, name string, state api.ClientStatusMode, pid int) {
	t.Helper()
	dir := filepath.Join(runPath, defaults.ClientsRunPath, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	doc := api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata:   api.ClientMetadata{Name: name},
		Spec:       api.ClientSpec{ID: api.ID(id), RunPath: runPath},
		Status: api.ClientStatus{
			State:         state,
			Pid:           pid,
			ClientRunPath: dir,
		},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if writeErr := os.WriteFile(filepath.Join(dir, "metadata.json"), b, 0o644); writeErr != nil {
		t.Fatalf("write metadata.json: %v", writeErr)
	}
}

func TestScanAndPrintTerminals(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()
	livePid := os.Getpid()

	t.Run("empty runPath prints placeholder", func(t *testing.T) {
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintTerminals(ctx, logger, t.TempDir(), &buf, false, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "no active or inactive terminals found") {
			t.Fatalf("expected placeholder, got %q", buf.String())
		}
	})

	t.Run("compact table lists active terminal", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-1", "alpha", api.Ready, livePid)
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintTerminals(ctx, logger, runPath, &buf, false, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "NAME") || !strings.Contains(out, "alpha") {
			t.Fatalf("expected header and alpha row, got %q", out)
		}
	})

	t.Run("wide table includes ID and labels columns", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-1", "alpha", api.Ready, livePid)
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintTerminals(ctx, logger, runPath, &buf, true, "wide"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "ID") || !strings.Contains(out, "LABELS") || !strings.Contains(out, "id-1") {
			t.Fatalf("expected wide header with id-1, got %q", out)
		}
	})

	t.Run("only exited terminals without printAll", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-1", "alpha", api.Exited, livePid)
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintTerminals(ctx, logger, runPath, &buf, false, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "no active terminals found") {
			t.Fatalf("expected no-active message, got %q", buf.String())
		}
	})
}

func TestScanAndPrintTerminals_Formats(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()
	livePid := os.Getpid()

	t.Run("json format", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-1", "alpha", api.Ready, livePid)
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintTerminals(ctx, logger, runPath, &buf, true, "json"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "\"id-1\"") {
			t.Fatalf("expected json with id-1, got %q", buf.String())
		}
	})

	t.Run("yaml format", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-1", "alpha", api.Ready, livePid)
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintTerminals(ctx, logger, runPath, &buf, true, "yaml"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "id-1") {
			t.Fatalf("expected yaml with id-1, got %q", buf.String())
		}
	})

	t.Run("unknown format errors", func(t *testing.T) {
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintTerminals(ctx, logger, t.TempDir(), &buf, false, "bogus"); err == nil {
			t.Fatal("expected error for unknown format")
		}
	})
}

func TestScanAndPrintClients(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()
	livePid := os.Getpid()

	t.Run("empty prints placeholder", func(t *testing.T) {
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintClients(ctx, logger, t.TempDir(), &buf, false, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "no active or inactive clients found") {
			t.Fatalf("expected placeholder, got %q", buf.String())
		}
	})

	t.Run("wide table lists active client", func(t *testing.T) {
		runPath := t.TempDir()
		writeClientDoc(t, runPath, "id-1", api.ClientReady, livePid)
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintClients(ctx, logger, runPath, &buf, true, "wide"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "ID") || !strings.Contains(out, "LABELS") || !strings.Contains(out, "id-1") {
			t.Fatalf("expected wide header with id-1, got %q", out)
		}
	})

	t.Run("only exited clients without printAll", func(t *testing.T) {
		runPath := t.TempDir()
		writeClientDoc(t, runPath, "id-1", api.ClientExited, livePid)
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintClients(ctx, logger, runPath, &buf, false, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "no active clients found") {
			t.Fatalf("expected no-active message, got %q", buf.String())
		}
	})

	t.Run("json format", func(t *testing.T) {
		runPath := t.TempDir()
		writeClientDoc(t, runPath, "id-1", api.ClientReady, livePid)
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintClients(ctx, logger, runPath, &buf, true, "json"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "\"id-1\"") {
			t.Fatalf("expected json with id-1, got %q", buf.String())
		}
	})

	t.Run("unknown format errors", func(t *testing.T) {
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintClients(ctx, logger, t.TempDir(), &buf, false, "bogus"); err == nil {
			t.Fatal("expected error for unknown format")
		}
	})
}

func TestFindAndPrintTerminalMetadata(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()
	livePid := os.Getpid()

	t.Run("found prints metadata", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-1", "alpha", api.Ready, livePid)
		var buf bytes.Buffer
		if err := discovery.FindAndPrintTerminalMetadata(ctx, logger, runPath, &buf, "alpha", "json"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "id-1") {
			t.Fatalf("expected metadata with id-1, got %q", buf.String())
		}
	})

	t.Run("not found returns error", func(t *testing.T) {
		runPath := t.TempDir()
		var buf bytes.Buffer
		if err := discovery.FindAndPrintTerminalMetadata(ctx, logger, runPath, &buf, "missing", "json"); err == nil {
			t.Fatal("expected error for missing terminal")
		}
	})

	t.Run("human format", func(t *testing.T) {
		runPath := t.TempDir()
		writeTerminalDoc(t, runPath, "id-1", "alpha", api.Ready, livePid)
		var buf bytes.Buffer
		if err := discovery.FindAndPrintTerminalMetadata(ctx, logger, runPath, &buf, "alpha", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if buf.Len() == 0 {
			t.Fatal("expected human output, got empty")
		}
	})
}

func TestFindAndPrintClientMetadata(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()
	livePid := os.Getpid()

	runPath := t.TempDir()
	writeNamedClientDoc(t, runPath, "id-1", "alpha", api.ClientReady, livePid)
	var buf bytes.Buffer
	if err := discovery.FindAndPrintClientMetadata(ctx, logger, runPath, &buf, "alpha", "json"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "id-1") {
		t.Fatalf("expected metadata with id-1, got %q", buf.String())
	}
}

func TestFindTerminalByIDAndName(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()
	runPath := t.TempDir()
	writeTerminalDoc(t, runPath, "id-1", "alpha", api.Ready, os.Getpid())

	got, err := discovery.FindTerminalByID(ctx, logger, runPath, "id-1")
	if err != nil {
		t.Fatalf("FindTerminalByID: %v", err)
	}
	if got == nil || string(got.Spec.ID) != "id-1" {
		t.Fatalf("expected id-1, got %+v", got)
	}

	got, err = discovery.FindTerminalByName(ctx, logger, runPath, "alpha")
	if err != nil {
		t.Fatalf("FindTerminalByName: %v", err)
	}
	if got == nil || got.Spec.Name != "alpha" {
		t.Fatalf("expected alpha, got %+v", got)
	}
}

func TestFindClientByName(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()
	runPath := t.TempDir()
	writeNamedClientDoc(t, runPath, "id-1", "alpha", api.ClientReady, os.Getpid())

	got, err := discovery.FindClientByName(ctx, logger, runPath, "alpha")
	if err != nil {
		t.Fatalf("FindClientByName: %v", err)
	}
	if got == nil || got.Metadata.Name != "alpha" {
		t.Fatalf("expected alpha, got %+v", got)
	}

	if _, missErr := discovery.FindClientByName(ctx, logger, runPath, "nonexistent"); missErr == nil {
		t.Fatal("expected error for unknown client name")
	}
}

func TestPrintTerminalSpec(t *testing.T) {
	logger := discardLogger()

	t.Run("nil spec is a no-op", func(t *testing.T) {
		if err := discovery.PrintTerminalSpec(nil, logger); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("fully populated spec", func(t *testing.T) {
		spec := &api.TerminalSpec{
			ID:          "id-1",
			Name:        "alpha",
			Kind:        api.TerminalLocal,
			Command:     "/bin/bash",
			CommandArgs: []string{"-l"},
			Prompt:      "$ ",
			RunPath:     "/tmp/run",
			CaptureFile: "/tmp/cap",
			LogFile:     "/tmp/log",
			LogLevel:    "debug",
			SocketFile:  "/tmp/sock",
			Env:         []string{"FOO=bar"},
			Labels:      map[string]string{"team": "infra", "env": "prod"},
		}
		if err := discovery.PrintTerminalSpec(spec, logger); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestScanAndPrintProfiles(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()

	t.Run("empty dir prints no-profiles", func(t *testing.T) {
		var buf, warn bytes.Buffer
		if err := discovery.ScanAndPrintProfiles(ctx, logger, t.TempDir(), &buf, &warn, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "no profiles found") {
			t.Fatalf("expected no-profiles message, got %q", buf.String())
		}
	})

	t.Run("compact table", func(t *testing.T) {
		dir := t.TempDir()
		writeProfileFile(t, dir, "alpha.yaml", sampleProfile)
		var buf, warn bytes.Buffer
		if err := discovery.ScanAndPrintProfiles(ctx, logger, dir, &buf, &warn, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "alpha") || !strings.Contains(out, "/bin/bash") {
			t.Fatalf("expected alpha profile row, got %q", out)
		}
	})

	t.Run("wide table includes target and envvars", func(t *testing.T) {
		dir := t.TempDir()
		writeProfileFile(t, dir, "alpha.yaml", sampleProfile)
		var buf, warn bytes.Buffer
		if err := discovery.ScanAndPrintProfiles(ctx, logger, dir, &buf, &warn, "wide"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "TARGET") || !strings.Contains(out, "vars") {
			t.Fatalf("expected wide header, got %q", out)
		}
	})
}

func TestScanAndPrintProfiles_FormatsAndWarnings(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()

	t.Run("json format", func(t *testing.T) {
		dir := t.TempDir()
		writeProfileFile(t, dir, "alpha.yaml", sampleProfile)
		var buf, warn bytes.Buffer
		if err := discovery.ScanAndPrintProfiles(ctx, logger, dir, &buf, &warn, "json"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "alpha") {
			t.Fatalf("expected json with alpha, got %q", buf.String())
		}
	})

	t.Run("unknown format errors", func(t *testing.T) {
		var buf, warn bytes.Buffer
		if err := discovery.ScanAndPrintProfiles(ctx, logger, t.TempDir(), &buf, &warn, "bogus"); err == nil {
			t.Fatal("expected error for unknown format")
		}
	})

	t.Run("warnings written to warnOut", func(t *testing.T) {
		dir := t.TempDir()
		// Missing apiVersion produces a warning but does not abort.
		writeProfileFile(t, dir, "bad.yaml", "kind: TerminalProfile\nmetadata:\n  name: bad\n")
		var buf, warn bytes.Buffer
		if err := discovery.ScanAndPrintProfiles(ctx, logger, dir, &buf, &warn, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(warn.String(), "sbsh: warning:") {
			t.Fatalf("expected warning line, got %q", warn.String())
		}
	})
}

func TestFindAndPrintProfileMetadata(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()

	t.Run("found", func(t *testing.T) {
		dir := t.TempDir()
		writeProfileFile(t, dir, "alpha.yaml", sampleProfile)
		var buf, warn bytes.Buffer
		if err := discovery.FindAndPrintProfileMetadata(ctx, logger, dir, &buf, &warn, "alpha", "json"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "alpha") {
			t.Fatalf("expected alpha in output, got %q", buf.String())
		}
	})

	t.Run("not found errors", func(t *testing.T) {
		dir := t.TempDir()
		writeProfileFile(t, dir, "alpha.yaml", sampleProfile)
		var buf, warn bytes.Buffer
		if err := discovery.FindAndPrintProfileMetadata(ctx, logger, dir, &buf, &warn, "missing", "json"); err == nil {
			t.Fatal("expected error for missing profile")
		}
	})
}

func TestLoadProfilesWrappers(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()

	t.Run("LoadProfilesFromDir", func(t *testing.T) {
		dir := t.TempDir()
		writeProfileFile(t, dir, "alpha.yaml", sampleProfile)
		profiles, warnings, err := discovery.LoadProfilesFromDir(ctx, logger, dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(profiles) != 1 || profiles[0].Metadata.Name != "alpha" {
			t.Fatalf("expected one alpha profile, got %+v", profiles)
		}
		if len(warnings) != 0 {
			t.Fatalf("expected no warnings, got %+v", warnings)
		}
	})

	t.Run("LoadProfilesFromReaderWithContext", func(t *testing.T) {
		profiles, err := discovery.LoadProfilesFromReaderWithContext(ctx, logger, strings.NewReader(sampleProfile))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(profiles) != 1 || profiles[0].Metadata.Name != "alpha" {
			t.Fatalf("expected one alpha profile, got %+v", profiles)
		}
	})

	t.Run("FindProfileByNameInDir", func(t *testing.T) {
		dir := t.TempDir()
		writeProfileFile(t, dir, "alpha.yaml", sampleProfile)
		doc, _, err := discovery.FindProfileByNameInDir(ctx, logger, dir, "alpha")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if doc == nil || doc.Metadata.Name != "alpha" {
			t.Fatalf("expected alpha, got %+v", doc)
		}
	})
}

func TestPrintProfileWarnings(t *testing.T) {
	t.Run("nil writer is a no-op", func(_ *testing.T) {
		discovery.PrintProfileWarnings(nil, []discovery.ProfileWarning{{File: "x", Reason: "y"}})
	})

	t.Run("writes one line per warning", func(t *testing.T) {
		var buf bytes.Buffer
		discovery.PrintProfileWarnings(&buf, []discovery.ProfileWarning{
			{File: "a.yaml", Reason: "boom"},
			{File: "b.yaml", DocIndex: 2, Reason: "bang"},
		})
		out := buf.String()
		if strings.Count(out, "sbsh: warning:") != 2 {
			t.Fatalf("expected two warning lines, got %q", out)
		}
	})
}

func TestPrintHuman(t *testing.T) {
	type inner struct {
		Name string
		Tags []string
	}
	type outer struct {
		Title  string
		Count  int
		Active bool
		Nested inner
		Labels map[string]string
		Empty  []string
		Ptr    *inner
		NilPtr *inner
	}
	v := outer{
		Title:  "  hello  ",
		Count:  3,
		Active: true,
		Nested: inner{Name: "n", Tags: []string{"a", "b"}},
		Labels: map[string]string{"k": "v"},
		Empty:  nil,
		Ptr:    &inner{Name: "p"},
		NilPtr: nil,
	}
	var buf bytes.Buffer
	discovery.PrintHuman(&buf, v, "")
	out := buf.String()
	for _, want := range []string{"Title", "hello", "Count", "3", "Active", "true", "Nested", "Labels", "k:", "[]", "<nil>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got %q", want, out)
		}
	}

	t.Run("nil interface", func(t *testing.T) {
		var b bytes.Buffer
		discovery.PrintHuman(&b, nil, "")
		if !strings.Contains(b.String(), "<nil>") {
			t.Fatalf("expected <nil>, got %q", b.String())
		}
	})

	t.Run("empty string renders quotes", func(t *testing.T) {
		var b bytes.Buffer
		discovery.PrintHuman(&b, struct{ S string }{S: "   "}, "")
		if !strings.Contains(b.String(), "\"\"") {
			t.Fatalf("expected empty-string marker, got %q", b.String())
		}
	})
}
