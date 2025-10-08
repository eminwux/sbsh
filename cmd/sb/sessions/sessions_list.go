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

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sbsh/pkg/discovery"
	"sbsh/pkg/env"
)

var (
	listAllInput bool

	// sessionsCmd represents the sessions command
	sessionsListCmd = &cobra.Command{
		Use:     "list",
		Aliases: []string{"l"},
		Short:   "A brief description of your command",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			slog.Debug("sessions list", "runPath", viper.GetString(env.RUN_PATH.ViperKey), "listAll", listAllInput)

			ctx := context.Background()
			return discovery.ScanAndPrintSessions(ctx, viper.GetString(env.RUN_PATH.ViperKey), os.Stdout, listAllInput)
		},
	}
)

func init() {
	sessionsListCmd.Flags().BoolVarP(&listAllInput, "all", "a", false, "List all sessions, including Exited")
}
