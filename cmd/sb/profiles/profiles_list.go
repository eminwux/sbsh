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

	"github.com/spf13/cobra"
	"sbsh/pkg/discovery"
)

// profilesListCmd represents the sessions command.
var profilesListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.Debug("-> profiles list")

		ctx := context.Background()
		// return profiles.ScanAndPrintProfiles(ctx, viper.GetString("global.runPath"), os.Stdout)
		return discovery.ScanAndPrintProfiles(ctx, "/home/inwx/.sbsh/profiles.yaml", os.Stdout)
	},
}
