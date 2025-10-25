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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	listAllSupervisorsInput     = "sb.get.supervisors.all"
	outputFormatSupervisorInput = "sb.get.supervisors.output"
)

func NewGetSupervisorCmd() *cobra.Command {
	// GetSupervisorCmd represents the get supervisor command.
	cmd := &cobra.Command{
		Use:     "supervisor",
		Aliases: []string{"supervisors", "supers", "super", "s"},
		Short:   "Get a supervisor",
		Long:    "Get a supervisor from the sbsh environment.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				// If user passed -o when listing, reject it
				if cmd.Flags().Changed("output") {
					return errors.New("the -o/--output flag is only valid when specifying a supervisor name")
				}
				return listSupervisors(cmd, args)
			}

			return getSupervisor(cmd, args)
		},
		// POSitional completion for NAME
		ValidArgsFunction: completeSupervisors,
	}

	setupNewGetSupervisorCmd(cmd)
	return cmd
}

func setupNewGetSupervisorCmd(cmd *cobra.Command) {
	cmd.Flags().BoolP("all", "a", false, "List all supervisors, including Exited")
	_ = viper.BindPFlag(listAllSupervisorsInput, cmd.Flags().Lookup("all"))

	cmd.Flags().StringP("output", "o", "", "Output format: json|yaml (default: human-readable)")
	_ = viper.BindPFlag(outputFormatSupervisorInput, cmd.Flags().Lookup("output"))

	_ = cmd.RegisterFlagCompletionFunc(
		"output",
		func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return []string{"json", "yaml"}, cobra.ShellCompDirectiveNoFileComp
		},
	)
}

func listSupervisors(cmd *cobra.Command, _ []string) error {
	logger, ok := cmd.Context().Value(logging.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errors.New("logger not found in context")
	}

	logger.Debug("sessions list command invoked",
		"run_path", viper.GetString(config.RUN_PATH.ViperKey),
		"list_all", listAllInput,
		"args", cmd.Flags().Args(),
	)

	err := discovery.ScanAndPrintSupervisors(
		cmd.Context(),
		logger,
		viper.GetString(config.RUN_PATH.ViperKey),
		os.Stdout,
		viper.GetBool(listAllSupervisorsInput),
	)
	if err != nil {
		logger.Debug("error scanning and printing sessions", "error", err)
		fmt.Fprintln(os.Stderr, "Could not scan sessions")
		return err
	}
	logger.Debug("sessions list completed successfully")
	return nil
}

func completeSupervisors(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	runPath, err := config.GetRunPathFromEnvAndFlags(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	names, err := fetchSupervisorNames(cmd.Context(), runPath, toComplete)
	if err != nil {
		// Optionally show an error message in completion
		return []string{"__error: cannot list supervisors"}, cobra.ShellCompDirectiveNoFileComp
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// Example source for names (replace with your real backend).
func fetchSupervisorNames(ctx context.Context, runPath string, toComplete string) ([]string, error) {
	logger, _ := ctx.Value(logging.CtxLogger).(*slog.Logger)

	all, err := config.AutoCompleteListSupervisors(ctx, logger, runPath, false)
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

func getSupervisor(cmd *cobra.Command, args []string) error {
	logger, ok := cmd.Context().Value(logging.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errors.New("logger not found in context")
	}
	supervisorName := args[0]
	format := viper.GetString(outputFormatSupervisorInput)
	if format != "" && format != "json" && format != "yaml" {
		return fmt.Errorf("invalid output format: %s", format)
	}
	logger.Debug("get supervisor command invoked",
		"run_path", viper.GetString(config.RUN_PATH.ViperKey),
		"terminal_name", supervisorName,
		"output_format", format,
		"args", cmd.Flags().Args(),
	)

	runPath, err := config.GetRunPathFromEnvAndFlags(cmd)
	if err != nil {
		return errors.New("failed to get run path from env and flags")
	}
	return discovery.FindAndPrintSupervisorMetadata(cmd.Context(), logger, runPath, os.Stdout, supervisorName, format)
}
