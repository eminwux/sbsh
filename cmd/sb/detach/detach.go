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
	"github.com/eminwux/sbsh/cmd/sb/get"
	"github.com/eminwux/sbsh/internal/logging"
	"github.com/eminwux/sbsh/pkg/rpcclient/supervisor"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// explicit timeout avoids magic numbers (mnd) and improves readability.
const detachTimeout = 3 * time.Second

const (
	detachNameInput   = "sb.detach.name"
	detachIDInput     = "sb.detach.id"
	detachSocketInput = "sb.detach.socket"
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
		RunE: func(cmd *cobra.Command, args []string) error {
			logger, ok := cmd.Context().Value(logging.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
			}

			logger.DebugContext(cmd.Context(), "detach command invoked")

			if len(args) == 1 {
				// If user passed -n when listing, reject it
				if cmd.Flags().Changed("id") {
					return errors.New("the --id flag is not valid when using positional terminal name")
				}
				if cmd.Flags().Changed("name") {
					return errors.New("the --name flag is not valid when using positional terminal name")
				}
				if cmd.Flags().Changed("socket") {
					return errors.New("the --socket flag is not valid when using positional terminal name")
				}
			} else if len(args) > 1 {
				return errors.New("too many arguments; only one terminal name is allowed")
			}

			err := runDetachCmd(cmd, args)
			if err != nil {
				logger.DebugContext(cmd.Context(), "detach command failed", "error", err)
				return err
			}

			logger.DebugContext(cmd.Context(), "detach command completed successfully")
			return nil
		},
		ValidArgsFunction: get.CompleteSupervisors,
	}
	setupDetachCmd(detachCmd)
	return detachCmd
}

func setupDetachCmd(detachCmd *cobra.Command) {
	detachCmd.Flags().String("id", "", "Session ID, cannot be set together with --name")
	_ = viper.BindPFlag(detachIDInput, detachCmd.Flags().Lookup("id"))
	detachCmd.Flags().StringP("name", "n", "", "Optional session name, cannot be set together with --id")
	_ = viper.BindPFlag(detachNameInput, detachCmd.Flags().Lookup("name"))
	detachCmd.Flags().String("socket", "", "Supervisor Socket Path")
	_ = viper.BindPFlag(detachSocketInput, detachCmd.Flags().Lookup("socket"))
}

func runDetachCmd(cmd *cobra.Command, args []string) error {
	logger, ok := cmd.Context().Value(logging.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errors.New("logger not found in context")
	}

	terminalSocketFlag := viper.GetString(detachSocketInput)
	terminalIDFlag := viper.GetString(detachIDInput)
	terminalNameFlag := viper.GetString(detachNameInput)

	if len(args) == 1 {
		if terminalIDFlag != "" || terminalNameFlag != "" || terminalSocketFlag != "" {
			return errors.New("when using flags only, a positional terminal name cannot be provided")
		}
	}

	var terminalNamePositional string
	if len(args) == 1 {
		terminalNamePositional = args[0]
	}

	if terminalIDFlag != "" && terminalNameFlag != "" {
		return errors.New("only one of --id or --name can be provided")
	}

	if terminalIDFlag != "" && terminalSocketFlag != "" {
		return errors.New("only one of --id or --socket can be provided")
	}

	if terminalNameFlag != "" && terminalSocketFlag != "" {
		return errors.New("only one of --name or --socket can be provided")
	}

	terminalName := terminalNamePositional
	if terminalNamePositional == "" {
		terminalName = terminalNameFlag
	}

	socket, errC := buildSocket(cmd, logger, terminalSocketFlag, terminalIDFlag, terminalName)
	if errC != nil {
		logger.ErrorContext(cmd.Context(), "cannot build socket path", "error", errC)
		return fmt.Errorf("cannot build socket path: %w", errC)
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
		return fmt.Errorf("could not detach: %w", err)
	}

	logger.DebugContext(ctx, "detach successful")
	return nil
}

func buildSocket(
	cmd *cobra.Command,
	logger *slog.Logger,
	terminalSocketFlag string,
	terminalIDFlag string,
	terminalName string,
) (string, error) {
	var socket string
	if terminalSocketFlag != "" {
		socket = terminalSocketFlag
	} else {
		runPath, err := config.GetRunPathFromEnvAndFlags(cmd)
		if err != nil {
			logger.ErrorContext(cmd.Context(), "cannot determine run path", "error", err)
			return "", fmt.Errorf("cannot determine run path: %w", err)
		}

		var supervisorID string
		switch {
		case terminalIDFlag != "":
			supervisorID = terminalIDFlag
		case terminalName != "":
			supervisorID, err = get.ResolveSupervisorNameToID(cmd.Context(), logger, runPath, terminalName)
			if err != nil {
				logger.ErrorContext(
					cmd.Context(),
					"cannot resolve terminal name to ID",
					"terminal_name",
					terminalName,
					"error",
					err,
				)
				return "", fmt.Errorf("cannot resolve terminal name to ID: %w", err)
			}
		default:
			logger.DebugContext(
				cmd.Context(),
				"no terminal identification method provided, cannot detach",
			)

			supervisorID = os.Getenv("SBSH_SUP_ID")
			if supervisorID == "" {
				logger.DebugContext(cmd.Context(), "no supervisor id found in SBSH_SUP_ID, cannot detach")
				return "", errors.New("no supervisor id found in SBSH_SUP_ID")
			}
		}

		socket = fmt.Sprintf("%s/supervisors/%s/socket", runPath, supervisorID)
	}
	return socket, nil
}
