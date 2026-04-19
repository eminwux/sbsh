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
	"fmt"
	"log/slog"
	"os"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewPruneClientsCmd() *cobra.Command {
	pruneClientsCmd := &cobra.Command{
		Use:   "clients",
		Short: "Prune dead or exited clients",
		Long: `Prune dead or exited clients.
This will remove all client files for terminals that are not running anymore.`,
		Args:         cobra.ExactArgs(0),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errdefs.ErrLoggerNotFound
			}

			logger.Debug("clients prune command invoked",
				"run_path", viper.GetString(config.SB_ROOT_RUN_PATH.ViperKey),
				"args", cmd.Flags().Args(),
			)

			err := discovery.ScanAndPruneClients(
				cmd.Context(),
				logger,
				viper.GetString(config.SB_ROOT_RUN_PATH.ViperKey),
				os.Stdout,
			)
			if err != nil {
				logger.Debug("error pruning clients", "error", err)
				_, _ = fmt.Fprintln(os.Stderr, "Could not prune clients:", err)
				os.Exit(1)
			}
			logger.Debug("clients prune completed successfully")
			return nil
		},
	}

	setupClientsPruneCmd(pruneClientsCmd)
	return pruneClientsCmd
}

func setupClientsPruneCmd(_ *cobra.Command) {
}
