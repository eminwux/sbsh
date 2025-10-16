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
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger, ok := cmd.Context().Value(logging.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
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
			sessionID := viper.GetString("sb.attach.id")
			sessionName := viper.GetString("sb.attach.name")
			runPath := viper.GetString(config.RUN_PATH.ViperKey)
			socketFile := viper.GetString("sb.attach.socket")

			if sessionID == "" && sessionName == "" {
				return errors.New("either --id or --name must be defined")
			}
			if sessionID != "" && sessionName != "" {
				return errors.New("only one of --id or --name must be defined")
			}

			return run(cmd.Context(), logger, sessionID, sessionName, runPath, socketFile)
		},
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
			profs, err := config.AutoCompleteListSessions(ctx, nil, runPath)
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
	parentCtx context.Context,
	logger *slog.Logger,
	sessionID string,
	sessionName string,
	runPath string,
	socketFileInput string,
) error {
	// Top-level context also reacts to SIGINT/SIGTERM (nice UX)
	ctx, cancel := signal.NotifyContext(parentCtx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create a new Controller

	logger.DebugContext(ctx, "creating supervisor controller for attach", "run_path", runPath)
	supCtrl := supervisor.NewSupervisorController(ctx, logger)

	supervisorID := naming.RandomID()

	if socketFileInput == "" {
		socketFileInput = filepath.Join(runPath, "supervisor", supervisorID, "socket")
	}

	if err := os.MkdirAll(filepath.Dir(socketFileInput), 0o700); err != nil {
		logger.ErrorContext(ctx, "failed to create supervisor dir", "dir", filepath.Dir(socketFileInput), "error", err)
		return fmt.Errorf("create supervisor dir %q: %w", filepath.Dir(socketFileInput), err)
	}

	supSpec := buildSupervisorSpec(ctx, supervisorID, sessionID, sessionName, runPath, socketFileInput, logger)

	if socketFileInput != "" {
		supSpec.SessionSpec.SocketFile = socketFileInput
	} else {
		supSpec.SessionSpec.SocketFile = filepath.Join(runPath, ".sbsh", "sessions", string(supSpec.SessionSpec.ID), "socket")
	}

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
	sessionID string,
	sessionName string,
	runPath string,
	socketFileInput string,
	logger *slog.Logger,
) *api.SupervisorSpec {
	var spec *api.SupervisorSpec

	supervisorName := naming.RandomName()
	if sessionID != "" && sessionName == "" {
		spec = &api.SupervisorSpec{
			Kind:       api.AttachToSession,
			ID:         api.ID(supervisorID),
			Name:       supervisorName,
			RunPath:    runPath,
			SockerCtrl: socketFileInput,
			AttachID:   api.ID(sessionID),
			SessionSpec: &api.SessionSpec{
				ID: api.ID(sessionID),
			},
		}
		logger.DebugContext(ctx, "attach spec (by id) created",
			"kind", spec.Kind,
			"id", spec.ID,
			"name", spec.Name,
			"run_path", spec.RunPath,
			"attach_id", spec.AttachID,
			"session_id", spec.SessionSpec.ID,
		)
	}

	if sessionID == "" && sessionName != "" {
		spec = &api.SupervisorSpec{
			Kind:       api.AttachToSession,
			ID:         api.ID(naming.RandomID()),
			Name:       naming.RandomName(),
			LogFile:    "/tmp/sbsh-logs/s0",
			RunPath:    runPath,
			SockerCtrl: socketFileInput,
			AttachName: sessionName,
			SessionSpec: &api.SessionSpec{
				Name: sessionName,
			},
		}
		logger.DebugContext(ctx, "attach spec (by name) created",
			"kind", spec.Kind,
			"id", spec.ID,
			"name", spec.Name,
			"log_dir", spec.LogFile,
			"run_path", spec.RunPath,
			"session_name", spec.SessionSpec.Name,
		)
	}
	return spec
}
