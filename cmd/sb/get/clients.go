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

package get

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewGetClientCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "clients",
		Aliases:      []string{"client", "cl", "c"},
		Short:        "Get clients",
		Long:         "Get clients from the sbsh environment.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				if cmd.Flags().Changed("output") {
					return fmt.Errorf(
						"%w: the -o/--output flag is only valid when specifying a client name",
						errdefs.ErrInvalidFlag,
					)
				}
				return listClients(cmd, args)
			} else if len(args) > 1 {
				return errdefs.ErrTooManyArguments
			}

			return getClient(cmd, args)
		},
		ValidArgsFunction: CompleteClients,
	}

	setupNewGetClientCmd(cmd)
	return cmd
}

func setupNewGetClientCmd(cmd *cobra.Command) {
	cmd.Flags().BoolP("all", "a", false, "List all clients, including Exited")
	_ = viper.BindPFlag(config.SB_GET_CLIENTS_ALL.ViperKey, cmd.Flags().Lookup("all"))

	cmd.Flags().StringP("output", "o", "", "Output format: json|yaml (default: human-readable)")
	_ = viper.BindPFlag(config.SB_GET_CLIENTS_OUTPUT.ViperKey, cmd.Flags().Lookup("output"))

	_ = cmd.RegisterFlagCompletionFunc(
		"output",
		func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return []string{"json", "yaml"}, cobra.ShellCompDirectiveNoFileComp
		},
	)
}

func listClients(cmd *cobra.Command, _ []string) error {
	logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errdefs.ErrLoggerNotFound
	}

	logger.Debug("clients list command invoked",
		"run_path", viper.GetString(config.SB_ROOT_RUN_PATH.ViperKey),
		"list_all", viper.GetBool(config.SB_GET_CLIENTS_ALL.ViperKey),
		"args", cmd.Flags().Args(),
	)

	err := discovery.ScanAndPrintClients(
		cmd.Context(),
		logger,
		viper.GetString(config.SB_ROOT_RUN_PATH.ViperKey),
		os.Stdout,
		viper.GetBool(config.SB_GET_CLIENTS_ALL.ViperKey),
	)
	if err != nil {
		logger.Debug("error scanning and printing clients", "error", err)
		fmt.Fprintln(os.Stderr, "Could not scan clients")
		return err
	}
	logger.Debug("clients list completed successfully")
	return nil
}

func CompleteClients(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	runPath, err := config.GetRunPathFromEnvAndFlags(cmd, config.SB_ROOT_RUN_PATH.EnvVar())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	names, err := fetchClientNames(cmd.Context(), runPath, toComplete)
	if err != nil {
		return []string{"__error: cannot list clients"}, cobra.ShellCompDirectiveNoFileComp
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func fetchClientNames(ctx context.Context, runPath string, toComplete string) ([]string, error) {
	logger, _ := ctx.Value(types.CtxLogger).(*slog.Logger)

	all, err := config.AutoCompleteListClientNames(ctx, logger, runPath, false)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(all))
	for _, n := range all {
		if toComplete == "" || strings.HasPrefix(n, toComplete) {
			out = append(out, n)
		}
	}

	return out, nil
}

func getClient(cmd *cobra.Command, args []string) error {
	logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errdefs.ErrLoggerNotFound
	}
	clientName := args[0]
	format := viper.GetString(config.SB_GET_CLIENTS_OUTPUT.ViperKey)
	if format != "" && format != "json" && format != "yaml" {
		return fmt.Errorf("%w: %s", errdefs.ErrInvalidOutputFormat, format)
	}
	logger.Debug("get client command invoked",
		"run_path", viper.GetString(config.SB_ROOT_RUN_PATH.ViperKey),
		"client_name", clientName,
		"output_format", format,
		"args", cmd.Flags().Args(),
	)

	runPath, err := config.GetRunPathFromEnvAndFlags(cmd, config.SB_ROOT_RUN_PATH.EnvVar())
	if err != nil {
		return fmt.Errorf("%w: %w", errdefs.ErrGetRunPath, err)
	}
	return discovery.FindAndPrintClientMetadata(cmd.Context(), logger, runPath, os.Stdout, clientName, format)
}

func ResolveClientNameToID(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	clientName string,
) (string, error) {
	clients, err := discovery.ScanClients(ctx, logger, runPath)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(
				ctx,
				"ResolveClientNameToID: failed to load clients",
				"path",
				runPath,
				"error",
				err,
			)
		}
		return "", err
	}
	if clients == nil {
		return "", errdefs.ErrNoClientsFound
	}

	for _, t := range clients {
		if t.Metadata.Name == clientName {
			return string(t.Spec.ID), nil
		}
	}

	return "", fmt.Errorf("%w: client with name '%s' not found", errdefs.ErrClientNotFound, clientName)
}
