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

package prune

import (
	"github.com/spf13/cobra"
)

func NewPruneCmd() *cobra.Command {
	pruneCmd := &cobra.Command{
		Use:   "prune",
		Short: "Prune stale supervisors and terminals",
		Long: `Prune stale supervisors and terminals from the sbsh environment.
A stale supervisor is one that is no longer running.
A stale terminal is one whose supervisor is no longer running.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	setupPruneCmd(pruneCmd)
	return pruneCmd
}

func setupPruneCmd(pruneCmd *cobra.Command) {
	pruneCmd.AddCommand(NewPruneTerminalsCmd())
	pruneCmd.AddCommand(NewPruneSupervisorsCmd())
}
