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
	"testing"

	"github.com/eminwux/sbsh/internal/errdefs"
)

func TestBuildTerminalSpec_EmptyRunPath_ReturnsErrRunPathRequired(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err := BuildTerminalSpec(context.Background(), logger, &BuildTerminalSpecParams{})
	if !errors.Is(err, errdefs.ErrRunPathRequired) {
		t.Fatalf("expected errdefs.ErrRunPathRequired, got %v", err)
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
