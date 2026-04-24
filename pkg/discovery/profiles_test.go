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
	"strings"
	"testing"

	"github.com/eminwux/sbsh/pkg/discovery"
)

const multiDocProfiles = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: alpha
spec:
  runTarget: local
  shell:
    cmd: /bin/bash
---
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: beta
spec:
  runTarget: local
  shell:
    cmd: /bin/zsh
`

// A doc missing apiVersion/kind/name should be skipped (with a warning).
const profilesWithEmptyDoc = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: only
spec:
  runTarget: local
  shell:
    cmd: /bin/bash
---
metadata:
  name: incomplete
---
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: also
spec:
  runTarget: local
  shell:
    cmd: /bin/zsh
`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir parent of %s: %v", p, err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestLoadProfilesFromDir_MultiDoc(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	dir := t.TempDir()
	writeFile(t, dir, "profiles.yaml", multiDocProfiles)

	got, warnings, err := discovery.LoadProfilesFromDir(ctx, logger, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(got))
	}
	if got[0].Metadata.Name != "alpha" || got[1].Metadata.Name != "beta" {
		t.Fatalf("unexpected names: %q, %q", got[0].Metadata.Name, got[1].Metadata.Name)
	}
}

func TestLoadProfilesFromDir_SkipsEmptyAndInvalidDocs(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	dir := t.TempDir()
	writeFile(t, dir, "profiles.yaml", profilesWithEmptyDoc)

	got, warnings, err := discovery.LoadProfilesFromDir(ctx, logger, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 profiles (invalid doc skipped), got %d", len(got))
	}
	if got[0].Metadata.Name != "only" || got[1].Metadata.Name != "also" {
		t.Fatalf("unexpected names: %q, %q", got[0].Metadata.Name, got[1].Metadata.Name)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for the incomplete doc, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0].Reason, "apiVersion") {
		t.Fatalf("expected warning to mention missing apiVersion, got %q", warnings[0].Reason)
	}
}

func TestLoadProfilesFromDir_MissingDirIsNotAnError(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	got, warnings, err := discovery.LoadProfilesFromDir(ctx, logger, missing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil || warnings != nil {
		t.Fatalf("expected (nil, nil, nil) for missing dir, got profiles=%v warnings=%v", got, warnings)
	}
}

func TestLoadProfilesFromDir_EmptyDirArgIsNotAnError(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	got, warnings, err := discovery.LoadProfilesFromDir(ctx, logger, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil || warnings != nil {
		t.Fatalf("expected (nil, nil, nil) for empty dir arg")
	}
}

func TestLoadProfilesFromDir_NotADirectory(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	dir := t.TempDir()
	file := writeFile(t, dir, "profiles.yaml", multiDocProfiles)

	_, _, err := discovery.LoadProfilesFromDir(ctx, logger, file)
	if err == nil {
		t.Fatal("expected error when path is a file not a directory")
	}
}

func TestLoadProfilesFromDir_Recursive(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	dir := t.TempDir()

	writeFile(t, dir, "top.yaml", `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: top
spec:
  runTarget: local
  shell:
    cmd: /bin/sh
`)
	writeFile(t, dir, "subdir/nested.yml", `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: nested
spec:
  runTarget: local
  shell:
    cmd: /bin/sh
`)
	// File with non-YAML extension must be ignored.
	writeFile(t, dir, "subdir/readme.txt", "ignore me")

	got, warnings, err := discovery.LoadProfilesFromDir(ctx, logger, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 profiles, got %d: %+v", len(got), got)
	}
	names := []string{got[0].Metadata.Name, got[1].Metadata.Name}
	if names[0] != "nested" || names[1] != "top" {
		t.Fatalf("expected deterministic order [nested, top], got %v", names)
	}
}

func TestLoadProfilesFromDir_MalformedYAMLProducesWarning(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	dir := t.TempDir()

	writeFile(t, dir, "bad.yaml", "apiVersion: sbsh/v1beta1\nkind: TerminalProfile\n  oops this is broken:\n    -\n")
	writeFile(t, dir, "good.yaml", `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: good
spec:
  runTarget: local
  shell:
    cmd: /bin/sh
`)

	got, warnings, err := discovery.LoadProfilesFromDir(ctx, logger, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Metadata.Name != "good" {
		t.Fatalf("expected the good profile to load, got %+v", got)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected exactly 1 warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0].Reason, "malformed YAML") {
		t.Fatalf("expected malformed YAML warning, got %q", warnings[0].Reason)
	}
	if !strings.HasSuffix(warnings[0].File, "bad.yaml") {
		t.Fatalf("expected warning to point at bad.yaml, got %s", warnings[0].File)
	}
}

func TestLoadProfilesFromDir_DuplicateNameFirstWins(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	dir := t.TempDir()

	// Lexicographic ordering picks a-first, b-second.
	writeFile(t, dir, "a-first.yaml", `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: dup
spec:
  runTarget: local
  shell:
    cmd: /bin/bash
`)
	writeFile(t, dir, "b-second.yaml", `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: dup
spec:
  runTarget: local
  shell:
    cmd: /bin/zsh
`)

	got, warnings, err := discovery.LoadProfilesFromDir(ctx, logger, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected only first-wins profile, got %d", len(got))
	}
	if got[0].Spec.Shell.Cmd != "/bin/bash" {
		t.Fatalf("expected first file's shell to win, got %q", got[0].Spec.Shell.Cmd)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected one duplicate warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0].Reason, "duplicate") {
		t.Fatalf("expected warning to mention duplicate, got %q", warnings[0].Reason)
	}
	if !strings.HasSuffix(warnings[0].File, "b-second.yaml") {
		t.Fatalf("expected warning to name the skipped file, got %s", warnings[0].File)
	}
	if !strings.Contains(warnings[0].Reason, "a-first.yaml") {
		t.Fatalf("expected warning to name the winning file, got %q", warnings[0].Reason)
	}
}

func TestLoadProfilesFromDir_UnsupportedKindProducesWarning(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	dir := t.TempDir()
	writeFile(t, dir, "p.yaml", `apiVersion: sbsh/v1beta1
kind: Something
metadata:
  name: wrong-kind
spec:
  runTarget: local
  shell:
    cmd: /bin/sh
`)

	got, warnings, err := discovery.LoadProfilesFromDir(ctx, logger, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no profiles, got %d", len(got))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Reason, "kind") {
		t.Fatalf("expected single kind warning, got %v", warnings)
	}
}

func TestFindProfileByNameInDir(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	dir := t.TempDir()
	writeFile(t, dir, "profiles.yaml", multiDocProfiles)

	got, warnings, err := discovery.FindProfileByNameInDir(ctx, logger, dir, "beta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if got == nil || got.Metadata.Name != "beta" {
		t.Fatalf("want name=beta, got %+v", got)
	}
}

func TestFindProfileByNameInDir_NotFound(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	dir := t.TempDir()
	writeFile(t, dir, "profiles.yaml", multiDocProfiles)

	_, _, err := discovery.FindProfileByNameInDir(ctx, logger, dir, "ghost")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("expected error to mention %q, got: %v", "ghost", err)
	}
}
