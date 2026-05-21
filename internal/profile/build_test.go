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
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

const buildProfileYAML = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: myprof
  labels:
    team: infra
spec:
  runTarget: local
  shell:
    cmd: /bin/zsh
    cmdArgs:
      - -l
    env:
      FOO: bar
`

func writeProfileDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.yaml"), []byte(buildProfileYAML), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	return dir
}

func TestBuildTerminalSpecFromProfile_LoadsNamedProfile(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	profilesDir := writeProfileDir(t)

	spec, err := BuildTerminalSpecFromProfile(ctx, logger, &BuildTerminalSpecParams{
		RunPath:     t.TempDir(),
		ProfilesDir: profilesDir,
		ProfileName: "myprof",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Command != "/bin/zsh" {
		t.Fatalf("expected /bin/zsh, got %q", spec.Command)
	}
	if spec.ProfileName != "myprof" {
		t.Fatalf("expected profile name myprof, got %q", spec.ProfileName)
	}
	if spec.Labels["team"] != "infra" {
		t.Fatalf("expected label team=infra, got %v", spec.Labels)
	}
	// applyParamDefaults must have populated ID/Name and artifact paths.
	if spec.ID == "" || spec.Name == "" || spec.LogFile == "" || spec.SocketFile == "" {
		t.Fatalf("expected defaults populated, got %+v", spec)
	}
}

func TestBuildTerminalSpecFromProfile_EmptyNameResolvesDefaultHardcoded(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	// Empty profilesDir + empty ProfileName -> "default" lookup misses ->
	// hardcoded default profile fallback.
	spec, err := BuildTerminalSpecFromProfile(ctx, logger, &BuildTerminalSpecParams{
		RunPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.ProfileName != "default" {
		t.Fatalf("expected default profile, got %q", spec.ProfileName)
	}
	// Hardcoded default uses the inline command default from applyParamDefaults.
	if spec.Command != "/bin/bash" {
		t.Fatalf("expected /bin/bash from hardcoded default, got %q", spec.Command)
	}
}

func TestBuildTerminalSpecFromProfile_MissingNonDefaultProfileErrors(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	profilesDir := writeProfileDir(t)

	_, err := BuildTerminalSpecFromProfile(ctx, logger, &BuildTerminalSpecParams{
		RunPath:     t.TempDir(),
		ProfilesDir: profilesDir,
		ProfileName: "does-not-exist",
	})
	if err == nil {
		t.Fatal("expected error for missing non-default profile")
	}
}

func TestBuildTerminalSpecFromProfile_AppliesOverrides(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	profilesDir := writeProfileDir(t)
	gid := 42

	spec, err := BuildTerminalSpecFromProfile(ctx, logger, &BuildTerminalSpecParams{
		RunPath:       t.TempDir(),
		ProfilesDir:   profilesDir,
		ProfileName:   "myprof",
		SocketMode:    "0660",
		SocketGID:     &gid,
		CaptureFormat: api.CaptureFormatAsciicast,
		EnvVars:       []string{"EXTRA=1"},
		Prompt:        "custom> ",
		PromptSet:     true,
		EnvInherit:    true,
		EnvInheritSet: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.SocketMode != 0o660 {
		t.Fatalf("expected socket mode 0660, got %o", spec.SocketMode)
	}
	if spec.SocketGID == nil || *spec.SocketGID != 42 {
		t.Fatalf("expected socket gid 42, got %v", spec.SocketGID)
	}
	if spec.CaptureFormat != api.CaptureFormatAsciicast {
		t.Fatalf("expected asciicast, got %q", spec.CaptureFormat)
	}
	if spec.Prompt != "custom> " {
		t.Fatalf("expected custom prompt, got %q", spec.Prompt)
	}
	if !spec.EnvInherit {
		t.Fatal("expected EnvInherit true")
	}
}

func TestBuildTerminalSpecFromProfile_InvalidCaptureFormatErrors(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	profilesDir := writeProfileDir(t)

	_, err := BuildTerminalSpecFromProfile(ctx, logger, &BuildTerminalSpecParams{
		RunPath:       t.TempDir(),
		ProfilesDir:   profilesDir,
		ProfileName:   "myprof",
		CaptureFormat: "bogus",
	})
	if !errors.Is(err, errdefs.ErrInvalidFlag) {
		t.Fatalf("expected ErrInvalidFlag, got %v", err)
	}
}

func TestBuildTerminalSpecFromProfile_InvalidSocketModeErrors(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	profilesDir := writeProfileDir(t)

	_, err := BuildTerminalSpecFromProfile(ctx, logger, &BuildTerminalSpecParams{
		RunPath:     t.TempDir(),
		ProfilesDir: profilesDir,
		ProfileName: "myprof",
		SocketMode:  "99999",
	})
	if !errors.Is(err, errdefs.ErrInvalidFlag) {
		t.Fatalf("expected ErrInvalidFlag, got %v", err)
	}
}

func TestBuildTerminalSpecInline_FullPath(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()
	gid := 7

	spec, err := BuildTerminalSpecInline(ctx, logger, &BuildTerminalSpecParams{
		RunPath:           t.TempDir(),
		TerminalCmd:       "/bin/sh",
		TerminalCmdArgs:   []string{"-c", "true"},
		Cwd:               "/tmp",
		CaptureMode:       "0640",
		CaptureGID:        &gid,
		CaptureFormat:     api.CaptureFormatRaw,
		Stages:            api.StagesSpec{OnInit: []api.ExecStep{{Script: "echo hi"}}},
		OnInitOverlay:     true,
		PostAttachOverlay: true,
		DisableSetPrompt:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Command != "/bin/sh" {
		t.Fatalf("expected /bin/sh, got %q", spec.Command)
	}
	if spec.Cwd != "/tmp" {
		t.Fatalf("expected cwd /tmp, got %q", spec.Cwd)
	}
	if spec.CaptureMode != 0o640 {
		t.Fatalf("expected capture mode 0640, got %o", spec.CaptureMode)
	}
	if spec.CaptureGID == nil || *spec.CaptureGID != 7 {
		t.Fatalf("expected capture gid 7, got %v", spec.CaptureGID)
	}
	if len(spec.Stages.OnInit) != 1 || spec.Stages.OnInit[0].Script != "echo hi" {
		t.Fatalf("expected OnInit overlay, got %v", spec.Stages.OnInit)
	}
	if spec.SetPrompt {
		t.Fatal("expected SetPrompt false when DisableSetPrompt true")
	}
}

func TestBuildTerminalSpecInline_StagesOverlayWins(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	full := api.StagesSpec{OnInit: []api.ExecStep{{Script: "a"}}, PostAttach: []api.ExecStep{{Script: "b"}}}
	spec, err := BuildTerminalSpecInline(ctx, logger, &BuildTerminalSpecParams{
		RunPath:       t.TempDir(),
		Stages:        full,
		StagesOverlay: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(spec.Stages.OnInit) != 1 || len(spec.Stages.PostAttach) != 1 {
		t.Fatalf("expected full stages overlay, got %+v", spec.Stages)
	}
}

func TestBuildTerminalSpecInline_InvalidCaptureFormatErrors(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	_, err := BuildTerminalSpecInline(ctx, logger, &BuildTerminalSpecParams{
		RunPath:       t.TempDir(),
		CaptureFormat: "nope",
	})
	if !errors.Is(err, errdefs.ErrInvalidFlag) {
		t.Fatalf("expected ErrInvalidFlag, got %v", err)
	}
}

func TestGetDefaultHardcodedProfile(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	doc, err := GetDefaultHardcodedProfile(ctx, logger, &BuildTerminalSpecParams{
		TerminalCmd:     "/bin/dash",
		TerminalCmdArgs: []string{"-i"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.Metadata.Name != "default" {
		t.Fatalf("expected default name, got %q", doc.Metadata.Name)
	}
	if doc.Spec.Shell.Cmd != "/bin/dash" {
		t.Fatalf("expected /bin/dash, got %q", doc.Spec.Shell.Cmd)
	}
	if !doc.Spec.Shell.EnvInherit {
		t.Fatal("expected EnvInherit true in hardcoded default")
	}
}

func TestCopyStringMapViaProfile(t *testing.T) {
	// copyStringMap with a non-empty map runs through CreateTerminalFromProfile;
	// an empty map returns nil.
	withLabels := &api.TerminalProfileDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminalProfile,
		Metadata:   api.TerminalProfileMetadata{Name: "p", Labels: map[string]string{"a": "b"}},
		Spec:       api.TerminalProfileSpec{Shell: api.ShellSpec{Cmd: "/bin/bash"}},
	}
	spec, err := CreateTerminalFromProfile(withLabels)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Labels["a"] != "b" {
		t.Fatalf("expected copied label, got %v", spec.Labels)
	}

	noLabels := &api.TerminalProfileDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminalProfile,
		Metadata:   api.TerminalProfileMetadata{Name: "p"},
		Spec:       api.TerminalProfileSpec{Shell: api.ShellSpec{Cmd: "/bin/bash"}},
	}
	spec, err = CreateTerminalFromProfile(noLabels)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Labels != nil {
		t.Fatalf("expected nil labels for empty map, got %v", spec.Labels)
	}
}
