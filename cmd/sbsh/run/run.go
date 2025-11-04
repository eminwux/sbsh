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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/discovery"
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

func checkFlag(cmd *cobra.Command, flag string, err string) error {
	if cmd.Flags().Changed(flag) {
		return fmt.Errorf("%w: the --%s flag is not valid when using %s", errdefs.ErrInvalidFlag, flag, err)
	}
	return nil
}

func checkStdInUsage(cmd *cobra.Command, _ []string) error {
	errorMessage := "positional argument '-'"
	flagsToCheck := []string{"id", "name", "command", "profile", "log-file", "log-level", "socket", "file"}
	for _, flag := range flagsToCheck {
		if err := checkFlag(cmd, flag, errorMessage); err != nil {
			return err
		}
	}
	return nil
}

func checkFileUsage(cmd *cobra.Command, _ []string) error {
	errorMessage := "the --file flag"
	flagsToCheck := []string{"id", "name", "command", "profile", "log-file", "log-level", "socket"}
	for _, flag := range flagsToCheck {
		if err := checkFlag(cmd, flag, errorMessage); err != nil {
			return err
		}
	}
	return nil
}

func buildSessionSpecFromFlags(cmd *cobra.Command, logger *slog.Logger) (*api.SessionSpec, error) {
	spec, buildErr := profile.BuildSessionSpec(
		cmd.Context(),
		logger,
		&profile.BuildSessionSpecParams{
			SessionID:    viper.GetString("sbsh.run.id"),
			SessionName:  viper.GetString("sbsh.run.name"),
			SessionCmd:   viper.GetString("sbsh.run.command"),
			CaptureFile:  viper.GetString("sbsh.run.captureFile"),
			RunPath:      viper.GetString(config.RUN_PATH.ViperKey),
			ProfilesFile: viper.GetString(config.PROFILES_FILE.ViperKey),
			ProfileName:  viper.GetString("sbsh.run.profile"),
			LogFile:      viper.GetString("sbsh.run.logFile"),
			LogLevel:     viper.GetString("sbsh.run.logLevel"),
			SocketFile:   viper.GetString("sbsh.run.socket"),
		},
	)

	if buildErr != nil {
		logger.Error("Failed to build session spec", "error", buildErr)
		return nil, buildErr
	}

	return spec, nil
}

func processInput(cmd *cobra.Command, args []string) (*api.SessionSpec, error) {
	var r io.Reader
	var f *os.File

	specFileFlag := viper.GetString("sbsh.run.spec")
	if len(args) > 0 && args[0] == "-" {
		// Use stdin as input
		errCheckStdin := checkStdInUsage(cmd, args)
		if errCheckStdin != nil {
			return nil, errCheckStdin
		}
		fi, errIn := os.Stdin.Stat()
		if errIn != nil {
			return nil, fmt.Errorf("%w: %w", errdefs.ErrStdinStat, errIn)
		}
		if (fi.Mode() & os.ModeCharDevice) != 0 {
			return nil, errdefs.ErrStdinEmpty
		}
		r = os.Stdin
	}
	if len(args) > 0 && args[0] != "-" {
		return nil, fmt.Errorf(
			"%w: the only accepted positional argument is '-' to read the session spec from stdin",
			errdefs.ErrInvalidArgument,
		)
	}

	if len(args) == 0 && specFileFlag != "" {
		// Spec file provided
		errCheckStdin := checkFileUsage(cmd, args)
		if errCheckStdin != nil {
			return nil, errCheckStdin
		}
		var err error
		f, err = os.Open(specFileFlag)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errdefs.ErrOpenSpecFile, err)
		}
		defer f.Close()
		r = f
	}

	if r == nil {
		// No stdin or file input
		return nil, errdefs.ErrNoSpecDefined
	}

	spec := api.SessionSpec{}
	dec := json.NewDecoder(r)
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("%w: %w", errdefs.ErrInvalidJSONSpec, err)
	}

	return &spec, nil
}

func setLoggingVarsFromFlags() {
	sessionID := viper.GetString("sbsh.run.id")
	if sessionID == "" {
		sessionID = naming.RandomID()
		viper.Set("sbsh.run.id", sessionID)
	}

	sesLogLevel := viper.GetString("sbsh.run.logLevel")
	if sesLogLevel == "" {
		sesLogLevel = "info"
		viper.Set("sbsh.run.logLevel", sesLogLevel)
	}

	sesLogfile := viper.GetString("sbsh.run.logFile")
	if sesLogfile == "" {
		sesLogfile = filepath.Join(
			viper.GetString(config.RUN_PATH.ViperKey),
			"sessions",
			sessionID,
			"log",
		)
		viper.Set("sbsh.run.logFile", sesLogfile)
	}
}

func processSpec(cmd *cobra.Command, spec **api.SessionSpec) error {
	// Check if spec is already provided
	if *spec != nil {
		// Spec provided via stdin
		err := logging.SetupFileLogger(cmd, (*spec).LogFile, (*spec).LogLevel)
		if err != nil {
			return err
		}

		logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
		if !ok || logger == nil {
			return errdefs.ErrLoggerNotFound
		}
		return nil
	}
	// No spec provided, build from flags
	// Set logging vars from flags
	setLoggingVarsFromFlags()

	// Setup logger
	err := logging.SetupFileLogger(cmd, viper.GetString("sbsh.run.logFile"), viper.GetString("sbsh.run.logLevel"))
	if err != nil {
		return err
	}

	// Retrieve logger from context
	logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errdefs.ErrLoggerNotFound
	}

	// Build spec from flags
	specBuilt, err := buildSessionSpecFromFlags(cmd, logger)
	if err != nil {
		return fmt.Errorf("%w: %w", errdefs.ErrBuildSessionSpec, err)
	}
	// Set built spec in context
	*spec = specBuilt

	jsonBytes, err := json.MarshalIndent(specBuilt, "", "  ")
	if err != nil {
		logger.Warn("failed to marshal session spec to JSON for debug", "error", err)
	} else {
		logger.Debug("Built session spec JSON", "sessionSpecJson", string(jsonBytes))
	}
	return nil
}

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
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Check if first argument indicates stdin usage
			var spec *api.SessionSpec

			// Process stdin input if '-' is provided
			spec, errProcess := processInput(cmd, args)
			if errProcess != nil && !errors.Is(errProcess, errdefs.ErrNoSpecDefined) {
				return errProcess
			}

			// Process spec (from stdin or flags)
			errProcessSpec := processSpec(cmd, &spec)
			if errProcessSpec != nil {
				return errProcessSpec
			}

			// Set session spec in context
			newCtx := context.WithValue(cmd.Context(), types.CtxSessionSpec, spec)
			cmd.SetContext(newCtx)

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errdefs.ErrLoggerNotFound
			}

			logger.Info("Starting sbsh run command")

			// Retrieve session spec from context
			sessionSpec, ok := cmd.Context().Value(types.CtxSessionSpec).(*api.SessionSpec)
			if !ok || sessionSpec == nil {
				return errdefs.ErrSessionSpecNotFound
			}

			logger.Debug("Built session spec", "sessionSpec", fmt.Sprintf("%+v", sessionSpec))

			if logger.Enabled(context.Background(), slog.LevelDebug) {
				logger.DebugContext(cmd.Context(), "printing session spec for debug")
				if printErr := discovery.PrintTerminalSpec(sessionSpec, logger); printErr != nil {
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
			if c, _ := cmd.Context().Value(types.CtxCloser).(io.Closer); c != nil {
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
	_ = viper.BindEnv("sbsh.run.profile", config.SES_PROFILE.EnvVar())

	runCmd.Flags().String("socket", "", "Optional socket file for the session")
	_ = viper.BindPFlag("sbsh.run.socket", runCmd.Flags().Lookup("socket"))

	runCmd.Flags().StringP("file", "f", "", "Optional JSON file with the session spec (use '-' for stdin)")
	_ = viper.BindPFlag("sbsh.run.spec", runCmd.Flags().Lookup("file"))
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
