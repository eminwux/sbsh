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
	"strings"
	"testing"

	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
)

func TestBuildTerminalSpecFromProfile_EmptyRunPath_ReturnsErrRunPathRequired(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err := BuildTerminalSpecFromProfile(context.Background(), logger, &BuildTerminalSpecParams{})
	if !errors.Is(err, errdefs.ErrRunPathRequired) {
		t.Fatalf("expected errdefs.ErrRunPathRequired, got %v", err)
	}
}

func TestBuildTerminalSpecInline_EmptyRunPath_ReturnsErrRunPathRequired(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err := BuildTerminalSpecInline(context.Background(), logger, &BuildTerminalSpecParams{})
	if !errors.Is(err, errdefs.ErrRunPathRequired) {
		t.Fatalf("expected errdefs.ErrRunPathRequired, got %v", err)
	}
}

func profileWithSocketGID(name string, gid *int) *api.TerminalProfileDoc {
	return &api.TerminalProfileDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminalProfile,
		Metadata:   api.TerminalProfileMetadata{Name: name},
		Spec: api.TerminalProfileSpec{
			RunTarget: api.RunTargetLocal,
			Shell:     api.ShellSpec{Cmd: "/bin/bash"},
			Socket:    &api.SocketSpec{GID: gid},
		},
	}
}

func TestCreateTerminalFromProfile_RejectsNegativeGID(t *testing.T) {
	gid := -5
	_, err := CreateTerminalFromProfile(profileWithSocketGID("tprof", &gid))
	if err == nil {
		t.Fatal("expected error for negative gid, got nil")
	}
	if !strings.Contains(err.Error(), "spec.socket.gid") {
		t.Fatalf("error %q should mention spec.socket.gid", err.Error())
	}
	if !strings.Contains(err.Error(), "tprof") {
		t.Fatalf("error %q should mention profile name", err.Error())
	}
}

func TestCreateTerminalFromProfile_AcceptsValidGID(t *testing.T) {
	gidZero := 0
	gidPos := 1234
	tests := []struct {
		name string
		gid  *int
	}{
		{name: "nil leaves group unchanged", gid: nil},
		{name: "zero is a valid root gid", gid: &gidZero},
		{name: "positive gid", gid: &gidPos},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := CreateTerminalFromProfile(profileWithSocketGID("tprof", tc.gid))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gotNil := spec.SocketGID == nil
			wantNil := tc.gid == nil
			if gotNil != wantNil {
				t.Fatalf("SocketGID nil=%v, want nil=%v", gotNil, wantNil)
			}
			if !wantNil && *spec.SocketGID != *tc.gid {
				t.Fatalf("SocketGID = %d, want %d", *spec.SocketGID, *tc.gid)
			}
		})
	}
}

func TestParseFileMode(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    os.FileMode
		wantErr bool
	}{
		{name: "leading-zero octal", in: "0660", want: 0o660},
		{name: "no leading zero", in: "660", want: 0o660},
		{name: "owner-only", in: "0600", want: 0o600},
		{name: "world-writable", in: "0666", want: 0o666},
		{name: "setgid bit", in: "2660", want: 0o2660},
		{name: "full mask", in: "7777", want: 0o7777},
		{name: "zero", in: "0", want: 0},
		{name: "out of mask", in: "10000", wantErr: true},
		{name: "non-octal digit", in: "0680", wantErr: true},
		{name: "non-numeric", in: "rw-", wantErr: true},
		{name: "empty", in: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseFileMode(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseFileMode(%q): want error, got mode 0o%o", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFileMode(%q): unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("parseFileMode(%q): want 0o%o, got 0o%o", tc.in, tc.want, got)
			}
		})
	}
}

// CreateTerminalFromProfile must reject negative gids in the capture or
// logFile permission blocks the same way it does for the socket block. The
// error messages must name the specific spec.<block>.gid field so the
// caller can locate the offending YAML key without grepping.
func TestCreateTerminalFromProfile_RejectsNegativeArtifactGID(t *testing.T) {
	bad := -1
	tests := []struct {
		name      string
		mutate    func(p *api.TerminalProfileDoc)
		fieldPath string
	}{
		{
			name: "capture",
			mutate: func(p *api.TerminalProfileDoc) {
				p.Spec.Capture = &api.FilePermSpec{GID: &bad}
			},
			fieldPath: "spec.capture.gid",
		},
		{
			name: "logFile",
			mutate: func(p *api.TerminalProfileDoc) {
				p.Spec.LogFile = &api.FilePermSpec{GID: &bad}
			},
			fieldPath: "spec.logFile.gid",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &api.TerminalProfileDoc{
				APIVersion: api.APIVersionV1Beta1,
				Kind:       api.KindTerminalProfile,
				Metadata:   api.TerminalProfileMetadata{Name: "tprof"},
				Spec: api.TerminalProfileSpec{
					RunTarget: api.RunTargetLocal,
					Shell:     api.ShellSpec{Cmd: "/bin/bash"},
				},
			}
			tc.mutate(p)
			_, err := CreateTerminalFromProfile(p)
			if err == nil {
				t.Fatalf("expected error for negative gid in %s", tc.fieldPath)
			}
			if !strings.Contains(err.Error(), tc.fieldPath) {
				t.Fatalf("error %q should mention %s", err.Error(), tc.fieldPath)
			}
			if !strings.Contains(err.Error(), "tprof") {
				t.Fatalf("error %q should mention profile name", err.Error())
			}
		})
	}
}

// A profile that supplies capture/logFile mode + gid must thread both into
// the produced TerminalSpec verbatim. The pointer-vs-sentinel discipline
// (nil = unset, *gid = explicit) must survive the copy.
func TestCreateTerminalFromProfile_AppliesArtifactPermBlocks(t *testing.T) {
	gidCapture := 1001
	gidLog := 2002
	p := &api.TerminalProfileDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminalProfile,
		Metadata:   api.TerminalProfileMetadata{Name: "tprof"},
		Spec: api.TerminalProfileSpec{
			RunTarget: api.RunTargetLocal,
			Shell:     api.ShellSpec{Cmd: "/bin/bash"},
			Capture:   &api.FilePermSpec{Mode: "0640", GID: &gidCapture},
			LogFile:   &api.FilePermSpec{Mode: "0660", GID: &gidLog},
		},
	}
	spec, err := CreateTerminalFromProfile(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.CaptureMode != 0o640 {
		t.Fatalf("CaptureMode: want 0o640, got 0o%o", spec.CaptureMode)
	}
	if spec.CaptureGID == nil || *spec.CaptureGID != gidCapture {
		t.Fatalf("CaptureGID: want %d, got %v", gidCapture, spec.CaptureGID)
	}
	if spec.LogFileMode != 0o660 {
		t.Fatalf("LogFileMode: want 0o660, got 0o%o", spec.LogFileMode)
	}
	if spec.LogFileGID == nil || *spec.LogFileGID != gidLog {
		t.Fatalf("LogFileGID: want %d, got %v", gidLog, spec.LogFileGID)
	}
}

// Omitting the capture/logFile blocks must leave the spec at the runner
// defaults: zero-valued FileMode (the runner falls back to 0o600) and a
// nil gid pointer (the runner leaves the group unchanged).
func TestCreateTerminalFromProfile_OmittedArtifactBlocksLeaveDefaults(t *testing.T) {
	p := &api.TerminalProfileDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindTerminalProfile,
		Metadata:   api.TerminalProfileMetadata{Name: "tprof"},
		Spec: api.TerminalProfileSpec{
			RunTarget: api.RunTargetLocal,
			Shell:     api.ShellSpec{Cmd: "/bin/bash"},
		},
	}
	spec, err := CreateTerminalFromProfile(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.CaptureMode != 0 {
		t.Fatalf("CaptureMode: want 0 (runner default), got 0o%o", spec.CaptureMode)
	}
	if spec.CaptureGID != nil {
		t.Fatalf("CaptureGID: want nil, got %d", *spec.CaptureGID)
	}
	if spec.LogFileMode != 0 {
		t.Fatalf("LogFileMode: want 0 (runner default), got 0o%o", spec.LogFileMode)
	}
	if spec.LogFileGID != nil {
		t.Fatalf("LogFileGID: want nil, got %d", *spec.LogFileGID)
	}
}

// applyOnePermOverride: an empty rawMode and nil gid leave prior values
// untouched, so profile-resolved values survive when the caller passes
// nothing.
func TestApplyOnePermOverride_NoOverrideKeepsPrior(t *testing.T) {
	gid7 := 7
	mode := os.FileMode(0o640)
	gid := &gid7
	if err := applyOnePermOverride("", nil, "capture", &mode, &gid); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != 0o640 {
		t.Fatalf("mode: want 0o640, got 0o%o", mode)
	}
	if gid == nil || *gid != 7 {
		t.Fatalf("gid: want 7, got %v", gid)
	}
}

// applyOnePermOverride: a non-empty mode string parses and overwrites the
// prior value.
func TestApplyOnePermOverride_ExplicitModeReplacesPrior(t *testing.T) {
	mode := os.FileMode(0)
	var gid *int
	if err := applyOnePermOverride("0660", nil, "capture", &mode, &gid); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != 0o660 {
		t.Fatalf("mode: want 0o660, got 0o%o", mode)
	}
	if gid != nil {
		t.Fatalf("gid: want nil, got %d", *gid)
	}
}

// applyOnePermOverride: a non-nil gid pointer overwrites with the
// dereferenced value, including the zero value (root).
func TestApplyOnePermOverride_ExplicitGIDReplacesPrior(t *testing.T) {
	gid7 := 7
	mode := os.FileMode(0)
	var gid *int
	if err := applyOnePermOverride("", &gid7, "capture", &mode, &gid); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gid == nil || *gid != 7 {
		t.Fatalf("gid: want 7, got %v", gid)
	}
}

// applyOnePermOverride: an invalid octal mode surfaces an
// ErrInvalidFlag-wrapped error so callers can distinguish bad-input from
// internal failures.
func TestApplyOnePermOverride_InvalidModeSurfacesErrInvalidFlag(t *testing.T) {
	mode := os.FileMode(0)
	var gid *int
	err := applyOnePermOverride("rw-", nil, "capture", &mode, &gid)
	if err == nil {
		t.Fatal("expected error for invalid mode, got nil")
	}
	if !errors.Is(err, errdefs.ErrInvalidFlag) {
		t.Fatalf("error %v should wrap errdefs.ErrInvalidFlag", err)
	}
}
