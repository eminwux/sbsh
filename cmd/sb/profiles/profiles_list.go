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
	"context"
	"log/slog"
	"os"

	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// profilesListCmd represents the sessions command.
var profilesListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "List available profiles",
	Long: `List available profiles.
This command scans and lists all available profiles in the specified configuration file.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		slog.Debug("-> profiles list")

		ctx := context.Background()
		if slog.Default().Enabled(ctx, slog.LevelDebug) {
			discovery.ScanAndPrintProfiles(ctx, viper.GetString("global.runPath"), os.Stdout)
		}
		return discovery.ScanAndPrintProfiles(ctx, "/home/inwx/.sbsh/profiles.yaml", os.Stdout)
	},
}
