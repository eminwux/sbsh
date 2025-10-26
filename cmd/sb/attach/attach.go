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
	"path/filepath"
	"syscall"
	"time"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/sb/get"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/logging"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/internal/supervisor"
	"github.com/eminwux/sbsh/pkg/api"
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
			logger, ok := cmd.Context().Value(logging.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
			}

			if len(args) == 1 {
				// If user passed -n when listing, reject it
				if cmd.Flags().Changed("id") {
					return errors.New("the --id flag is not valid when using positional terminal name")
				}
				if cmd.Flags().Changed("name") {
					return errors.New("the --name flag is not valid when using positional terminal name")
				}
			}
			logger.DebugContext(cmd.Context(), "attach command invoked",
				"args", cmd.Flags().Args(),
				"sb.attach.id", viper.GetString("sb.attach.id"),
				"sb.attach.name", viper.GetString("sb.attach.name"),
				"sb.attach.socket", viper.GetString("sb.attach.socket"),
				"run_path", viper.GetString(config.RUN_PATH.ViperKey),
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

func setupAttachCmdFlags(attachCmd *cobra.Command) {
	attachCmd.Flags().String("id", "", "Session ID, cannot be set together with --name")
	_ = viper.BindPFlag("sb.attach.id", attachCmd.Flags().Lookup("id"))

	attachCmd.Flags().StringP("name", "n", "", "Optional session name, cannot be set together with --id")
	_ = viper.BindPFlag("sb.attach.name", attachCmd.Flags().Lookup("name"))

	_ = attachCmd.RegisterFlagCompletionFunc(
		"id",
		func(c *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			//nolint:mnd // 150ms is a good compromise between snappy completion and enough time to read files
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			runPath, err := config.GetRunPathFromEnvAndFlags(c)
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
			runPath, err := config.GetRunPathFromEnvAndFlags(c)
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
	attachCmd.Flags().String("socket", "", "Optional socket file for the session")
	_ = viper.BindPFlag("sb.attach.socket", attachCmd.Flags().Lookup("socket"))
}

func run(
	cmd *cobra.Command,
	args []string,
) error {
	logger, ok := cmd.Context().Value(logging.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errors.New("logger not found in context")
	}

	// Top-level context also reacts to SIGINT/SIGTERM (nice UX)
	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	supervisorID := naming.RandomID()
	socketFileFlag := viper.GetString("sb.attach.socket")
	runPath := viper.GetString(config.RUN_PATH.ViperKey)

	var terminalNamePositional string
	if len(args) > 0 {
		terminalNamePositional = args[0]
	}

	terminalNameFlag := viper.GetString("sb.attach.name")
	terminalIDFlag := viper.GetString("sb.attach.id")

	terminalName := terminalNamePositional
	if terminalNamePositional == "" {
		terminalName = terminalNameFlag
	}
	// Create a new Controller

	logger.DebugContext(ctx, "creating supervisor controller for attach", "run_path", runPath)
	supCtrl := supervisor.NewSupervisorController(ctx, logger)

	if socketFileFlag == "" {
		socketFileFlag = filepath.Join(runPath, "supervisors", supervisorID, "socket")
	}

	if err := os.MkdirAll(filepath.Dir(socketFileFlag), 0o700); err != nil {
		logger.ErrorContext(ctx, "failed to create supervisor dir", "dir", filepath.Dir(socketFileFlag), "error", err)
		return fmt.Errorf("create supervisor dir %q: %w", filepath.Dir(socketFileFlag), err)
	}

	supSpec := buildSupervisorSpec(ctx, supervisorID, runPath, socketFileFlag, logger)

	if terminalIDFlag != "" {
		supSpec.SessionSpec.ID = api.ID(terminalIDFlag)
	}
	if terminalName != "" {
		supSpec.SessionSpec.Name = terminalName
	}

	var socket string
	if socketFileFlag != "" {
		socket = socketFileFlag
	} else {
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
				return fmt.Errorf("cannot resolve terminal name to ID: %w", errR)
			}
		default:
			logger.DebugContext(
				cmd.Context(),
				"no terminal identification method provided, cannot detach",
			)
		}

		socket = fmt.Sprintf("%s/sessions/%s/socket", runPath, terminalID)
	}

	supSpec.SessionSpec.SocketFile = socket

	logger.DebugContext(ctx, "Built supervisor spec", "supervisorSpec", fmt.Sprintf("%+v", supSpec))

	logger.DebugContext(ctx, "starting supervisor controller goroutine for attach")
	errCh := make(chan error, 1)
	go func() {
		errCh <- supCtrl.Run(supSpec)
		close(errCh)
		logger.DebugContext(ctx, "controller goroutine exited (attach)")
	}()

	logger.DebugContext(ctx, "waiting for supervisor controller to signal ready (attach)")
	if err := supCtrl.WaitReady(); err != nil {
		logger.DebugContext(ctx, "controller not ready (attach)", "error", err)
		return fmt.Errorf("%w: %w", errdefs.ErrWaitOnReady, err)
	}

	logger.DebugContext(ctx, "controller ready, entering attach event loop")
	select {
	case <-ctx.Done():
		logger.DebugContext(ctx, "context canceled, waiting for controller to exit (attach)")
		waitErr := supCtrl.WaitClose()
		if waitErr != nil {
			logger.DebugContext(ctx, "error waiting for controller to close after context canceled", "error", waitErr)
			return fmt.Errorf("%w: %w", errdefs.ErrWaitOnClose, waitErr)
		}
		logger.DebugContext(ctx, "context canceled, controller exited (attach)")
		return errdefs.ErrContextDone

	case ctrlErr := <-errCh:
		logger.DebugContext(ctx, "controller stopped (attach)", "error", ctrlErr)
		if ctrlErr != nil && !errors.Is(ctrlErr, context.Canceled) {
			waitErr := supCtrl.WaitClose()
			if waitErr != nil {
				logger.DebugContext(ctx, "error waiting for controller to close after error", "error", waitErr)
				return fmt.Errorf("%w: %w", errdefs.ErrWaitOnClose, waitErr)
			}
			logger.DebugContext(ctx, "controller exited after error (attach)")
			if errors.Is(ctrlErr, errdefs.ErrAttach) {
				logger.DebugContext(ctx, "attach error", "error", ctrlErr)
				fmt.Fprintf(os.Stderr, "Could not attach: %v\n", ctrlErr)
				cancel()
				//nolint:gocritic // os.Exit is fine here
				os.Exit(1)
			}
			// return nothing to avoid polluting the terminal with errors
			return nil
		}
	}
	return nil
}

func buildSupervisorSpec(
	ctx context.Context,
	supervisorID string,
	runPath string,
	socketFileInput string,
	logger *slog.Logger,
) *api.SupervisorSpec {
	var spec *api.SupervisorSpec

	supervisorName := naming.RandomName()
	spec = &api.SupervisorSpec{
		Kind:        api.AttachToSession,
		ID:          api.ID(supervisorID),
		Name:        supervisorName,
		RunPath:     runPath,
		SockerCtrl:  socketFileInput,
		SessionSpec: &api.SessionSpec{},
	}
	logger.DebugContext(ctx, "attach spec created",
		"kind", spec.Kind,
		"id", spec.ID,
		"name", spec.Name,
		"run_path", spec.RunPath,
	)

	return spec
}
