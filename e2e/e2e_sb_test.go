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

package e2e_test

import (
	"testing"

	"github.com/eminwux/sbsh/internal/discovery"
)

func TestSb_Help(t *testing.T) {
	// Make this test parallel-friendly with other tests
	t.Parallel()

	// Check sb helper binary help output
	_ = runReturningBinary(t, nil, sb, "-h")
	_ = runReturningBinary(t, nil, sb, "--help")
}

func TestSb_NoTerminals(t *testing.T) {
	t.Parallel()

	var out []byte
	var expected, got string
	tests := [][]string{
		{"get", "terminals"},
		{"get", "term"},
		{"g", "t"},
	}
	for _, args := range tests {
		runPath := getRandomRunPath(t)
		mkdirRunPath(t, runPath)
		runPathEnv := buildSbRunPathEnv(t, runPath)
		out = runReturningBinary(t, []string{runPathEnv}, sb, args...)
		expected = discovery.NoTerminalsString
		got = string(out)
		if got != expected {
			t.Fatalf("expected %s\n got:\n%s", expected, got)
		}
	}
}

func TestSb_NoSupervisors(t *testing.T) {
	t.Parallel()

	var out []byte
	var expected, got string
	tests := [][]string{
		{"get", "supervisors"},
		{"get", "super"},
		{"g", "s"},
	}
	for _, args := range tests {
		runPath := getRandomRunPath(t)
		mkdirRunPath(t, runPath)
		runPathEnv := buildSbRunPathEnv(t, runPath)
		out = runReturningBinary(t, []string{runPathEnv}, sb, args...)
		expected = discovery.NoSupervisorsString
		got = string(out)
		if got != expected {
			t.Fatalf("expected %s\n got:\n%s", expected, got)
		}
	}
}
