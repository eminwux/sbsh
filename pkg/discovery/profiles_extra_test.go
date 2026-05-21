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

package discovery_test

import (
	"context"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/pkg/discovery"
)

func TestProfileWarning_String(t *testing.T) {
	t.Parallel()

	fileLevel := discovery.ProfileWarning{File: "/x/p.yaml", Reason: "open: denied"}
	if got := fileLevel.String(); got != "/x/p.yaml: open: denied" {
		t.Fatalf("file-level String() = %q", got)
	}

	docLevel := discovery.ProfileWarning{File: "/x/p.yaml", DocIndex: 3, Reason: "bad"}
	if got := docLevel.String(); got != "/x/p.yaml (doc 3): bad" {
		t.Fatalf("doc-level String() = %q", got)
	}
}

func TestLoadProfilesFromReaderWithContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := testLogger()

	t.Run("valid multi-doc", func(t *testing.T) {
		t.Parallel()
		profiles, err := discovery.LoadProfilesFromReaderWithContext(ctx, logger, strings.NewReader(multiDocProfiles))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(profiles) != 2 {
			t.Fatalf("expected 2 profiles, got %d", len(profiles))
		}
	})

	t.Run("skips empty/invalid doc", func(t *testing.T) {
		t.Parallel()
		profiles, err := discovery.LoadProfilesFromReaderWithContext(
			ctx,
			logger,
			strings.NewReader(profilesWithEmptyDoc),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// The incomplete middle document (no apiVersion/kind) is skipped.
		if len(profiles) != 2 {
			t.Fatalf("expected 2 valid profiles, got %d", len(profiles))
		}
	})

	t.Run("malformed YAML returns error", func(t *testing.T) {
		t.Parallel()
		bad := "apiVersion: sbsh/v1beta1\nkind: TerminalProfile\n  oops:\n    -\n"
		if _, err := discovery.LoadProfilesFromReaderWithContext(ctx, logger, strings.NewReader(bad)); err == nil {
			t.Fatal("expected error for malformed YAML")
		}
	})
}

// TestValidateRequiredFields_Branches drives each rejection branch of the
// (unexported) validateRequiredFields via LoadProfilesFromDir, asserting on
// the warning Reason each one produces.
func TestValidateRequiredFields_Branches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := testLogger()

	cases := []struct {
		name    string
		yaml    string
		wantSub string
	}{
		{
			name:    "missing apiVersion",
			yaml:    "kind: TerminalProfile\nmetadata:\n  name: x\n",
			wantSub: "missing required apiVersion",
		},
		{
			name:    "unsupported apiVersion",
			yaml:    "apiVersion: sbsh/v2\nkind: TerminalProfile\nmetadata:\n  name: x\n",
			wantSub: "unsupported apiVersion",
		},
		{
			name:    "missing kind",
			yaml:    "apiVersion: sbsh/v1beta1\nmetadata:\n  name: x\nspec: {}\n",
			wantSub: "missing required kind",
		},
		{
			name:    "unsupported kind",
			yaml:    "apiVersion: sbsh/v1beta1\nkind: Widget\nmetadata:\n  name: x\n",
			wantSub: "unsupported kind",
		},
		{
			name:    "missing name",
			yaml:    "apiVersion: sbsh/v1beta1\nkind: TerminalProfile\nspec:\n  runTarget: local\n",
			wantSub: "missing required metadata.name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeFile(t, dir, "p.yaml", tc.yaml)
			profiles, warnings, err := discovery.LoadProfilesFromDir(ctx, logger, dir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(profiles) != 0 {
				t.Fatalf("expected no valid profiles, got %d", len(profiles))
			}
			if len(warnings) == 0 || !strings.Contains(warnings[0].Reason, tc.wantSub) {
				t.Fatalf("expected warning containing %q, got %+v", tc.wantSub, warnings)
			}
		})
	}
}
