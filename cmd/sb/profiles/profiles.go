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

package profiles

import (
	"github.com/spf13/cobra"
)

func NewProfilesCmd() *cobra.Command {
	// ProfilesCmd represents the sessions command.
	profilesCmd := &cobra.Command{
		Use:     "profiles",
		Aliases: []string{"p"},
		Short:   "Manage sbsh profiles (category, not a final command)",
		Long: `This is a category command for managing sbsh profiles.
See 'sb profiles --help' for available subcommands.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	setupProfilesCmd(profilesCmd)
	return profilesCmd
}

func setupProfilesCmd(profilesCmd *cobra.Command) {
	profilesCmd.AddCommand(profilesListCmd)
}
