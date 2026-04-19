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
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/sb/get"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/rpcclient/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// explicit timeout avoids magic numbers (mnd) and improves readability.
const detachTimeout = 3 * time.Second

func NewDetachCmd() *cobra.Command {
	detachCmd := &cobra.Command{
		Use:     "detach",
		Aliases: []string{"d"},
		Short:   "Detach from a running client",
		Long: `Detach from a running client.

This command takes a --socket argument to specify the client socket path.
If not provided, it will look for the SBSH_ROOT_CLIENT_SOCKET environment variable.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errdefs.ErrLoggerNotFound
			}

			logger.DebugContext(cmd.Context(), "detach command invoked")

			switch {
			case len(args) == 0:
				if !cmd.Flags().Changed("id") && !cmd.Flags().Changed("name") && !cmd.Flags().Changed("socket") {
					return errdefs.ErrNoClientIdentifier
				}
			case len(args) == 1:
				// If user passed -n when listing, reject it
				if cmd.Flags().Changed("id") {
					return fmt.Errorf(
						"%w: the --id flag is not valid when using positional terminal name",
						errdefs.ErrInvalidFlag,
					)
				}
				if cmd.Flags().Changed("name") {
					return fmt.Errorf(
						"%w: the --name flag is not valid when using positional terminal name",
						errdefs.ErrInvalidFlag,
					)
				}
				if cmd.Flags().Changed("socket") {
					return fmt.Errorf(
						"%w: the --socket flag is not valid when using positional terminal name",
						errdefs.ErrInvalidFlag,
					)
				}
			case len(args) > 1:
				return errdefs.ErrTooManyArguments
			}

			err := runDetachCmd(cmd, args)
			if err != nil {
				logger.DebugContext(cmd.Context(), "detach command failed", "error", err)
				return err
			}

			logger.DebugContext(cmd.Context(), "detach command completed successfully")
			return nil
		},
		ValidArgsFunction: get.CompleteClients,
	}
	setupDetachCmd(detachCmd)
	return detachCmd
}

func setupDetachCmd(detachCmd *cobra.Command) {
	detachCmd.Flags().String("id", "", "Terminal ID, cannot be set together with --name")
	_ = viper.BindPFlag(config.SB_DETACH_ID.ViperKey, detachCmd.Flags().Lookup("id"))
	detachCmd.Flags().StringP("name", "n", "", "Optional terminal name, cannot be set together with --id")
	_ = viper.BindPFlag(config.SB_DETACH_NAME.ViperKey, detachCmd.Flags().Lookup("name"))
	detachCmd.Flags().String("socket", "", "Client Socket Path")
	_ = viper.BindPFlag(config.SB_DETACH_SOCKET.ViperKey, detachCmd.Flags().Lookup("socket"))
}

func runDetachCmd(cmd *cobra.Command, args []string) error {
	logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errdefs.ErrLoggerNotFound
	}

	clientSocketFlag := viper.GetString(config.SB_DETACH_SOCKET.ViperKey)
	clientIDFlag := viper.GetString(config.SB_DETACH_ID.ViperKey)
	clientNameFlag := viper.GetString(config.SB_DETACH_NAME.ViperKey)

	if len(args) == 1 {
		if clientIDFlag != "" || clientNameFlag != "" || clientSocketFlag != "" {
			return errdefs.ErrPositionalWithFlags
		}
	}

	var clientNamePositional string
	if len(args) == 1 {
		clientNamePositional = args[0]
	}

	if clientIDFlag != "" && clientNameFlag != "" {
		return fmt.Errorf("%w: only one of --id or --name can be provided", errdefs.ErrConflictingFlags)
	}

	if clientIDFlag != "" && clientSocketFlag != "" {
		return fmt.Errorf("%w: only one of --id or --socket can be provided", errdefs.ErrConflictingFlags)
	}

	if clientNameFlag != "" && clientSocketFlag != "" {
		return fmt.Errorf("%w: only one of --name or --socket can be provided", errdefs.ErrConflictingFlags)
	}

	clientName := clientNamePositional
	if clientNamePositional == "" {
		clientName = clientNameFlag
	}

	socket, errC := buildSocket(cmd, logger, clientSocketFlag, clientIDFlag, clientName)
	if errC != nil {
		logger.ErrorContext(cmd.Context(), "cannot build socket path", "error", errC)
		return fmt.Errorf("%w: %w", errdefs.ErrBuildSocketPath, errC)
	}

	logger.DebugContext(cmd.Context(), "creating client unix client", "socket", socket)
	sup := client.NewUnix(socket)
	defer sup.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), detachTimeout)
	defer cancel()

	logger.DebugContext(ctx, "detaching from client", "timeout", detachTimeout)
	fmt.Fprintf(os.Stdout, "detaching..\r\n")
	if err := sup.Detach(ctx); err != nil {
		logger.DebugContext(ctx, "detach failed", "error", err)
		fmt.Fprintf(os.Stderr, "Could not detach: %v\n", err)
		cancel()
		return fmt.Errorf("%w: %w", errdefs.ErrDetachTerminal, err)
	}

	logger.DebugContext(ctx, "detach successful")
	return nil
}

func buildSocket(
	cmd *cobra.Command,
	logger *slog.Logger,
	clientSocketFlag string,
	clientIDFlag string,
	clientName string,
) (string, error) {
	if clientSocketFlag != "" {
		return clientSocketFlag, nil
	}

	runPath, err := config.GetRunPathFromEnvAndFlags(cmd, config.SB_ROOT_RUN_PATH.EnvVar())
	if err != nil {
		logger.ErrorContext(cmd.Context(), "cannot determine run path", "error", err)
		return "", fmt.Errorf("%w: %w", errdefs.ErrDetermineRunPath, err)
	}

	var clientID string
	switch {
	case clientIDFlag != "":
		clientID = clientIDFlag
	case clientName != "":
		clientID, err = get.ResolveClientNameToID(cmd.Context(), logger, runPath, clientName)
		if err != nil {
			logger.ErrorContext(
				cmd.Context(),
				"cannot resolve terminal name to ID",
				"terminal_name",
				clientName,
				"error",
				err,
			)
			return "", fmt.Errorf("%w: %w", errdefs.ErrResolveTerminalName, err)
		}
	default:
		logger.DebugContext(
			cmd.Context(),
			"no client identification method provided, cannot detach",
		)
		return "", errdefs.ErrNoClientIdentification
	}

	socket := fmt.Sprintf("%s/%s/%s/socket", runPath, defaults.ClientsRunPath, clientID)
	return socket, nil
}
