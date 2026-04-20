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

package stop

import (
	"github.com/spf13/cobra"
)

func NewStopCmd() *cobra.Command {
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop running terminals",
		Long: `Stop running terminals.

Signals a live terminal to shut down via the terminal's control RPC, with a
SIGTERM fallback if the RPC is unreachable. Stale or already-exited terminals
are reported idempotently. Metadata is left behind for 'sb prune terminals'.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	setupStopCmd(stopCmd)
	return stopCmd
}

func setupStopCmd(stopCmd *cobra.Command) {
	stopCmd.AddCommand(NewStopTerminalsCmd())
}
