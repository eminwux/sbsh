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
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/env"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/logging"
	"github.com/eminwux/sbsh/internal/naming"
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
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			sessionID := viper.GetString("sbsh.run.id")
			if sessionID == "" {
				sessionID = naming.RandomID()
				viper.Set("sbsh.run.id", sessionID)
			}

			sesLogfile := viper.GetString("sbsh.run.logFile")
			if sesLogfile == "" {
				sesLogfile = filepath.Join(
					viper.GetString(env.RUN_PATH.ViperKey),
					"sessions",
					sessionID,
					"log",
				)
			}

			sesLogLevel := viper.GetString("sbsh.run.logLevel")
			if sesLogLevel == "" {
				sesLogLevel = "info"
			}

			if sesLogfile != "" {
				if err := os.MkdirAll(filepath.Dir(sesLogfile), 0o700); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
					os.Exit(1)
				}

				f, err := os.OpenFile(sesLogfile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
					os.Exit(1)
				}

				// Create a new logger that writes to the file with the specified log level
				lv := new(slog.LevelVar)
				lv.Set(logging.ParseLevel(sesLogLevel))

				handler, ok := cmd.Context().Value(logging.CtxHandler).(*logging.ReformatHandler)
				if !ok || handler == nil {
					return errors.New("logger handler not found in context")
				}

				handler.Inner = slog.NewTextHandler(f, &slog.HandlerOptions{Level: lv})
				handler.Writer = f

				ctx := cmd.Context()
				ctx = context.WithValue(ctx, logging.CtxLevelVar, lv)
				ctx = context.WithValue(ctx, logging.CtxCloser, f)
				cmd.SetContext(ctx)
				return nil
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger, ok := cmd.Context().Value(logging.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
			}

			logger.Info("Starting sbsh run command")

			sessionSpec, buildErr := profile.BuildSessionSpec(
				cmd.Context(),
				logger,
				&profile.BuildSessionSpecParams{
					SessionID:    viper.GetString("sbsh.run.id"),
					SessionName:  viper.GetString("sbsh.run.name"),
					SessionCmd:   viper.GetString("sbsh.run.command"),
					CaptureFile:  viper.GetString("sbsh.run.captureFile"),
					RunPath:      viper.GetString(env.RUN_PATH.ViperKey),
					ProfilesFile: viper.GetString(env.PROFILES_FILE.ViperKey),
					ProfileName:  viper.GetString("sbsh.run.profile"),
					LogFile:      viper.GetString("sbsh.run.logFile"),
					LogLevel:     viper.GetString("sbsh.run.logLevel"),
					SocketFile:   viper.GetString("sbsh.run.socket"),
					EnvVars:      os.Environ(),
				},
			)

			if buildErr != nil {
				logger.Error("Failed to build session spec", "error", buildErr)
				return buildErr
			}

			logger.Debug("Built session spec", "sessionSpec", fmt.Sprintf("%+v", sessionSpec))

			if logger.Enabled(context.Background(), slog.LevelDebug) {
				logger.DebugContext(cmd.Context(), "printing session spec for debug")
				if printErr := discovery.PrintSessionSpec(sessionSpec, logger); printErr != nil {
					logger.Warn("Failed to print session spec", "error", printErr)
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			ctrl := session.NewSessionController(ctx, logger)

			logger.Info("Starting session controller")
			runErr := runSession(ctx, cancel, logger, ctrl, sessionSpec)
			if runErr != nil {
				logger.Error("Session controller returned error", "error", runErr)
				return runErr
			}
			logger.Info("Session controller completed successfully")
			return nil
		},
		PostRunE: func(cmd *cobra.Command, _ []string) error {
			if c, _ := cmd.Context().Value(logging.CtxCloser).(io.Closer); c != nil {
				_ = c.Close()
			}
			return nil
		},
	}

	setupRunCmdFlags(runCmd)
	return runCmd
}

func setupRunCmdFlags(runCmd *cobra.Command) {
	runCmd.Flags().String("id", "", "Optional session ID (random if omitted)")
	_ = viper.BindPFlag("sbsh.run.id", runCmd.Flags().Lookup("id"))

	runCmd.Flags().String("command", "", "Optional command (default: /bin/bash)")
	_ = viper.BindPFlag("sbsh.run.command", runCmd.Flags().Lookup("command"))

	runCmd.Flags().String("name", "", "Optional name for the session")
	_ = viper.BindPFlag("sbsh.run.name", runCmd.Flags().Lookup("name"))

	runCmd.Flags().String("session-log", "", "Optional filename for the session log")
	_ = viper.BindPFlag("sbsh.run.sessionLog", runCmd.Flags().Lookup("session-log"))

	runCmd.Flags().String("log-file", "", "Optional filename for the session log")
	_ = viper.BindPFlag("sbsh.run.logFile", runCmd.Flags().Lookup("log-file"))

	runCmd.Flags().String("log-level", "", "Optional log level for the session")
	_ = viper.BindPFlag("sbsh.run.logLevel", runCmd.Flags().Lookup("log-level"))

	runCmd.Flags().StringP("profile", "p", "", "Optional profile for the session")
	_ = viper.BindPFlag("sbsh.run.profile", runCmd.Flags().Lookup("profile"))
	_ = viper.BindEnv("sbsh.run.profile", env.SES_PROFILE.EnvVar())

	runCmd.Flags().String("socket", "", "Optional socket file for the session")
	_ = viper.BindPFlag("sbsh.run.socket", runCmd.Flags().Lookup("socket"))
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
			logger.DebugContext(ctx, "error waiting for session controller to close", "error", waitErr)
		}
		logger.DebugContext(ctx, "context canceled, session controller exited")
		return nil

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
