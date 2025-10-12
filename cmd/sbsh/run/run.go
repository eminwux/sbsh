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

package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/env"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/profile"
	"github.com/eminwux/sbsh/internal/session"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	Command      string = "run"
	CommandAlias string = "r"
)

func NewRunCmd() *cobra.Command {
	// runCmd represents the run command.
	runCmd := &cobra.Command{
		Use:     Command,
		Aliases: []string{CommandAlias},
		Short:   "Run a new sbsh session",
		Long: `Run a new sbsh session.
The session can be customized via command-line options or by specifying a profile.
If no profile is specified, a default profile is used with the provided command or /bin/bash.

Examples:
  sbsh run --name mysession --command "/bin/zsh"
  sbsh run --profile devprofile
  sbsh run --profile devprofile --name customname --command "/usr/bin/fish"

If no session name is provided, a random name will be generated.
If no command is provided, /bin/bash will be used by default.
If no log filename is provided, a default path under the run directory will be used.
`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger, ok := cmd.Context().Value("logger").(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
			}

			logger.Info("Starting sbsh run command")
			logger.DebugContext(cmd.Context(), "building session spec from viper and flags",
				"run_path", viper.GetString(env.RUN_PATH.ViperKey),
				"profiles_file", viper.GetString(env.PROFILES_FILE.ViperKey),
				"profile", viper.GetString("run.session.profile"),
				"id", viper.GetString("run.session.id"),
				"name", viper.GetString("run.session.name"),
				"command", viper.GetString("run.session.command"),
				"logFilename", viper.GetString("run.session.logFilename"),
				"socket", viper.GetString("run.session.socket"),
			)

			sessionSpec, buildErr := profile.BuildSessionSpec(
				viper.GetString(env.RUN_PATH.ViperKey),
				viper.GetString(env.PROFILES_FILE.ViperKey),
				viper.GetString("run.session.profile"),
				viper.GetString("run.session.id"),
				viper.GetString("run.session.name"),
				viper.GetString("run.session.command"),
				viper.GetString("run.session.logFilename"),
				viper.GetString("run.session.socket"),
				os.Environ(),
				context.Background(),
			)
			if buildErr != nil {
				logger.Error("Failed to build session spec", "error", buildErr)
				return buildErr
			}

			logger.Info("Session spec built", "session_name", sessionSpec.Name, "session_id", sessionSpec.ID)

			profileNameInput := viper.GetString("session.profile")
			if profileNameInput != "" {
				logger.Info("Setting SBSH_SES_PROFILE env var", "profile", profileNameInput)
				if setErr := env.SES_PROFILE.Set(profileNameInput); setErr != nil {
					logger.Error("Failed to set SBSH_SES_PROFILE", "error", setErr)
					return setErr
				}
				if bindErr := env.SES_PROFILE.BindEnv(); bindErr != nil {
					logger.Error("Failed to bind SBSH_SES_PROFILE env", "error", bindErr)
					return bindErr
				}
			}

			if logger.Enabled(context.Background(), slog.LevelDebug) {
				logger.DebugContext(cmd.Context(), "printing session spec for debug")
				if printErr := discovery.PrintSessionSpec(sessionSpec, os.Stdout); printErr != nil {
					logger.Warn("Failed to print session spec", "error", printErr)
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			ctrl := session.NewSessionController(ctx)

			logger.Info("Starting session controller")
			runErr := runSession(ctx, cancel, logger, ctrl, sessionSpec)
			if runErr != nil {
				logger.Error("Session controller returned error", "error", runErr)
				return runErr
			}
			logger.Info("Session controller completed successfully")
			return nil
		},
	}

	setupRunCmdFlags(runCmd)
	return runCmd
}

func setupRunCmdFlags(runCmd *cobra.Command) {
	runCmd.Flags().String("id", "", "Optional session ID (random if omitted)")
	runCmd.Flags().String("command", "", "Optional command (default: /bin/bash)")
	runCmd.Flags().String("name", "", "Optional name for the session")
	runCmd.Flags().String("log-filename", "", "Optional filename for the session log")
	runCmd.Flags().String("profile", "", "Optional profile for the session")
	runCmd.Flags().String("socket", "", "Optional socket file for the session")

	_ = viper.BindPFlag("run.session.id", runCmd.Flags().Lookup("id"))
	_ = viper.BindPFlag("run.session.command", runCmd.Flags().Lookup("command"))
	_ = viper.BindPFlag("run.session.name", runCmd.Flags().Lookup("name"))
	_ = viper.BindPFlag("run.session.logFilename", runCmd.Flags().Lookup("log-filename"))
	_ = viper.BindPFlag("run.session.profile", runCmd.Flags().Lookup("profile"))
	_ = viper.BindPFlag("run.session.socket", runCmd.Flags().Lookup("socket"))
}

func runSession(
	ctx context.Context,
	cancel context.CancelFunc,
	logger *slog.Logger,
	ctrl api.SessionController,
	spec *api.SessionSpec,
) error {
	defer cancel()

	logger.DebugContext(ctx, "starting session controller goroutine", "session_name", spec.Name, "session_id", spec.ID)
	errCh := make(chan error, 1)
	go func() {
		errCh <- ctrl.Run(spec)
		close(errCh)
		logger.DebugContext(ctx, "session controller goroutine exited")
	}()

	logger.DebugContext(ctx, "waiting for session controller to signal ready")
	if err := ctrl.WaitReady(); err != nil {
		logger.DebugContext(ctx, "session controller not ready", "error", err)
		return fmt.Errorf("%w: %w", errdefs.ErrWaitOnReady, err)
	}

	logger.DebugContext(ctx, "session controller ready, entering event loop")
	select {
	case <-ctx.Done():
		logger.DebugContext(ctx, "context canceled, waiting for session controller to exit")
		var waitErr error
		if errC := ctrl.WaitClose(); errC != nil {
			waitErr = fmt.Errorf("%w %w: %w", waitErr, errdefs.ErrWaitOnClose, errC)
		}
		logger.DebugContext(ctx, "context canceled, session controller exited")
		return fmt.Errorf("%w: %w", errdefs.ErrContextDone, waitErr)

	case err := <-errCh:
		logger.DebugContext(ctx, "session controller stopped", "error", err)
		if err != nil && !errors.Is(err, context.Canceled) {
			childErr := fmt.Errorf("%w: %w", errdefs.ErrChildExit, err)
			if errC := ctrl.WaitClose(); errC != nil {
				_ = fmt.Errorf("%w: %w: %w", childErr, errdefs.ErrWaitOnClose, errC)
			}
			logger.DebugContext(ctx, "session controller exited after error")
			// return nothing to avoid polluting the terminal with errors
			return nil
		}
	}
	return nil
}
