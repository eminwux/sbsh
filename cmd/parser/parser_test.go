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

package parser

import (
	"reflect"
	"testing"

	"github.com/eminwux/sbsh/pkg/api"
)

func TestJoinLabels(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]string
		want string
	}{
		{name: "nil map", in: nil, want: "none"},
		{name: "empty map", in: map[string]string{}, want: "none"},
		{name: "single", in: map[string]string{"env": "prod"}, want: "env=prod"},
		{
			name: "multiple sorted by key",
			in:   map[string]string{"team": "infra", "env": "prod", "app": "api"},
			want: "app=api,env=prod,team=infra",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := JoinLabels(tt.in); got != tt.want {
				t.Fatalf("JoinLabels(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGetTerminalLabelsString(t *testing.T) {
	t.Run("returns labels when present", func(t *testing.T) {
		labels := map[string]string{"env": "prod"}
		doc := api.TerminalDoc{Spec: api.TerminalSpec{Labels: labels}}
		if got := GetTerminalLabelsString(doc); !reflect.DeepEqual(got, labels) {
			t.Fatalf("GetTerminalLabelsString() = %v, want %v", got, labels)
		}
	})

	t.Run("returns empty map when no labels", func(t *testing.T) {
		doc := api.TerminalDoc{}
		got := GetTerminalLabelsString(doc)
		if len(got) != 0 {
			t.Fatalf("GetTerminalLabelsString() = %v, want empty map", got)
		}
	})
}
