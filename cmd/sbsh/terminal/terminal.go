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

package terminal

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
	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/logging"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/internal/profile"
	"github.com/eminwux/sbsh/internal/terminal"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	Command      string = "terminal"
	CommandAlias string = "t"
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

func buildTerminalSpecFromFlags(cmd *cobra.Command, logger *slog.Logger) (*api.TerminalSpec, error) {
	spec, buildErr := profile.BuildTerminalSpec(
		cmd.Context(),
		logger,
		&profile.BuildTerminalSpecParams{
			TerminalID:   viper.GetString(config.SBSH_TERM_ID.ViperKey),
			TerminalName: viper.GetString(config.SBSH_TERM_NAME.ViperKey),
			TerminalCmd:  viper.GetString(config.SBSH_TERM_COMMAND.ViperKey),
			CaptureFile:  viper.GetString(config.SBSH_TERM_CAPTURE_FILE.ViperKey),
			RunPath:      viper.GetString(config.SB_ROOT_RUN_PATH.ViperKey),
			ProfilesFile: viper.GetString(config.SBSH_ROOT_PROFILES_FILE.ViperKey),
			ProfileName:  viper.GetString(config.SBSH_TERM_PROFILE.ViperKey),
			LogFile:      viper.GetString(config.SBSH_TERM_LOG_FILE.ViperKey),
			LogLevel:     viper.GetString(config.SBSH_TERM_LOG_LEVEL.ViperKey),
			SocketFile:   viper.GetString(config.SBSH_TERM_SOCKET.ViperKey),
		},
	)

	if buildErr != nil {
		logger.Error("Failed to build terminal spec", "error", buildErr)
		return nil, buildErr
	}

	return spec, nil
}

func processInput(cmd *cobra.Command, args []string) (*api.TerminalSpec, error) {
	var r io.Reader
	var f *os.File

	specFileFlag := viper.GetString(config.SBSH_TERM_SPEC.ViperKey)
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
			"%w: the only accepted positional argument is '-' to read the terminal spec from stdin",
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

	spec := api.TerminalSpec{}
	dec := json.NewDecoder(r)
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("%w: %w", errdefs.ErrInvalidJSONSpec, err)
	}

	return &spec, nil
}

func setLoggingVarsFromFlags() {
	terminalID := viper.GetString(config.SBSH_TERM_ID.ViperKey)
	if terminalID == "" {
		terminalID = naming.RandomID()
		viper.Set(config.SBSH_TERM_ID.ViperKey, terminalID)
	}

	sesLogLevel := viper.GetString(config.SBSH_TERM_LOG_LEVEL.ViperKey)
	if sesLogLevel == "" {
		sesLogLevel = "info"
		viper.Set(config.SBSH_TERM_LOG_LEVEL.ViperKey, sesLogLevel)
	}

	sesLogfile := viper.GetString(config.SBSH_TERM_LOG_FILE.ViperKey)
	if sesLogfile == "" {
		sesLogfile = filepath.Join(
			viper.GetString(config.SB_ROOT_RUN_PATH.ViperKey),
			defaults.TerminalsRunPath,
			terminalID,
			"log",
		)
		viper.Set(config.SBSH_TERM_LOG_FILE.ViperKey, sesLogfile)
	}
}

func processSpec(cmd *cobra.Command, spec **api.TerminalSpec) error {
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
	err := logging.SetupFileLogger(
		cmd,
		viper.GetString(config.SBSH_TERM_LOG_FILE.ViperKey),
		viper.GetString(config.SBSH_TERM_LOG_LEVEL.ViperKey),
	)
	if err != nil {
		return err
	}

	// Retrieve logger from context
	logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errdefs.ErrLoggerNotFound
	}

	// Build spec from flags
	specBuilt, err := buildTerminalSpecFromFlags(cmd, logger)
	if err != nil {
		return fmt.Errorf("%w: %w", errdefs.ErrBuildTerminalSpec, err)
	}
	// Set built spec in context
	*spec = specBuilt

	jsonBytes, err := json.MarshalIndent(specBuilt, "", "  ")
	if err != nil {
		logger.Warn("failed to marshal terminal spec to JSON for debug", "error", err)
	} else {
		logger.Debug("Built terminal spec JSON", "terminalSpecJson", string(jsonBytes))
	}
	return nil
}

func NewTerminalCmd() *cobra.Command {
	// terminalCmd represents the terminal command.
	terminalCmd := &cobra.Command{
		Use:     Command,
		Aliases: []string{CommandAlias},
		Short:   "Run a new sbsh terminal",
		Long: `Run a new sbsh terminal.
The terminal can be customized via command-line options or by specifying a profile.
If no profile is specified, a default profile is used with the provided command or /bin/bash.

Examples:
  sbsh terminal --name myterminal --command "/bin/zsh"
  sbsh terminal --profile devprofile
  sbsh terminal --profile devprofile --name customname --command "/usr/bin/fish"

If no terminal name is provided, a random name will be generated.
If no command is provided, /bin/bash will be used by default.
If no log filename is provided, a default path under the run directory will be used.
`,
		SilenceUsage: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			var runPath string
			if viper.GetString(config.SB_ROOT_RUN_PATH.ViperKey) == "" {
				runPath = config.DefaultRunPath()
			}
			_ = config.SB_ROOT_RUN_PATH.BindEnv()
			config.SB_ROOT_RUN_PATH.SetDefault(runPath)

			// Check if first argument indicates stdin usage
			var spec *api.TerminalSpec

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

			// Set terminal spec in context
			newCtx := context.WithValue(cmd.Context(), types.CtxTerminalSpec, spec)
			cmd.SetContext(newCtx)

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errdefs.ErrLoggerNotFound
			}

			logger.Info("Starting sbsh terminal command")

			// Retrieve terminal spec from context
			terminalSpec, ok := cmd.Context().Value(types.CtxTerminalSpec).(*api.TerminalSpec)
			if !ok || terminalSpec == nil {
				return errdefs.ErrTerminalSpecNotFound
			}

			logger.Debug("Built terminal spec", "terminalSpec", fmt.Sprintf("%+v", terminalSpec))

			if logger.Enabled(context.Background(), slog.LevelDebug) {
				logger.DebugContext(cmd.Context(), "printing terminal spec for debug")
				if printErr := discovery.PrintTerminalSpec(terminalSpec, logger); printErr != nil {
					logger.Warn("Failed to print terminal spec", "error", printErr)
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			ctrl := terminal.NewTerminalController(ctx, logger)

			logger.Info("Starting terminal controller")
			runErr := runTerminal(ctx, cancel, logger, ctrl, terminalSpec)
			if runErr != nil {
				logger.Error("Terminal controller returned error", "error", runErr)
				return runErr
			}
			logger.Info("Terminal controller completed successfully")
			return nil
		},
		PostRunE: func(cmd *cobra.Command, _ []string) error {
			if c, _ := cmd.Context().Value(types.CtxCloser).(io.Closer); c != nil {
				_ = c.Close()
			}
			return nil
		},
	}

	setupTerminalCmdFlags(terminalCmd)
	return terminalCmd
}

func setupTerminalCmdFlags(terminalCmd *cobra.Command) {
	terminalCmd.Flags().String("id", "", "Optional terminal ID (random if omitted)")
	_ = viper.BindPFlag(config.SBSH_TERM_ID.ViperKey, terminalCmd.Flags().Lookup("id"))

	terminalCmd.Flags().String("command", "", "Optional command (default: /bin/bash)")
	_ = viper.BindPFlag(config.SBSH_TERM_COMMAND.ViperKey, terminalCmd.Flags().Lookup("command"))

	terminalCmd.Flags().String("name", "", "Optional name for the terminal (random if omitted)")
	_ = viper.BindPFlag(config.SBSH_TERM_NAME.ViperKey, terminalCmd.Flags().Lookup("name"))

	terminalCmd.Flags().String("terminal-log", "", "Optional filename for the terminal log")
	_ = viper.BindPFlag(config.SBSH_TERM_TERMINAL_LOG.ViperKey, terminalCmd.Flags().Lookup("terminal-log"))

	terminalCmd.Flags().String("log-file", "", "Optional filename for the terminal log")
	_ = viper.BindPFlag(config.SBSH_TERM_LOG_FILE.ViperKey, terminalCmd.Flags().Lookup("log-file"))

	terminalCmd.Flags().String("log-level", "", "Optional log level for the terminal logger")
	_ = viper.BindPFlag(config.SBSH_TERM_LOG_LEVEL.ViperKey, terminalCmd.Flags().Lookup("log-level"))

	terminalCmd.Flags().StringP("profile", "p", "", "Optional profile for the terminal")
	_ = viper.BindPFlag(config.SBSH_TERM_PROFILE.ViperKey, terminalCmd.Flags().Lookup("profile"))
	_ = viper.BindEnv(config.SBSH_TERM_PROFILE.ViperKey, config.SBSH_TERM_PROFILE.EnvVar())

	terminalCmd.Flags().String("socket", "", "Optional socket file for the terminal")
	_ = viper.BindPFlag(config.SBSH_TERM_SOCKET.ViperKey, terminalCmd.Flags().Lookup("socket"))

	terminalCmd.Flags().StringP("file", "f", "", "Optional JSON file with the terminal spec (use '-' for stdin)")
	_ = viper.BindPFlag(config.SBSH_TERM_SPEC.ViperKey, terminalCmd.Flags().Lookup("file"))
}

func runTerminal(
	ctx context.Context,
	cancel context.CancelFunc,
	logger *slog.Logger,
	ctrl api.TerminalController,
	spec *api.TerminalSpec,
) error {
	defer cancel()

	logger.DebugContext(
		ctx,
		"starting terminal controller goroutine",
		"terminal_name",
		spec.Name,
		"terminal_id",
		spec.ID,
	)
	errCh := make(chan error, 1)
	go func() {
		errCh <- ctrl.Run(spec)
		close(errCh)
		logger.DebugContext(ctx, "terminal controller goroutine exited")
	}()

	logger.DebugContext(ctx, "waiting for terminal controller to signal ready")
	if err := ctrl.WaitReady(); err != nil {
		logger.DebugContext(ctx, "terminal controller not ready", "error", err)
		return fmt.Errorf("%w: %w", errdefs.ErrWaitOnReady, err)
	}

	logger.DebugContext(ctx, "terminal controller ready, entering event loop")
	select {
	case <-ctx.Done():
		logger.DebugContext(ctx, "context canceled, waiting for terminal controller to exit")
		var waitErr error
		if errC := ctrl.WaitClose(); errC != nil {
			waitErr = fmt.Errorf("%w %w: %w", waitErr, errdefs.ErrWaitOnClose, errC)
			logger.DebugContext(ctx, "error waiting for terminal controller to close", "error", waitErr)
		}
		logger.DebugContext(ctx, "context canceled, terminal controller exited")
		return nil

	case err := <-errCh:
		logger.DebugContext(ctx, "terminal controller stopped", "error", err)
		if err != nil && !errors.Is(err, context.Canceled) {
			childErr := fmt.Errorf("%w: %w", errdefs.ErrChildExit, err)
			if errC := ctrl.WaitClose(); errC != nil {
				_ = fmt.Errorf("%w: %w: %w", childErr, errdefs.ErrWaitOnClose, errC)
			}
			logger.DebugContext(ctx, "terminal controller exited after error")
			// return nothing to avoid polluting the terminal with errors
			return nil
		}
	}
	return nil
}
