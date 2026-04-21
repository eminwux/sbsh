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

package profile

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestValidateProfilesFromReader_Valid(t *testing.T) {
	const doc = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: bash
spec:
  runTarget: local
  shell:
    cmd: /bin/sh
`
	results, err := ValidateProfilesFromReader(context.Background(), discardLogger(), strings.NewReader(doc))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if !results[0].OK() {
		t.Fatalf("expected OK result, got errors: %v", results[0].Errors)
	}
	if results[0].ProfileName != "bash" {
		t.Fatalf("want profile name 'bash', got %q", results[0].ProfileName)
	}
}

func TestValidateProfilesFromReader_UnknownFieldRejected(t *testing.T) {
	const doc = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: bash
spec:
  runTarget: local
  shell:
    cmd: /bin/sh
    unknownField: oops
`
	results, err := ValidateProfilesFromReader(context.Background(), discardLogger(), strings.NewReader(doc))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].OK() {
		t.Fatalf("expected failure for unknown field, got OK")
	}
	joined := joinErrors(results[0].Errors)
	if !strings.Contains(joined, "unknownField") {
		t.Fatalf("expected error to mention unknownField, got: %s", joined)
	}
}

func TestValidateProfilesFromReader_BadEnums(t *testing.T) {
	const doc = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: bogus-enums
spec:
  runTarget: kubernetes
  restartPolicy: restart-sometimes
  shell:
    cmd: /bin/sh
`
	results, err := ValidateProfilesFromReader(context.Background(), discardLogger(), strings.NewReader(doc))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].OK() {
		t.Fatalf("expected enum validation failure")
	}
	joined := joinErrors(results[0].Errors)
	if !strings.Contains(joined, "runTarget") {
		t.Fatalf("expected runTarget error, got: %s", joined)
	}
	if !strings.Contains(joined, "restartPolicy") {
		t.Fatalf("expected restartPolicy error, got: %s", joined)
	}
}

func TestValidateProfilesFromReader_CmdNotInPath(t *testing.T) {
	const doc = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: bad-cmd
spec:
  runTarget: local
  shell:
    cmd: definitely-not-a-real-binary-sbsh
`
	results, err := ValidateProfilesFromReader(context.Background(), discardLogger(), strings.NewReader(doc))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].OK() {
		t.Fatalf("expected PATH lookup to fail")
	}
	joined := joinErrors(results[0].Errors)
	if !strings.Contains(joined, "shell.cmd") {
		t.Fatalf("expected shell.cmd error, got: %s", joined)
	}
}

func TestValidateProfilesFromReader_MissingRequired(t *testing.T) {
	const doc = `apiVersion: ""
kind: ""
metadata:
  name: ""
spec:
  shell:
    cmd: ""
`
	results, err := ValidateProfilesFromReader(context.Background(), discardLogger(), strings.NewReader(doc))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	joined := joinErrors(results[0].Errors)
	for _, want := range []string{"apiVersion", "kind", "metadata.name", "spec.shell.cmd"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected error mentioning %q, got: %s", want, joined)
		}
	}
}

func TestValidateProfilesFromReader_MultiDocMixed(t *testing.T) {
	const doc = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: good
spec:
  runTarget: local
  shell:
    cmd: /bin/sh
---
apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: bad
spec:
  runTarget: kubernetes
  shell:
    cmd: /bin/sh
`
	results, err := ValidateProfilesFromReader(context.Background(), discardLogger(), strings.NewReader(doc))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	if !results[0].OK() {
		t.Fatalf("first doc expected OK, got: %v", results[0].Errors)
	}
	if results[1].OK() {
		t.Fatalf("second doc expected invalid")
	}
}

func TestValidateProfilesFromPath_FileNotFound(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	_, err := ValidateProfilesFromPath(context.Background(), discardLogger(), missing)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func joinErrors(errs []error) string {
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, e.Error())
	}
	return strings.Join(parts, " | ")
}
