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
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const listAllInput = "sb.sessions.list.all"

func NewSessionListCmd() *cobra.Command {
	// sessionsListCmd represents the sessions command.
	sessionsListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"l"},
		Short:   "List sessions",
		Long: `List sessions.
This command scans and lists all sessions in the specified run path.
By default, it lists only running sessions. Use the --all flag to include exited sessions.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger, ok := cmd.Context().Value(logging.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
			}

			logger.Debug("sessions list command invoked",
				"run_path", viper.GetString(config.RUN_PATH.ViperKey),
				"list_all", listAllInput,
				"args", cmd.Flags().Args(),
			)

			err := discovery.ScanAndPrintSessions(
				cmd.Context(),
				logger,
				viper.GetString(config.RUN_PATH.ViperKey),
				os.Stdout,
				viper.GetBool(listAllInput),
			)
			if err != nil {
				logger.Debug("error scanning and printing sessions", "error", err)
				fmt.Fprintln(os.Stderr, "Could not scan sessions")
				return err
			}
			logger.Debug("sessions list completed successfully")
			return nil
		},
	}

	setupSessionsListCmd(sessionsListCmd)
	return sessionsListCmd
}

func setupSessionsListCmd(sessionsListCmd *cobra.Command) {
	sessionsListCmd.Flags().BoolP("all", "a", false, "List all sessions, including Exited")
	_ = viper.BindPFlag(listAllInput, sessionsListCmd.Flags().Lookup("all"))
}
