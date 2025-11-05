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

const (
	listAllInput = "sb.get.terminals.all"
	outputFormat = "sb.get.terminals.output"
)

func NewGetTerminalCmd() *cobra.Command {
	// GetTerminalCmd represents the get terminal command.
	cmd := &cobra.Command{
		Use:          "terminal",
		Aliases:      []string{"terminals", "terms", "term", "t"},
		Short:        "Get a terminal",
		Long:         "Get a terminal from the sbsh environment.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				// If user passed -o when listing, reject it
				if cmd.Flags().Changed("output") {
					return fmt.Errorf(
						"%w: the -o/--output flag is only valid when specifying a terminal name",
						errdefs.ErrInvalidFlag,
					)
				}
				return listTerminals(cmd, args)
			} else if len(args) > 1 {
				return errdefs.ErrTooManyArguments
			}

			return getTerminal(cmd, args)
		},
		// POSitional completion for NAME
		ValidArgsFunction: CompleteTerminals,
	}

	setupNewGetTerminalCmd(cmd)
	return cmd
}

func setupNewGetTerminalCmd(cmd *cobra.Command) {
	cmd.Flags().BoolP("all", "a", false, "List all terminals, including Exited")
	_ = viper.BindPFlag(listAllInput, cmd.Flags().Lookup("all"))

	cmd.Flags().StringP("output", "o", "", "Output format: json|yaml (default: human-readable)")
	_ = viper.BindPFlag(outputFormat, cmd.Flags().Lookup("output"))

	_ = cmd.RegisterFlagCompletionFunc(
		"output",
		func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return []string{"json", "yaml"}, cobra.ShellCompDirectiveNoFileComp
		},
	)
}

func listTerminals(cmd *cobra.Command, _ []string) error {
	logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errdefs.ErrLoggerNotFound
	}

	logger.Debug("terminals list command invoked",
		"run_path", viper.GetString(config.RUN_PATH.ViperKey),
		"list_all", viper.GetBool(listAllInput),
		"args", cmd.Flags().Args(),
	)

	err := discovery.ScanAndPrintTerminals(
		cmd.Context(),
		logger,
		viper.GetString(config.RUN_PATH.ViperKey),
		os.Stdout,
		viper.GetBool(listAllInput),
	)
	if err != nil {
		logger.Debug("error scanning and printing terminals", "error", err)
		fmt.Fprintln(os.Stderr, "Could not scan terminals")
		return err
	}
	logger.Debug("terminals list completed successfully")
	return nil
}

func CompleteTerminals(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// If user set --id or --name, don't offer positional completions.
	if cmd.Flags().Changed("id") || cmd.Flags().Changed("name") {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	runPath, err := config.GetRunPathFromEnvAndFlags(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	names, err := fetchTerminalNames(cmd.Context(), runPath, toComplete)
	if err != nil {
		// Optionally show an error message in completion
		return []string{"__error: cannot list terminals"}, cobra.ShellCompDirectiveNoFileComp
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// Example source for names (replace with your real backend).
func fetchTerminalNames(ctx context.Context, runPath string, toComplete string) ([]string, error) {
	logger, _ := ctx.Value(types.CtxLogger).(*slog.Logger)

	all, err := config.AutoCompleteListTerminalNames(ctx, logger, runPath, false)
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

func getTerminal(cmd *cobra.Command, args []string) error {
	logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errdefs.ErrLoggerNotFound
	}
	terminalName := args[0]
	format := viper.GetString(outputFormat)
	if format != "" && format != "json" && format != "yaml" {
		return fmt.Errorf("%w: %s", errdefs.ErrInvalidOutputFormat, format)
	}
	logger.Debug("get terminal command invoked",
		"run_path", viper.GetString(config.RUN_PATH.ViperKey),
		"terminal_name", terminalName,
		"output_format", format,
		"args", cmd.Flags().Args(),
	)

	runPath, err := config.GetRunPathFromEnvAndFlags(cmd)
	if err != nil {
		return fmt.Errorf("%w: %w", errdefs.ErrGetRunPath, err)
	}
	return discovery.FindAndPrintTerminalMetadata(cmd.Context(), logger, runPath, os.Stdout, terminalName, format)
}

func ResolveTerminalNameToID(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	terminalName string,
) (string, error) {
	terminals, err := discovery.ScanTerminals(ctx, logger, runPath)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, "ResolveTerminalNameToID: failed to load terminals", "path", runPath, "error", err)
		}
		return "", err
	}
	if terminals == nil {
		return "", errdefs.ErrNoTerminalsFound
	}

	for _, t := range terminals {
		if t.Spec.Name == terminalName {
			return string(t.Spec.ID), nil
		}
	}

	return "", fmt.Errorf("%w: terminal with name '%s' not found", errdefs.ErrTerminalNotFound, terminalName)
}
