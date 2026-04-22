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

package builder_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/builder"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

const twoProfilesYAML = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: alpha
spec:
  runTarget: local
  shell:
    cmd: /bin/bash
    cmdArgs: ["-l"]
    env:
      FOO: bar
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

func writeProfiles(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "profiles.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write profiles: %v", err)
	}
	return p
}

func TestBuildTerminalSpec_EmptyRunPath(t *testing.T) {
	_, err := builder.BuildTerminalSpec(context.Background(), testLogger(), "")
	if !errors.Is(err, errdefs.ErrRunPathRequired) {
		t.Fatalf("expected ErrRunPathRequired, got %v", err)
	}
}

// InlineOnly: no profile file, no profile name — falls back to the
// hardcoded "default" profile with the inline command/env.
func TestBuildTerminalSpec_InlineOnly(t *testing.T) {
	runPath := t.TempDir()
	spec, err := builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithCommand([]string{"/bin/sh", "-c", "exec bash"}),
		builder.WithEnv(map[string]string{"A": "1", "B": "2"}),
		builder.WithID("term-id"),
		builder.WithName("term-name"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Command != "/bin/sh" {
		t.Fatalf("command: want /bin/sh, got %q", spec.Command)
	}
	if !reflect.DeepEqual(spec.CommandArgs, []string{"-c", "exec bash"}) {
		t.Fatalf("args: got %v", spec.CommandArgs)
	}
	if string(spec.ID) != "term-id" {
		t.Fatalf("id: want term-id, got %q", spec.ID)
	}
	if spec.Name != "term-name" {
		t.Fatalf("name: want term-name, got %q", spec.Name)
	}
	if spec.RunPath != runPath {
		t.Fatalf("runPath: want %q, got %q", runPath, spec.RunPath)
	}
	// WithEnv entries are appended after profile env (empty for the
	// hardcoded default), in stable key order.
	wantEnv := []string{"A=1", "B=2"}
	if !reflect.DeepEqual(spec.Env, wantEnv) {
		t.Fatalf("env: want %v, got %v", wantEnv, spec.Env)
	}
}

// ProfileByName: when WithProfile + WithProfileFile resolve, the
// profile's shell cmd/args flow into the spec.
func TestBuildTerminalSpec_ProfileByName(t *testing.T) {
	runPath := t.TempDir()
	profiles := writeProfiles(t, twoProfilesYAML)

	spec, err := builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfileFile(profiles),
		builder.WithProfile("alpha"),
		builder.WithID("id-a"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Command != "/bin/bash" {
		t.Fatalf("command: want /bin/bash, got %q", spec.Command)
	}
	if !reflect.DeepEqual(spec.CommandArgs, []string{"-l"}) {
		t.Fatalf("args: got %v", spec.CommandArgs)
	}
	if spec.ProfileName != "alpha" {
		t.Fatalf("profileName: want alpha, got %q", spec.ProfileName)
	}
	// Env comes from the profile's map (sorted) plus any WithEnv adds
	// (none here).
	wantEnv := []string{"FOO=bar"}
	if !reflect.DeepEqual(spec.Env, wantEnv) {
		t.Fatalf("env: want %v, got %v", wantEnv, spec.Env)
	}
}

// UnknownProfile: a non-default profile name that is not present in
// the profiles file must surface the underlying not-found error.
func TestBuildTerminalSpec_UnknownProfile(t *testing.T) {
	runPath := t.TempDir()
	profiles := writeProfiles(t, twoProfilesYAML)

	_, err := builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfileFile(profiles),
		builder.WithProfile("ghost"),
	)
	if err == nil {
		t.Fatal("expected error for unknown profile, got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("expected error to mention profile name, got: %v", err)
	}
}

// Override order: later options win for scalar fields.
func TestBuildTerminalSpec_OptionOverrideOrder(t *testing.T) {
	runPath := t.TempDir()
	spec, err := builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithID("first"),
		builder.WithID("second"),
		builder.WithName("a"),
		builder.WithName("b"),
		builder.WithCommand([]string{"/bin/echo", "one"}),
		builder.WithCommand([]string{"/bin/sh", "-c", "exit 0"}),
		builder.WithLogLevel("info"),
		builder.WithLogLevel("debug"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(spec.ID) != "second" {
		t.Fatalf("id: want second, got %q", spec.ID)
	}
	if spec.Name != "b" {
		t.Fatalf("name: want b, got %q", spec.Name)
	}
	if spec.Command != "/bin/sh" {
		t.Fatalf("command: want /bin/sh, got %q", spec.Command)
	}
	if !reflect.DeepEqual(spec.CommandArgs, []string{"-c", "exit 0"}) {
		t.Fatalf("args: got %v", spec.CommandArgs)
	}
	if spec.LogLevel != "debug" {
		t.Fatalf("logLevel: want debug, got %q", spec.LogLevel)
	}
}

// WithEnv composes additively across calls; each call contributes
// its entries in stable key order.
func TestBuildTerminalSpec_WithEnvComposition(t *testing.T) {
	runPath := t.TempDir()
	spec, err := builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithEnv(map[string]string{"B": "2"}),
		builder.WithEnv(map[string]string{"A": "1"}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Each WithEnv call sorts its own keys, so the net order is
	// "B=2" (first call) then "A=1" (second call).
	wantEnv := []string{"B=2", "A=1"}
	if !reflect.DeepEqual(spec.Env, wantEnv) {
		t.Fatalf("env: want %v, got %v", wantEnv, spec.Env)
	}
}

// Missing profile file with a non-default profile name produces an
// open/not-found error rather than silently falling back.
func TestBuildTerminalSpec_MissingProfileFile(t *testing.T) {
	runPath := t.TempDir()
	missing := filepath.Join(runPath, "does-not-exist.yaml")

	_, err := builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfileFile(missing),
		builder.WithProfile("alpha"),
	)
	if err == nil {
		t.Fatal("expected error for missing profile file, got nil")
	}
}

// Nil options are tolerated.
func TestBuildTerminalSpec_NilOptionSafe(t *testing.T) {
	runPath := t.TempDir()
	_, err := builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		nil,
		builder.WithID("x"),
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// WithCommand with empty argv (or empty argv[0]) is a no-op.
func TestBuildTerminalSpec_WithCommandEmpty(t *testing.T) {
	runPath := t.TempDir()
	spec, err := builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithCommand(nil),
		builder.WithCommand([]string{""}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With no command set, the internal builder defaults to /bin/bash -i.
	if spec.Command != "/bin/bash" {
		t.Fatalf("command: want /bin/bash (default), got %q", spec.Command)
	}
	if !reflect.DeepEqual(spec.CommandArgs, []string{"-i"}) {
		t.Fatalf("args: got %v", spec.CommandArgs)
	}
}
