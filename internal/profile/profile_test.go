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

func TestBuildTerminalSpec_EmptyRunPath_ReturnsErrRunPathRequired(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err := BuildTerminalSpec(context.Background(), logger, &BuildTerminalSpecParams{})
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

func TestParseSocketMode(t *testing.T) {
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
			got, err := parseSocketMode(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseSocketMode(%q): want error, got mode 0o%o", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSocketMode(%q): unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("parseSocketMode(%q): want 0o%o, got 0o%o", tc.in, tc.want, got)
			}
		})
	}
}
