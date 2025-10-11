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
	"log/slog"
	"os"

	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/env"
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
			logger, ok := cmd.Context().Value("logger").(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
			}

			logger.Debug("sessions prune")

			return discovery.ScanAndPruneSessions(cmd.Context(), viper.GetString(env.RUN_PATH.ViperKey), os.Stdout)
		},
	}

	setupSessionsPruneCmd(sessionsPruneCmd)
	return sessionsPruneCmd
}

func setupSessionsPruneCmd(_ *cobra.Command) {
}
