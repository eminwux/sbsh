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

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/internal/logging"
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
			logger, ok := cmd.Context().Value(logging.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
			}

			logger.DebugContext(cmd.Context(), "detach command invoked")

			// explicit timeout avoids magic numbers (mnd) and improves readability
			const detachTimeout = 3 * time.Second

			// check SBSH_SUP_SOCKET
			socket := viper.GetString("sb.detach.socket")
			if socket == "" {
				logger.DebugContext(
					cmd.Context(),
					"socket not defined with flags, using run path to determine supervisor socket",
				)
				id := os.Getenv("SBSH_SUP_ID")
				if id == "" {
					logger.DebugContext(cmd.Context(), "no supervisor id found in SBSH_SUP_ID, cannot detach")
					return errors.New("no supervisor id found in SBSH_SUP_ID")
				}

				runPath, err := config.GetRunPathFromEnvAndFlags(cmd)
				if err != nil {
					logger.ErrorContext(cmd.Context(), "cannot determine run path", "error", err)
					return fmt.Errorf("cannot determine run path: %w", err)
				}

				socket = fmt.Sprintf("%s/supervisors/%s/socket", runPath, id)
			}

			logger.DebugContext(cmd.Context(), "creating supervisor unix client", "socket", socket)
			sup := supervisor.NewUnix(socket)
			defer sup.Close()

			ctx, cancel := context.WithTimeout(cmd.Context(), detachTimeout)
			defer cancel()

			logger.DebugContext(ctx, "detaching from supervisor", "timeout", detachTimeout)
			fmt.Fprintf(os.Stdout, "detaching..\r\n")
			if err := sup.Detach(ctx); err != nil {
				logger.DebugContext(ctx, "detach failed", "error", err)
				fmt.Fprintf(os.Stderr, "Could not detach: %v\n", err)
				cancel()
				os.Exit(1)
			}

			logger.DebugContext(ctx, "detach successful")
			return nil
		},
	}
	setupDetachCmd(detachCmd)
	return detachCmd
}

func setupDetachCmd(detachCmd *cobra.Command) {
	detachCmd.Flags().String("socket", "", "Supervisor Socket Path")

	_ = viper.BindPFlag("sb.detach.socket", detachCmd.Flags().Lookup("socket"))
}
