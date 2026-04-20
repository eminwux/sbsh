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

// A doc missing apiVersion/kind/name should be skipped (logged, not errored).
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
  name: ""
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

func writeProfileFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "profiles.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write profiles: %v", err)
	}
	return p
}

func TestLoadProfilesFromPath_MultiDoc(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	path := writeProfileFile(t, multiDocProfiles)

	got, err := discovery.LoadProfilesFromPath(ctx, logger, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(got))
	}
	if got[0].Metadata.Name != "alpha" || got[1].Metadata.Name != "beta" {
		t.Fatalf("unexpected names: %q, %q", got[0].Metadata.Name, got[1].Metadata.Name)
	}
}

func TestLoadProfilesFromPath_SkipsEmptyDocs(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	path := writeProfileFile(t, profilesWithEmptyDoc)

	got, err := discovery.LoadProfilesFromPath(ctx, logger, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 profiles (empty doc skipped), got %d", len(got))
	}
	if got[0].Metadata.Name != "only" || got[1].Metadata.Name != "also" {
		t.Fatalf("unexpected names: %q, %q", got[0].Metadata.Name, got[1].Metadata.Name)
	}
}

func TestLoadProfilesFromPath_FileNotFound(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	_, err := discovery.LoadProfilesFromPath(ctx, logger, filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestFindProfileByName(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	path := writeProfileFile(t, multiDocProfiles)

	got, err := discovery.FindProfileByName(ctx, logger, path, "beta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Metadata.Name != "beta" {
		t.Fatalf("want name=beta, got %+v", got)
	}
}

func TestFindProfileByName_NotFound(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	path := writeProfileFile(t, multiDocProfiles)

	_, err := discovery.FindProfileByName(ctx, logger, path, "ghost")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("expected error to mention %q, got: %v", "ghost", err)
	}
}
