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

package autocomplete

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func newRoot() *cobra.Command {
	return &cobra.Command{Use: "sb"}
}

func Test_NewAutoCompleteCmd_Metadata(t *testing.T) {
	cmd := NewAutoCompleteCmd(newRoot())
	if cmd == nil {
		t.Fatal("NewAutoCompleteCmd returned nil")
	}
	if cmd.Use != Command {
		t.Errorf("Use = %q, want %q", cmd.Use, Command)
	}
	if got, want := cmd.ValidArgs, []string{"bash", "zsh", "fish"}; len(got) != len(want) {
		t.Fatalf("ValidArgs = %v, want %v", got, want)
	}
}

func Test_AutoComplete_GeneratesScripts(t *testing.T) {
	tests := []struct {
		shell  string
		needle string
	}{
		{"bash", "bash completion"},
		{"zsh", "compdef"},
		{"fish", "complete"},
	}
	for _, tc := range tests {
		t.Run(tc.shell, func(t *testing.T) {
			cmd := NewAutoCompleteCmd(newRoot())
			var buf bytes.Buffer
			cmd.SetOut(&buf)

			if err := cmd.RunE(cmd, []string{tc.shell}); err != nil {
				t.Fatalf("RunE(%q) returned error: %v", tc.shell, err)
			}
			if buf.Len() == 0 {
				t.Fatalf("RunE(%q) produced no output", tc.shell)
			}
			if !bytes.Contains(bytes.ToLower(buf.Bytes()), []byte(tc.needle)) {
				t.Errorf("RunE(%q) output missing %q marker", tc.shell, tc.needle)
			}
		})
	}
}

func Test_AutoComplete_UnknownShell_NoOutputNoError(t *testing.T) {
	cmd := NewAutoCompleteCmd(newRoot())
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.RunE(cmd, []string{"powershell"}); err != nil {
		t.Fatalf("RunE(unknown) returned error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("RunE(unknown) should produce no output, got %q", buf.String())
	}
}

func Test_AutoComplete_RejectsWrongArgCount(t *testing.T) {
	cmd := NewAutoCompleteCmd(newRoot())
	cmd.SetArgs([]string{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing shell argument, got nil")
	}
}
