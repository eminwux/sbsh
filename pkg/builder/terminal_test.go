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
	"github.com/eminwux/sbsh/pkg/api"
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

// InlineOnly: BuildTerminalSpec is the profile-free lane — the
// resulting spec carries the inline command/env verbatim and never
// touches pkg/discovery or the hardcoded "default" profile.
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

// ProfileByName: when WithProfile + WithProfilesDir resolve, the
// profile's shell cmd/args flow into the spec.
func TestBuildTerminalSpec_ProfileByName(t *testing.T) {
	runPath := t.TempDir()
	profiles := writeProfiles(t, twoProfilesYAML)

	spec, err := builder.BuildTerminalSpecFromProfile(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfilesDir(filepath.Dir(profiles)),
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

	_, err := builder.BuildTerminalSpecFromProfile(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfilesDir(filepath.Dir(profiles)),
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

// A non-default profile name that resolves to no profile at all
// (missing directory or simply absent) surfaces a profile-not-found
// error rather than silently falling back to the hardcoded default.
func TestBuildTerminalSpec_MissingProfilesDir(t *testing.T) {
	runPath := t.TempDir()
	missing := filepath.Join(runPath, "no-such-profiles-dir")

	_, err := builder.BuildTerminalSpecFromProfile(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfilesDir(missing),
		builder.WithProfile("alpha"),
	)
	if err == nil {
		t.Fatal("expected error for missing profile, got nil")
	}
	if !strings.Contains(err.Error(), "alpha") {
		t.Fatalf("expected error to mention profile name, got: %v", err)
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

// WithCwd flows through to the resulting spec. With no profile cwd,
// the inline value is what ends up on the spec.
func TestBuildTerminalSpec_WithCwd(t *testing.T) {
	runPath := t.TempDir()
	spec, err := builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithCwd("/tmp/custom-cwd"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Cwd != "/tmp/custom-cwd" {
		t.Fatalf("cwd: want /tmp/custom-cwd, got %q", spec.Cwd)
	}
}

// WithCwd overrides a profile-provided Shell.Cwd when non-empty, and
// an empty WithCwd leaves the profile value intact.
func TestBuildTerminalSpec_WithCwdOverridesProfile(t *testing.T) {
	const profilesYAML = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: cwd-profile
spec:
  runTarget: local
  shell:
    cmd: /bin/bash
    cwd: /from/profile
`
	runPath := t.TempDir()
	profiles := writeProfiles(t, profilesYAML)

	// No WithCwd: profile value sticks.
	spec, err := builder.BuildTerminalSpecFromProfile(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfilesDir(filepath.Dir(profiles)),
		builder.WithProfile("cwd-profile"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Cwd != "/from/profile" {
		t.Fatalf("cwd (profile only): want /from/profile, got %q", spec.Cwd)
	}

	// With WithCwd: inline wins.
	spec, err = builder.BuildTerminalSpecFromProfile(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfilesDir(filepath.Dir(profiles)),
		builder.WithProfile("cwd-profile"),
		builder.WithCwd("/from/option"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Cwd != "/from/option" {
		t.Fatalf("cwd (override): want /from/option, got %q", spec.Cwd)
	}
}

// WithSocketMode threads the octal mode string through to spec.SocketMode.
// Empty leaves spec.SocketMode at its zero value (runner default, 0o600).
func TestBuildTerminalSpec_WithSocketMode(t *testing.T) {
	runPath := t.TempDir()

	// Default: no WithSocketMode → zero value, runner picks 0o600.
	spec, err := builder.BuildTerminalSpec(context.Background(), testLogger(), runPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.SocketMode != 0 {
		t.Fatalf("default socketMode: want 0, got 0o%o", spec.SocketMode)
	}

	// Override: "0660" parses to 0o660.
	spec, err = builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithSocketMode("0660"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.SocketMode != 0o660 {
		t.Fatalf("socketMode: want 0o660, got 0o%o", spec.SocketMode)
	}
}

// WithSocketGID threads the explicit gid through to spec.SocketGID, and
// preserves the unset/zero distinction: never calling WithSocketGID leaves
// spec.SocketGID nil; calling WithSocketGID(0) sets *spec.SocketGID to 0.
func TestBuildTerminalSpec_WithSocketGID(t *testing.T) {
	runPath := t.TempDir()

	// Default: no WithSocketGID → nil pointer, runner leaves group unchanged.
	spec, err := builder.BuildTerminalSpec(context.Background(), testLogger(), runPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.SocketGID != nil {
		t.Fatalf("default socketGID: want nil, got %d", *spec.SocketGID)
	}

	// Explicit non-zero: gets forwarded as the same value.
	spec, err = builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithSocketGID(1234),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.SocketGID == nil {
		t.Fatalf("socketGID: want non-nil pointer to 1234, got nil")
	}
	if *spec.SocketGID != 1234 {
		t.Fatalf("socketGID: want 1234, got %d", *spec.SocketGID)
	}

	// Explicit zero: WithSocketGID(0) means "set to root", distinct from unset.
	spec, err = builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithSocketGID(0),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.SocketGID == nil {
		t.Fatalf("socketGID(0): want non-nil pointer to 0, got nil")
	}
	if *spec.SocketGID != 0 {
		t.Fatalf("socketGID(0): want 0, got %d", *spec.SocketGID)
	}
}

// WithCaptureMode threads the octal mode string through to spec.CaptureMode.
// Empty leaves spec.CaptureMode at its zero value (runner default, 0o600).
func TestBuildTerminalSpec_WithCaptureMode(t *testing.T) {
	runPath := t.TempDir()

	spec, err := builder.BuildTerminalSpec(context.Background(), testLogger(), runPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.CaptureMode != 0 {
		t.Fatalf("default captureMode: want 0, got 0o%o", spec.CaptureMode)
	}

	spec, err = builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithCaptureMode("0640"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.CaptureMode != 0o640 {
		t.Fatalf("captureMode: want 0o640, got 0o%o", spec.CaptureMode)
	}
}

// WithCaptureGID preserves the unset/zero distinction the same way
// WithSocketGID does: never calling it leaves spec.CaptureGID nil; calling
// WithCaptureGID(0) sets *spec.CaptureGID to 0.
func TestBuildTerminalSpec_WithCaptureGID(t *testing.T) {
	runPath := t.TempDir()

	spec, err := builder.BuildTerminalSpec(context.Background(), testLogger(), runPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.CaptureGID != nil {
		t.Fatalf("default captureGID: want nil, got %d", *spec.CaptureGID)
	}

	spec, err = builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithCaptureGID(1234),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.CaptureGID == nil || *spec.CaptureGID != 1234 {
		t.Fatalf("captureGID: want 1234, got %v", spec.CaptureGID)
	}

	spec, err = builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithCaptureGID(0),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.CaptureGID == nil || *spec.CaptureGID != 0 {
		t.Fatalf("captureGID(0): want 0, got %v", spec.CaptureGID)
	}
}

// WithLogFileMode threads the octal mode string through to spec.LogFileMode.
// Empty leaves spec.LogFileMode at its zero value (runner default, 0o600).
func TestBuildTerminalSpec_WithLogFileMode(t *testing.T) {
	runPath := t.TempDir()

	spec, err := builder.BuildTerminalSpec(context.Background(), testLogger(), runPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.LogFileMode != 0 {
		t.Fatalf("default logFileMode: want 0, got 0o%o", spec.LogFileMode)
	}

	spec, err = builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithLogFileMode("0640"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.LogFileMode != 0o640 {
		t.Fatalf("logFileMode: want 0o640, got 0o%o", spec.LogFileMode)
	}
}

// WithLogFileGID preserves the unset/zero distinction the same way the
// other GID options do.
func TestBuildTerminalSpec_WithLogFileGID(t *testing.T) {
	runPath := t.TempDir()

	spec, err := builder.BuildTerminalSpec(context.Background(), testLogger(), runPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.LogFileGID != nil {
		t.Fatalf("default logFileGID: want nil, got %d", *spec.LogFileGID)
	}

	spec, err = builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithLogFileGID(1234),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.LogFileGID == nil || *spec.LogFileGID != 1234 {
		t.Fatalf("logFileGID: want 1234, got %v", spec.LogFileGID)
	}

	spec, err = builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithLogFileGID(0),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.LogFileGID == nil || *spec.LogFileGID != 0 {
		t.Fatalf("logFileGID(0): want 0, got %v", spec.LogFileGID)
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

// WithStages stamps the full StagesSpec on the inline lane — no
// profile, no hardcoded default, just the bytes the caller passed.
func TestBuildTerminalSpec_WithStages_Inline(t *testing.T) {
	runPath := t.TempDir()
	stages := api.StagesSpec{
		OnInit:     []api.ExecStep{{Script: "echo init"}},
		PostAttach: []api.ExecStep{{Script: "echo attach"}},
	}
	spec, err := builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithStages(stages),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(spec.Stages, stages) {
		t.Fatalf("Stages: want %+v, got %+v", stages, spec.Stages)
	}
}

// WithOnInit / WithPostAttach populate only their respective
// sub-field on the inline lane, leaving the other side at zero.
func TestBuildTerminalSpec_WithOnInitWithPostAttach_Inline(t *testing.T) {
	runPath := t.TempDir()
	onInit := []api.ExecStep{{Script: "init"}}
	post := []api.ExecStep{{Script: "post"}}

	t.Run("OnInit only", func(t *testing.T) {
		spec, err := builder.BuildTerminalSpec(
			context.Background(),
			testLogger(),
			runPath,
			builder.WithOnInit(onInit),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(spec.Stages.OnInit, onInit) {
			t.Fatalf("OnInit: want %+v, got %+v", onInit, spec.Stages.OnInit)
		}
		if len(spec.Stages.PostAttach) != 0 {
			t.Fatalf("PostAttach: want empty, got %+v", spec.Stages.PostAttach)
		}
	})
	t.Run("PostAttach only", func(t *testing.T) {
		spec, err := builder.BuildTerminalSpec(
			context.Background(),
			testLogger(),
			runPath,
			builder.WithPostAttach(post),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(spec.Stages.PostAttach, post) {
			t.Fatalf("PostAttach: want %+v, got %+v", post, spec.Stages.PostAttach)
		}
		if len(spec.Stages.OnInit) != 0 {
			t.Fatalf("OnInit: want empty, got %+v", spec.Stages.OnInit)
		}
	})
	t.Run("OnInit + PostAttach compose", func(t *testing.T) {
		spec, err := builder.BuildTerminalSpec(
			context.Background(),
			testLogger(),
			runPath,
			builder.WithOnInit(onInit),
			builder.WithPostAttach(post),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(spec.Stages.OnInit, onInit) {
			t.Fatalf("OnInit: got %+v", spec.Stages.OnInit)
		}
		if !reflect.DeepEqual(spec.Stages.PostAttach, post) {
			t.Fatalf("PostAttach: got %+v", spec.Stages.PostAttach)
		}
	})
}

// WithPrompt populates Spec.Prompt on the inline lane.
func TestBuildTerminalSpec_WithPrompt_Inline(t *testing.T) {
	runPath := t.TempDir()
	spec, err := builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithPrompt("> "),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Prompt != "> " {
		t.Fatalf("Prompt: want %q, got %q", "> ", spec.Prompt)
	}
}

// WithEnvInherit populates Spec.EnvInherit on the inline lane, with
// the set-sentinel discipline that distinguishes "caller passed
// false" from "caller did not pass anything".
func TestBuildTerminalSpec_WithEnvInherit_Inline(t *testing.T) {
	runPath := t.TempDir()

	// Default: no WithEnvInherit → zero value (false). This is a
	// behavior change from the FromProfile lane's hardcoded-default
	// (which sets EnvInherit=true), and the contract for the inline
	// lane is "zero unless set".
	spec, err := builder.BuildTerminalSpec(context.Background(), testLogger(), runPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.EnvInherit {
		t.Fatalf("default EnvInherit: want false, got true")
	}

	// Explicit true.
	spec, err = builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithEnvInherit(true),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spec.EnvInherit {
		t.Fatalf("WithEnvInherit(true): want true, got false")
	}
}

// Inline lane must reject WithProfile / WithProfilesDir with
// errdefs.ErrInvalidOption. Silently ignoring them would hide a
// programmer bug — fail loud.
func TestBuildTerminalSpec_RejectsWithProfile(t *testing.T) {
	runPath := t.TempDir()
	_, err := builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfile("anything"),
	)
	if !errors.Is(err, errdefs.ErrInvalidOption) {
		t.Fatalf("expected errdefs.ErrInvalidOption, got %v", err)
	}
}

func TestBuildTerminalSpec_RejectsWithProfilesDir(t *testing.T) {
	runPath := t.TempDir()
	_, err := builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfilesDir("/tmp/whatever"),
	)
	if !errors.Is(err, errdefs.ErrInvalidOption) {
		t.Fatalf("expected errdefs.ErrInvalidOption, got %v", err)
	}
}

// Inline lane must not touch disk — even when the runPath-derived
// default profilesDir does not exist, the build must succeed.
// Passing WithProfilesDir is rejected upfront (see above), so the
// "did not touch disk" guarantee turns on the absence of any
// discovery call in the no-profile-option case.
func TestBuildTerminalSpec_InlineDoesNotTouchDisk(t *testing.T) {
	runPath := t.TempDir()
	// Deliberately wipe out the runPath so any opportunistic
	// directory scan would surface as an error.
	if err := os.RemoveAll(runPath); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	_, err := builder.BuildTerminalSpec(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithCommand([]string{"/bin/sh"}),
	)
	if err != nil {
		t.Fatalf("inline lane should not touch disk; got %v", err)
	}
}

// FromProfile lane must accept the new With* options as overlays on
// top of a profile-derived spec.
func TestBuildTerminalSpecFromProfile_WithStages_Overlay(t *testing.T) {
	const profilesYAML = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: stages-profile
spec:
  runTarget: local
  shell:
    cmd: /bin/bash
  stages:
    onInit:
      - script: profile-init
`
	runPath := t.TempDir()
	profiles := writeProfiles(t, profilesYAML)

	// Profile alone: OnInit comes from YAML, PostAttach is zero.
	spec, err := builder.BuildTerminalSpecFromProfile(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfilesDir(filepath.Dir(profiles)),
		builder.WithProfile("stages-profile"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(spec.Stages.OnInit) != 1 || spec.Stages.OnInit[0].Script != "profile-init" {
		t.Fatalf("profile-derived OnInit: got %+v", spec.Stages.OnInit)
	}

	// Inline override of OnInit only: PostAttach stays zero,
	// OnInit is replaced wholesale.
	spec, err = builder.BuildTerminalSpecFromProfile(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfilesDir(filepath.Dir(profiles)),
		builder.WithProfile("stages-profile"),
		builder.WithOnInit([]api.ExecStep{{Script: "inline-init"}}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(spec.Stages.OnInit) != 1 || spec.Stages.OnInit[0].Script != "inline-init" {
		t.Fatalf("overlay OnInit: got %+v", spec.Stages.OnInit)
	}

	// Inline override with WithStages: replaces both sub-fields.
	spec, err = builder.BuildTerminalSpecFromProfile(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfilesDir(filepath.Dir(profiles)),
		builder.WithProfile("stages-profile"),
		builder.WithStages(api.StagesSpec{
			OnInit:     []api.ExecStep{{Script: "new-init"}},
			PostAttach: []api.ExecStep{{Script: "new-post"}},
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(spec.Stages.OnInit) != 1 || spec.Stages.OnInit[0].Script != "new-init" {
		t.Fatalf("WithStages OnInit: got %+v", spec.Stages.OnInit)
	}
	if len(spec.Stages.PostAttach) != 1 || spec.Stages.PostAttach[0].Script != "new-post" {
		t.Fatalf("WithStages PostAttach: got %+v", spec.Stages.PostAttach)
	}
}

// FromProfile + WithPrompt overlays prompt onto the profile-derived
// value.
func TestBuildTerminalSpecFromProfile_WithPrompt_Overlay(t *testing.T) {
	const profilesYAML = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: prompt-profile
spec:
  runTarget: local
  shell:
    cmd: /bin/bash
    prompt: "(profile) "
`
	runPath := t.TempDir()
	profiles := writeProfiles(t, profilesYAML)

	// Profile alone.
	spec, err := builder.BuildTerminalSpecFromProfile(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfilesDir(filepath.Dir(profiles)),
		builder.WithProfile("prompt-profile"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Prompt != "(profile) " {
		t.Fatalf("profile Prompt: got %q", spec.Prompt)
	}

	// Inline override wins.
	spec, err = builder.BuildTerminalSpecFromProfile(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfilesDir(filepath.Dir(profiles)),
		builder.WithProfile("prompt-profile"),
		builder.WithPrompt("(inline) "),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Prompt != "(inline) " {
		t.Fatalf("inline Prompt: got %q", spec.Prompt)
	}
}

// FromProfile + WithEnvInherit overlays EnvInherit. The profile
// schema's `inheritEnv: true` is overlaid by WithEnvInherit(false)
// without losing the "did the caller say so" sentinel.
func TestBuildTerminalSpecFromProfile_WithEnvInherit_Overlay(t *testing.T) {
	const profilesYAML = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: env-profile
spec:
  runTarget: local
  shell:
    cmd: /bin/bash
    inheritEnv: true
`
	runPath := t.TempDir()
	profiles := writeProfiles(t, profilesYAML)

	// Profile alone: EnvInherit=true.
	spec, err := builder.BuildTerminalSpecFromProfile(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfilesDir(filepath.Dir(profiles)),
		builder.WithProfile("env-profile"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spec.EnvInherit {
		t.Fatalf("profile EnvInherit: want true, got false")
	}

	// Inline EnvInherit(false) wins.
	spec, err = builder.BuildTerminalSpecFromProfile(
		context.Background(),
		testLogger(),
		runPath,
		builder.WithProfilesDir(filepath.Dir(profiles)),
		builder.WithProfile("env-profile"),
		builder.WithEnvInherit(false),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.EnvInherit {
		t.Fatalf("overlay EnvInherit(false): want false, got true")
	}
}
