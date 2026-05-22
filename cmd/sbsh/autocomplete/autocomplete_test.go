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
	return &cobra.Command{Use: "sbsh"}
}

func TestNewAutoCompleteCmd_Metadata(t *testing.T) {
	cmd := NewAutoCompleteCmd(newRoot())
	if cmd.Use != Command {
		t.Fatalf("Use = %q, want %q", cmd.Use, Command)
	}
	if cmd.Short == "" {
		t.Fatal("Short should not be empty")
	}
	wantValid := []string{"bash", "zsh", "fish"}
	if len(cmd.ValidArgs) != len(wantValid) {
		t.Fatalf("ValidArgs = %v, want %v", cmd.ValidArgs, wantValid)
	}
	for i, v := range wantValid {
		if cmd.ValidArgs[i] != v {
			t.Fatalf("ValidArgs[%d] = %q, want %q", i, cmd.ValidArgs[i], v)
		}
	}
}

func TestNewAutoCompleteCmd_RejectsWrongArgCount(t *testing.T) {
	root := newRoot()
	cmd := NewAutoCompleteCmd(root)
	root.AddCommand(cmd)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	root.SetArgs([]string{"autocomplete"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for missing shell argument (ExactArgs(1))")
	}
}

func TestNewAutoCompleteCmd_GeneratesCompletions(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			root := newRoot()
			cmd := NewAutoCompleteCmd(root)
			root.AddCommand(cmd)

			var buf bytes.Buffer
			cmd.SetOut(&buf)

			if err := cmd.RunE(cmd, []string{shell}); err != nil {
				t.Fatalf("RunE(%q) error = %v", shell, err)
			}
			if buf.Len() == 0 {
				t.Fatalf("RunE(%q) produced no output", shell)
			}
		})
	}
}

func TestNewAutoCompleteCmd_UnknownShellNoOp(t *testing.T) {
	root := newRoot()
	cmd := NewAutoCompleteCmd(root)
	root.AddCommand(cmd)

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.RunE(cmd, []string{"powershell"}); err != nil {
		t.Fatalf("RunE(unknown) error = %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("RunE(unknown) should produce no output; got %q", buf.String())
	}
}
