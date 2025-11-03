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
	outputFormatProfilesInput = "sb.get.profiles.output"
)

func NewGetProfilesCmd() *cobra.Command {
	// GetProfilesCmd represents the get profiles command.
	cmd := &cobra.Command{
		Use:          "profiles",
		Aliases:      []string{"profile", "prof", "pro", "p"},
		Short:        "Get profiles",
		Long:         "Get profiles from the sbsh environment.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				// If user passed -o when listing, reject it
				if cmd.Flags().Changed("output") {
					return fmt.Errorf(
						"%w: the -o/--output flag is only valid when specifying a profile name",
						errdefs.ErrInvalidFlag,
					)
				}
				return listProfiles(cmd, args)
			} else if len(args) > 1 {
				return errdefs.ErrTooManyArguments
			}

			return getProfile(cmd, args)
		},
		// POSitional completion for NAME
		ValidArgsFunction: completeProfiles,
	}

	setupNewGetProfilesCmd(cmd)
	return cmd
}

func setupNewGetProfilesCmd(cmd *cobra.Command) {
	cmd.Flags().StringP("output", "o", "", "Output format: json|yaml (default: human-readable)")
	_ = viper.BindPFlag(outputFormatProfilesInput, cmd.Flags().Lookup("output"))

	_ = cmd.RegisterFlagCompletionFunc(
		"output",
		func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return []string{"json", "yaml"}, cobra.ShellCompDirectiveNoFileComp
		},
	)
}

func listProfiles(cmd *cobra.Command, _ []string) error {
	logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errdefs.ErrLoggerNotFound
	}

	logger.Debug("profiles list command invoked",
		"profiles_file", viper.GetString(config.PROFILES_FILE.ViperKey),
		"run_path", viper.GetString(config.RUN_PATH.ViperKey),
		"list_all", listAllInput,
		"args", cmd.Flags().Args(),
	)

	err := discovery.ScanAndPrintProfiles(
		cmd.Context(),
		logger,
		viper.GetString(config.PROFILES_FILE.ViperKey),
		os.Stdout,
	)
	if err != nil {
		logger.Debug("error scanning and printing profiles", "error", err)
		fmt.Fprintln(os.Stderr, "Could not scan profiles")
		return err
	}
	logger.Debug("profiles list completed successfully")
	return nil
}

func completeProfiles(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	names, err := fetchProfileNames(cmd.Context(), viper.GetString(config.PROFILES_FILE.ViperKey), toComplete)
	if err != nil {
		// Optionally show an error message in completion
		return []string{"__error: cannot list profiles"}, cobra.ShellCompDirectiveNoFileComp
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}

// Example source for names (replace with your real backend).
func fetchProfileNames(ctx context.Context, basePath string, toComplete string) ([]string, error) {
	logger, _ := ctx.Value(types.CtxLogger).(*slog.Logger)

	all, err := config.AutoCompleteListProfileNames(ctx, logger, basePath)
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

func getProfile(cmd *cobra.Command, args []string) error {
	logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errdefs.ErrLoggerNotFound
	}
	profileName := args[0]
	format := viper.GetString(outputFormatProfilesInput)
	if format != "" && format != "json" && format != "yaml" {
		return fmt.Errorf("%w: %s", errdefs.ErrInvalidOutputFormat, format)
	}
	logger.Debug("get profile command invoked",
		"run_path", viper.GetString(config.RUN_PATH.ViperKey),
		"terminal_name", profileName,
		"output_format", format,
		"args", cmd.Flags().Args(),
	)

	return discovery.FindAndPrintProfileMetadata(
		cmd.Context(),
		logger,
		viper.GetString(config.PROFILES_FILE.ViperKey),
		os.Stdout,
		profileName,
		format,
	)
}
