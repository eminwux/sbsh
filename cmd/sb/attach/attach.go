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

package attach

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sbsh/pkg/env"
	"sbsh/pkg/rpcclient/supervisor"
)

// AttachCmd represents the attach command.
var AttachCmd = &cobra.Command{
	Use:     "attach",
	Aliases: []string{"a"},
	Short:   "Attach to a running supervisor",
	Long: `Attach to a running supervisor
This command connects to a running supervisor instance and attaches to its console.
It requires the SBSH_SUP_SOCKET environment variable to be set, pointing to the supervisor's socket file.

Examples:
  # Attach using environment variable
  sb attach

  # Attach specifying a socket file
  sb attach --socket /tmp/sbsh.sock

  # Attach to a supervisor by id
  sb attach --id 1234

  # Attach to a supervisor by name
  sb attach --name my-supervisor
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.Debug("-> attach")

		// check SBSH_SUP_SOCKET

		socket := viper.GetString(env.SUP_SOCKET.ViperKey)
		if socket == "" {
			fmt.Fprintln(os.Stderr, "no supervisor socket found")
			os.Exit(1)
		}
		sup := supervisor.NewUnix(socket)
		defer sup.Close()

		ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Second)
		defer cancel()

		fmt.Fprintf(os.Stdout, "attaching..\r\n")
		if err := sup.Detach(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "attach failed: %v\r\n", err)
			os.Exit(1)
		}

		return nil
	},
}
