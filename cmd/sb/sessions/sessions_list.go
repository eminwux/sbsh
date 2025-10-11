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

package sessions

import (
	"context"
	"log/slog"
	"os"

	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/env"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var listAllInput bool

func NewSessionListCmd() *cobra.Command {
	// sessionsListCmd represents the sessions command.
	sessionsListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"l"},
		Short:   "List sessions",
		Long: `List sessions.
This command scans and lists all sessions in the specified run path.
By default, it lists only running sessions. Use the --all flag to include exited sessions.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			slog.Debug("sessions list", "runPath", viper.GetString(env.RUN_PATH.ViperKey), "listAll", listAllInput)

			ctx := context.Background()
			return discovery.ScanAndPrintSessions(ctx, viper.GetString(env.RUN_PATH.ViperKey), os.Stdout, listAllInput)
		},
	}

	setupSessionsListCmd(sessionsListCmd)
	return sessionsListCmd
}

func setupSessionsListCmd(sessionsListCmd *cobra.Command) {
	sessionsListCmd.Flags().BoolVarP(&listAllInput, "all", "a", false, "List all sessions, including Exited")
}
