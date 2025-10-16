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

func NewSessionsPruneCmd() *cobra.Command {
	// sessionsPruneCmd represents the sessions command.
	sessionsPruneCmd := &cobra.Command{
		Use:     "prune",
		Aliases: []string{"p"},
		Short:   "Prune dead or exited sessions",
		Long: `Prune dead or exited sessions.
This will remove all session files for sessions that are not running anymore.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger, ok := cmd.Context().Value(logging.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
			}

			logger.Debug("sessions prune command invoked",
				"run_path", viper.GetString(config.RUN_PATH.ViperKey),
				"args", cmd.Flags().Args(),
			)

			err := discovery.ScanAndPruneSessions(
				cmd.Context(),
				logger,
				viper.GetString(config.RUN_PATH.ViperKey),
				os.Stdout,
			)
			if err != nil {
				logger.Debug("error pruning sessions", "error", err)
				// Print to stderr and exit 1 as requested
				_, _ = fmt.Fprintln(os.Stderr, "Could not prune sessions:", err)
				os.Exit(1)
			}
			logger.Debug("sessions prune completed successfully")
			return nil
		},
	}

	setupSessionsPruneCmd(sessionsPruneCmd)
	return sessionsPruneCmd
}

func setupSessionsPruneCmd(_ *cobra.Command) {
}
