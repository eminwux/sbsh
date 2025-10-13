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
	"errors"
	"log/slog"
	"os"

	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/logging"
	"github.com/spf13/cobra"
)

// NewProfilesListCmd represents the sessions command.
func NewProfilesListCmd() *cobra.Command {
	profilesListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"l"},
		Short:   "List available profiles",
		Long: `List available profiles.
This command scans and lists all available profiles in the specified configuration file.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger, ok := cmd.Context().Value(logging.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
			}

			logger.Debug("profiles list command invoked",
				"args", cmd.Flags().Args(),
				"profiles_file", "/home/inwx/.sbsh/profiles.yaml",
			)

			err := discovery.ScanAndPrintProfiles(cmd.Context(), logger, "/home/inwx/.sbsh/profiles.yaml", os.Stdout)
			if err != nil {
				logger.Debug("error scanning and printing profiles", "error", err)
				_, _ = os.Stderr.WriteString("Could not list profiles: " + err.Error() + "\n")
				os.Exit(1)
			}
			logger.Debug("profiles list completed successfully")
			return nil
		},
	}

	setupProfilesListCmd(profilesListCmd)
	return profilesListCmd
}

func setupProfilesListCmd(_ *cobra.Command) {
}
