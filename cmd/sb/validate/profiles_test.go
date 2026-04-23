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

package validate

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/errdefs"
)

func writeTempProfile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp profile: %v", err)
	}
	return path
}

func runCmd(t *testing.T, args []string) (string, string, error) {
	t.Helper()
	cmd := NewValidateProfilesCmd()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx := context.WithValue(context.Background(), types.CtxLogger, logger)
	cmd.SetContext(ctx)

	var stdout, stderr bytes.Buffer
	err := runValidateProfiles(cmd, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func TestValidateProfilesCmd_NoLogger(t *testing.T) {
	cmd := NewValidateProfilesCmd()
	cmd.SetContext(context.Background())
	err := runValidateProfiles(cmd, nil, io.Discard, io.Discard)
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected ErrLoggerNotFound, got %v", err)
	}
}

func TestValidateProfilesCmd_ValidFile(t *testing.T) {
	path := writeTempProfile(t, `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: bash
spec:
  runTarget: local
  shell:
    cmd: /bin/sh
`)
	stdout, _, err := runCmd(t, []string{path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "1 profile(s): 1 valid, 0 invalid") {
		t.Fatalf("unexpected summary: %s", stdout)
	}
}

func TestValidateProfilesCmd_InvalidReturnsError(t *testing.T) {
	path := writeTempProfile(t, `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: bad
spec:
  runTarget: kubernetes
  shell:
    cmd: /bin/sh
`)
	stdout, _, err := runCmd(t, []string{path})
	if !errors.Is(err, errdefs.ErrInvalidProfiles) {
		t.Fatalf("expected ErrInvalidProfiles, got %v", err)
	}
	if !strings.Contains(stdout, "[INVALID]") {
		t.Fatalf("stdout should show INVALID tag: %s", stdout)
	}
}

func TestValidateProfilesCmd_MissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.yaml")
	_, _, err := runCmd(t, []string{missing})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidateProfilesCmd_DirWithDuplicatesAndMalformed(t *testing.T) {
	dir := t.TempDir()
	writeFile := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	writeFile("a-good.yaml", `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: dup
spec:
  runTarget: local
  shell:
    cmd: /bin/sh
`)
	writeFile("b-dup.yaml", `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: dup
spec:
  runTarget: local
  shell:
    cmd: /bin/sh
`)
	writeFile("c-bad.yaml", "apiVersion: sbsh/v1beta1\nkind: TerminalProfile\n  bad: indent\n  broken:\n")

	stdout, stderr, err := runCmd(t, []string{dir})
	if !errors.Is(err, errdefs.ErrInvalidProfiles) {
		t.Fatalf("expected ErrInvalidProfiles, got %v (stderr: %s)", err, stderr)
	}
	if !strings.Contains(stdout, "Loader warnings:") {
		t.Fatalf("expected loader warnings section in stdout, got: %s", stdout)
	}
	if !strings.Contains(stdout, "duplicate profile name") {
		t.Fatalf("expected duplicate warning, got: %s", stdout)
	}
	if !strings.Contains(stdout, "malformed YAML") {
		t.Fatalf("expected malformed YAML warning, got: %s", stdout)
	}
	if !strings.Contains(stdout, "a-good.yaml") {
		t.Fatalf("expected per-file report for a-good.yaml, got: %s", stdout)
	}
}
