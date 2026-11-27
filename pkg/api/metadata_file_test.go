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

package api

import "testing"

func TestResolveMetadataFile(t *testing.T) {
	tests := []struct {
		name    string
		spec    *TerminalSpec
		want    string
		wantErr bool
	}{
		{name: "nil spec", spec: nil, want: DefaultMetadataFile},
		{name: "empty means default", spec: &TerminalSpec{}, want: DefaultMetadataFile},
		{
			name: "explicit default without dir",
			spec: &TerminalSpec{MetadataFile: "metadata.json"},
			want: DefaultMetadataFile,
		},
		{
			name: "custom with dir",
			spec: &TerminalSpec{MetadataFile: "terminal.json", MetadataDir: "/run/x"},
			want: "terminal.json",
		},
		{name: "custom without dir", spec: &TerminalSpec{MetadataFile: "terminal.json"}, wantErr: true},
		{name: "separator", spec: &TerminalSpec{MetadataFile: "a/b", MetadataDir: "/run/x"}, wantErr: true},
		{name: "parent traversal", spec: &TerminalSpec{MetadataFile: "../x", MetadataDir: "/run/x"}, wantErr: true},
		{name: "trailing slash", spec: &TerminalSpec{MetadataFile: "x/", MetadataDir: "/run/x"}, wantErr: true},
		{name: "dot", spec: &TerminalSpec{MetadataFile: ".", MetadataDir: "/run/x"}, wantErr: true},
		{name: "dotdot", spec: &TerminalSpec{MetadataFile: "..", MetadataDir: "/run/x"}, wantErr: true},
		{name: "absolute", spec: &TerminalSpec{MetadataFile: "/etc/passwd", MetadataDir: "/run/x"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveMetadataFile(tc.spec)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolveMetadataFile = %q, nil; want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveMetadataFile: %v", err)
			}
			if got != tc.want {
				t.Fatalf("ResolveMetadataFile = %q, want %q", got, tc.want)
			}
		})
	}
}
