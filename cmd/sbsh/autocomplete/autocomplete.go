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

import "github.com/spf13/cobra"

const (
	Command string = "autocomplete"
)

func NewAutoCompleteCmd(root *cobra.Command) *cobra.Command {
	autocompleteCmd := &cobra.Command{
		Use:   Command,
		Short: "Generate shell autocompletion scripts",
		Long: `Generate shell autocompletion scripts for sbsh.

To load completions in your current shell session:

Bash:

  $ source <(sbsh autocomplete bash)

  # To load completions for each session, execute once:
  # Linux:
  $ sbsh autocomplete bash > /etc/bash_completion.d/sbsh
  # macOS:
  $ sbsh autocomplete bash > /usr/local/etc/bash_completion.d/sbsh

Zsh:

  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  $ sbsh autocomplete zsh > "${fpath[1]}/_sbsh"

  # You will need to start a new shell for this setup to take effect.

Fish:

  $ sbsh autocomplete fish | source

  # To load completions for each session, execute once:
  $ sbsh autocomplete fish > ~/.config/fish/completions/sbsh.fish
`,
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
		Args:                  cobra.ExactArgs(1),
		ValidArgs:             []string{"bash", "zsh", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			default:
				return nil
			}
		},
	}

	return autocompleteCmd
}
