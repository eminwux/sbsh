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

package detach

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/eminwux/sbsh/internal/env"
	"github.com/eminwux/sbsh/pkg/rpcclient/supervisor"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewDetachCmd() *cobra.Command {
	// sessionsCmd represents the sessions command
	detachCmd := &cobra.Command{
		Use:     "detach",
		Aliases: []string{"d"},
		Short:   "Detach from a running supervisor",
		Long: `Detach from a running supervisor.

This command takes a --socket argument to specify the supervisor socket path.
If not provided, it will look for the SBSH_SUP_SOCKET environment variable.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger, ok := cmd.Context().Value("logger").(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
			}

			logger.Debug("-> detach")

			// explicit timeout avoids magic numbers (mnd) and improves readability
			const detachTimeout = 3 * time.Second

			// check SBSH_SUP_SOCKET

			socket := viper.GetString(env.SUP_SOCKET.ViperKey)
			if socket == "" {
				fmt.Fprintln(os.Stderr, "no supervisor socket found")
				os.Exit(1)
			}
			sup := supervisor.NewUnix(socket)
			defer sup.Close()

			ctx, cancel := context.WithTimeout(cmd.Context(), detachTimeout)
			defer cancel()

			fmt.Fprintf(os.Stdout, "detaching..\r\n")
			if err := sup.Detach(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "detach failed: %v\r\n", err)
				os.Exit(1)
			}

			return nil
		},
	}
	setupDetachCmd(detachCmd)
	return detachCmd
}

func setupDetachCmd(detachCmd *cobra.Command) {
	flagS := "socket"
	// define the flag without binding to a package-level variable; use viper to read it
	detachCmd.Flags().String(flagS, "", "Supervisor Socket Path")

	_ = env.SUP_SOCKET.BindEnv()

	if err := viper.BindPFlag(env.SUP_SOCKET.ViperKey, detachCmd.Flags().Lookup(flagS)); err != nil {
		// avoid fatal exit here; log and continue so command can still run with env var
		slog.Warn("could not bind cobra flag to viper", "error", err)
	}
}
