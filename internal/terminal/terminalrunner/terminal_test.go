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

package terminalrunner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandCwd(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	t.Setenv("SBSH_TEST_DIR", "/var/tmp/sbsh-test")

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"absolute", "/tmp/foo", "/tmp/foo"},
		{"relative", "foo/bar", "foo/bar"},
		{"tilde", "~", home},
		{"tilde-slash", "~/projects", filepath.Join(home, "projects")},
		{"env-bare", "$HOME/projects", filepath.Join(home, "projects")},
		{"env-braced", "${HOME}/projects", filepath.Join(home, "projects")},
		{"env-custom", "$SBSH_TEST_DIR/sub", "/var/tmp/sbsh-test/sub"},
		{"env-unset", "$UNSET_VAR_XYZ_12345/sub", "/sub"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expandCwd(tc.in)
			if err != nil {
				t.Fatalf("expandCwd(%q) returned error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("expandCwd(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
