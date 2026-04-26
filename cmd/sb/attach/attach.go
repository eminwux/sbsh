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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/sb/get"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/attach"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	Command      = "attach"
	CommandAlias = "a"
)

func NewAttachCmd() *cobra.Command {
	// runCmd represents the run command
	attachCmd := &cobra.Command{
		Use:     Command,
		Aliases: []string{CommandAlias},
		Short:   "A brief description of your command",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errdefs.ErrLoggerNotFound
			}

			if err := validateAttachArgs(cmd, args); err != nil {
				return err
			}

			logger.DebugContext(cmd.Context(), "attach command invoked",
				"args", cmd.Flags().Args(),
				config.SB_ATTACH_ID.ViperKey, viper.GetString(config.SB_ATTACH_ID.ViperKey),
				config.SB_ATTACH_NAME.ViperKey, viper.GetString(config.SB_ATTACH_NAME.ViperKey),
				config.SB_ATTACH_SOCKET.ViperKey, viper.GetString(config.SB_ATTACH_SOCKET.ViperKey),
				"run_path", viper.GetString(config.SB_ROOT_RUN_PATH.ViperKey),
			)
			cmd.Flags().VisitAll(func(f *pflag.Flag) {
				logger.DebugContext(cmd.Context(), "flag value", "name", f.Name, "value", f.Value.String())
			})
			cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) {
				logger.DebugContext(cmd.Context(), "inherited flag value", "name", f.Name, "value", f.Value.String())
			})

			return run(cmd, args)
		},
		ValidArgsFunction: get.CompleteTerminals,
	}

	setupAttachCmdFlags(attachCmd)
	return attachCmd
}

// validateAttachArgs enforces the positional/flag contract for
// `sb attach`: zero or one positional terminal identifier, optionally
// replaced by --id or --name; the flags are mutually exclusive with a
// positional arg but are themselves a valid way to identify the target
// (so library callers like pkg/spawn can use --id alone).
func validateAttachArgs(cmd *cobra.Command, args []string) error {
	switch {
	case len(args) == 0:
		if !cmd.Flags().Changed("id") && !cmd.Flags().Changed("name") {
			return errdefs.ErrNoTerminalIdentifier
		}
	case len(args) == 1:
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
	case len(args) > 1:
		return errdefs.ErrTooManyArguments
	}
	return nil
}

func setupAttachCmdFlags(attachCmd *cobra.Command) {
	attachCmd.Flags().String("id", "", "Terminal ID, cannot be set together with --name")
	_ = viper.BindPFlag(config.SB_ATTACH_ID.ViperKey, attachCmd.Flags().Lookup("id"))

	attachCmd.Flags().StringP("name", "n", "", "Optional terminal name, cannot be set together with --id")
	_ = viper.BindPFlag(config.SB_ATTACH_NAME.ViperKey, attachCmd.Flags().Lookup("name"))

	_ = attachCmd.RegisterFlagCompletionFunc(
		"id",
		func(c *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			//nolint:mnd // 150ms is a good compromise between snappy completion and enough time to read files
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			runPath, err := config.GetRunPathFromEnvAndFlags(c, config.SB_ROOT_RUN_PATH.EnvVar())
			if err != nil {
				// fail silent to keep completion snappy
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			profs, err := config.AutoCompleteListTerminalIDs(ctx, nil, runPath, false)
			if err != nil {
				// fail silent to keep completion snappy
				return nil, cobra.ShellCompDirectiveNoFileComp
			}

			// Optionally add descriptions: "value\tpath" for nicer columns
			return profs, cobra.ShellCompDirectiveNoFileComp
		},
	)

	_ = attachCmd.RegisterFlagCompletionFunc(
		"name",
		func(c *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			//nolint:mnd // 150ms is a good compromise between snappy completion and enough time to read files
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			runPath, err := config.GetRunPathFromEnvAndFlags(c, config.SB_ROOT_RUN_PATH.EnvVar())
			if err != nil {
				// fail silent to keep completion snappy
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			profs, err := config.AutoCompleteListTerminalNames(ctx, nil, runPath, false)
			if err != nil {
				// fail silent to keep completion snappy
				return nil, cobra.ShellCompDirectiveNoFileComp
			}

			// Optionally add descriptions: "value\tpath" for nicer columns
			return profs, cobra.ShellCompDirectiveNoFileComp
		},
	)
	attachCmd.Flags().String("socket", "", "Optional socket file for the terminal")
	_ = viper.BindPFlag(config.SB_ATTACH_SOCKET.ViperKey, attachCmd.Flags().Lookup("socket"))
	attachCmd.Flags().Bool("disable-detach", false, "Disable detach keystroke (^] twice)")
	_ = viper.BindPFlag(config.SB_ATTACH_DISABLE_DETACH_KEYSTROKE.ViperKey, attachCmd.Flags().Lookup("disable-detach"))
}

func run(
	cmd *cobra.Command,
	args []string,
) error {
	logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errdefs.ErrLoggerNotFound
	}

	// Top-level context also reacts to SIGINT/SIGTERM (nice UX). pkg/attach
	// deliberately does not trap signals itself; the CLI owns that policy.
	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	socketPath, err := resolveTerminalSocket(cmd, logger, args)
	if err != nil {
		return err
	}

	disableDetach := viper.GetBool(config.SB_ATTACH_DISABLE_DETACH_KEYSTROKE.ViperKey)

	logger.DebugContext(ctx, "delegating to pkg/attach.Run", "socket", socketPath, "disable_detach", disableDetach)

	runErr := attach.Run(ctx, attach.Options{
		SocketPath:             socketPath,
		Stdin:                  os.Stdin,
		Stdout:                 os.Stdout,
		Stderr:                 os.Stderr,
		DisableDetachKeystroke: disableDetach,
		Logger:                 logger,
	})
	if runErr == nil || errors.Is(runErr, context.Canceled) || errors.Is(runErr, errdefs.ErrContextDone) {
		return nil
	}
	if errors.Is(runErr, errdefs.ErrAttach) {
		logger.DebugContext(ctx, "attach error", "error", runErr)
		fmt.Fprintf(os.Stderr, "Could not attach: %v\n", runErr)
		cancel()
		//nolint:gocritic // os.Exit is fine here
		os.Exit(1)
	}
	// Other errors are intentionally swallowed to avoid polluting the
	// terminal — preserves prior `sb attach` UX.
	logger.DebugContext(ctx, "attach loop exited with error", "error", runErr)
	return nil
}

// resolveTerminalSocket maps the user's positional/--id/--name/--socket
// arguments into the absolute path of the target terminal's control
// socket. Centralised here so pkg/attach stays free of CLI / discovery
// concerns.
func resolveTerminalSocket(cmd *cobra.Command, logger *slog.Logger, args []string) (string, error) {
	socketFileFlag := viper.GetString(config.SB_ATTACH_SOCKET.ViperKey)
	if socketFileFlag != "" {
		return socketFileFlag, nil
	}

	runPath := viper.GetString(config.SB_ROOT_RUN_PATH.ViperKey)
	terminalIDFlag := viper.GetString(config.SB_ATTACH_ID.ViperKey)
	terminalNameFlag := viper.GetString(config.SB_ATTACH_NAME.ViperKey)

	terminalName := terminalNameFlag
	if len(args) > 0 {
		terminalName = args[0]
	}

	var terminalID string
	switch {
	case terminalIDFlag != "":
		terminalID = terminalIDFlag
	case terminalName != "":
		var errR error
		terminalID, errR = get.ResolveTerminalNameToID(cmd.Context(), logger, runPath, terminalName)
		if errR != nil {
			logger.ErrorContext(
				cmd.Context(),
				"cannot resolve terminal name to ID",
				"terminal_name",
				terminalName,
				"error",
				errR,
			)
			return "", fmt.Errorf("%w: %w", errdefs.ErrResolveTerminalName, errR)
		}
	default:
		logger.DebugContext(
			cmd.Context(),
			"no terminal identification method provided, cannot attach",
		)
		return "", errdefs.ErrNoTerminalIdentification
	}

	return fmt.Sprintf("%s/%s/%s/socket", runPath, defaults.TerminalsRunPath, terminalID), nil
}
