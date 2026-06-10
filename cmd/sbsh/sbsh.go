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

package sbsh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/sbsh/autocomplete"
	"github.com/eminwux/sbsh/cmd/sbsh/terminal"
	"github.com/eminwux/sbsh/cmd/sbsh/version"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/client"
	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/logging"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/internal/profile"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

//nolint:gocognit,funlen // complex because of logging and config setup
func NewSbshRootCmd() (*cobra.Command, error) {
	// rootCmd represents the base command when called without any subcommands.
	rootCmd := &cobra.Command{
		Use:   "sbsh",
		Short: "sbsh command line tool",
		Long: `sbsh is a command line tool to launch sbsh clients and terminals
You can see available options and commands with:
  sbsh help

If you run sbsh with no options, a default terminal will start.

You can also use sbsh with parameters. For example:
  sbsh --log-level=debug
  sbsh terminal
`,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		SilenceUsage: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			err := LoadConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Config error:", err)
				os.Exit(1)
			}

			return nil
		},
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			clientID := viper.GetString(config.SBSH_CLIENT_ID.ViperKey)
			if clientID == "" {
				clientID = naming.RandomID()
				viper.Set(config.SBSH_CLIENT_ID.ViperKey, clientID)
			}

			supLogLevel := viper.GetString(config.SBSH_CLIENT_LOG_LEVEL.ViperKey)
			if supLogLevel == "" {
				supLogLevel = "info"
			}

			supLogfile := viper.GetString(config.SBSH_CLIENT_LOG_FILE.ViperKey)
			if supLogfile == "" {
				supLogfile = filepath.Join(
					viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
					defaults.ClientsRunPath,
					clientID,
					"log",
				)
			}

			err := logging.SetupFileLogger(cmd, supLogfile, supLogLevel)
			if err != nil {
				return err
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
			}

			if viper.GetBool(config.SBSH_CLIENT_DETACH.ViperKey) {
				detachSelf()
				return nil
			}

			logger.DebugContext(
				cmd.Context(), "parameters received in sbsh",
				"runPath", viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
				"configFile", viper.GetString(config.SBSH_ROOT_CONFIG_FILE.ViperKey),
				"logLevel", viper.GetString(config.SBSH_ROOT_LOG_LEVEL.ViperKey),
				"profilesDir", viper.GetString(config.SBSH_ROOT_PROFILES_DIR.ViperKey),
				"terminalID", viper.GetString(config.SBSH_ROOT_TERM_ID.ViperKey),
				"terminalCommand", viper.GetString(config.SBSH_ROOT_TERM_COMMAND.ViperKey),
				"terminalName", viper.GetString(config.SBSH_ROOT_TERM_NAME.ViperKey),
				"terminalLogFilename", viper.GetString(config.SBSH_ROOT_TERM_LOG_FILE.ViperKey),
				"terminalProfile", viper.GetString(config.SBSH_ROOT_TERM_PROFILE.ViperKey),
				"terminalSocket", viper.GetString(config.SBSH_ROOT_TERM_SOCKET.ViperKey),
				"clientSocket", viper.GetString(config.SBSH_CLIENT_SOCKET.ViperKey),
				"detach", viper.GetBool(config.SBSH_CLIENT_DETACH.ViperKey),
			)

			// Set in PreRunE - shouldn't be null here
			clientID := viper.GetString(config.SBSH_CLIENT_ID.ViperKey)
			supLogfile := viper.GetString(config.SBSH_CLIENT_LOG_FILE.ViperKey)

			clientName := viper.GetString(config.SBSH_CLIENT_NAME.ViperKey)
			if clientName == "" {
				clientName = naming.RandomName()
				viper.Set(config.SBSH_CLIENT_NAME.ViperKey, clientName)
			}

			socketFileInput := viper.GetString(config.SBSH_CLIENT_SOCKET.ViperKey)
			if socketFileInput == "" {
				socketFileInput = filepath.Join(
					viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
					defaults.ClientsRunPath,
					clientID,
					"socket",
				)
			}

			terminalSpec, buildErr := profile.BuildTerminalSpecFromProfile(
				cmd.Context(),
				logger,
				&profile.BuildTerminalSpecParams{
					TerminalID:       viper.GetString(config.SBSH_ROOT_TERM_ID.ViperKey),
					TerminalName:     viper.GetString(config.SBSH_ROOT_TERM_NAME.ViperKey),
					TerminalCmd:      viper.GetString(config.SBSH_ROOT_TERM_COMMAND.ViperKey),
					CaptureFile:      viper.GetString(config.SBSH_ROOT_TERM_CAPTURE_FILE.ViperKey),
					RunPath:          viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
					ProfilesDir:      viper.GetString(config.SBSH_ROOT_PROFILES_DIR.ViperKey),
					ProfileName:      viper.GetString(config.SBSH_ROOT_TERM_PROFILE.ViperKey),
					LogFile:          viper.GetString(config.SBSH_ROOT_TERM_LOG_FILE.ViperKey),
					LogLevel:         viper.GetString(config.SBSH_ROOT_TERM_LOG_LEVEL.ViperKey),
					SocketFile:       viper.GetString(config.SBSH_ROOT_TERM_SOCKET.ViperKey),
					SocketMode:       viper.GetString(config.SBSH_ROOT_TERM_SOCKET_MODE.ViperKey),
					SocketGID:        readRootSocketGID(cmd),
					CaptureMode:      viper.GetString(config.SBSH_ROOT_TERM_CAPTURE_MODE.ViperKey),
					CaptureGID:       readRootCaptureGID(cmd),
					CaptureFormat:    viper.GetString(config.SBSH_ROOT_TERM_CAPTURE_FORMAT.ViperKey),
					LogFileMode:      viper.GetString(config.SBSH_ROOT_TERM_LOG_FILE_MODE.ViperKey),
					LogFileGID:       readRootLogFileGID(cmd),
					DisableSetPrompt: viper.GetBool(config.SBSH_ROOT_TERM_DISABLE_SET_PROMPT.ViperKey),
				},
			)

			if buildErr != nil {
				logger.Error("Failed to build terminal spec", "error", buildErr)
				return buildErr
			}

			if errAvail := discovery.VerifyTerminalNameAvailable(
				cmd.Context(),
				logger,
				terminalSpec.RunPath,
				terminalSpec.Name,
			); errAvail != nil {
				logger.Error("Terminal name collision", "error", errAvail)
				return errAvail
			}

			logger.Debug("Built terminal spec", "terminalSpec", fmt.Sprintf("%+v", terminalSpec))

			// Define a new ClientDoc
			disableDetach := viper.GetBool(config.SBSH_CLIENT_DISABLE_DETACH_KEYSTROKE.ViperKey)
			doc := &api.ClientDoc{
				APIVersion: api.APIVersionV1Beta1,
				Kind:       api.KindClient,
				Metadata: api.ClientMetadata{
					Name:        clientName,
					Labels:      make(map[string]string),
					Annotations: make(map[string]string),
				},
				Spec: api.ClientSpec{
					ID:              api.ID(clientID),
					RunPath:         viper.GetString(config.SBSH_ROOT_RUN_PATH.ViperKey),
					LogFile:         supLogfile,
					SockerCtrl:      socketFileInput,
					TerminalSpec:    terminalSpec,
					DetachKeystroke: !disableDetach, // Invert flag: when disable-detach is true, DetachKeystroke is false
					ClientMode:      api.RunNewTerminal,
				},
			}

			logger.DebugContext(cmd.Context(), "ClientDoc values",
				"Kind", doc.Kind,
				"ID", doc.Spec.ID,
				"Name", doc.Metadata.Name,
				"RunPath", doc.Spec.RunPath,
				"SockerCtrl", doc.Spec.SockerCtrl,
				"TerminalSpec", doc.Spec.TerminalSpec,
			)

			if logger.Enabled(cmd.Context(), slog.LevelDebug) {
				if printErr := discovery.PrintTerminalSpec(terminalSpec, logger); printErr != nil {
					logger.Warn("Failed to print terminal spec", "error", printErr)
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			ctrl := client.NewClientController(ctx, logger)

			return runClient(ctx, cancel, logger, ctrl, doc)
		},
		PostRunE: func(cmd *cobra.Command, _ []string) error {
			if c, _ := cmd.Context().Value(types.CtxCloser).(io.Closer); c != nil {
				_ = c.Close()
			}
			return nil
		},
	}

	err := setupRootCmd(rootCmd)
	if err != nil {
		return nil, err
	}

	return rootCmd, nil
}

func setPersistentLoggingFlags(rootCmd *cobra.Command) error {
	rootCmd.PersistentFlags().String("run-path", "", "Optional run path for the client")
	if err := viper.BindPFlag(config.SBSH_ROOT_RUN_PATH.ViperKey, rootCmd.PersistentFlags().Lookup("run-path")); err != nil {
		return err
	}

	rootCmd.PersistentFlags().String("config", "", "config file (default is $HOME/.sbsh/config.yaml)")
	if err := viper.BindPFlag(config.SBSH_ROOT_CONFIG_FILE.ViperKey, rootCmd.PersistentFlags().Lookup("config")); err != nil {
		return err
	}

	rootCmd.PersistentFlags().String(
		"profiles-dir", "",
		"Directory scanned for TerminalProfile YAML documents (default: $HOME/.sbsh/profiles.d/)",
	)
	if err := viper.BindPFlag(
		config.SBSH_ROOT_PROFILES_DIR.ViperKey,
		rootCmd.PersistentFlags().Lookup("profiles-dir"),
	); err != nil {
		return err
	}

	return nil
}

func setClientFlags(rootCmd *cobra.Command) error {
	rootCmd.Flags().String("id", "", "Optional ID for the client")
	if err := viper.BindPFlag(config.SBSH_CLIENT_ID.ViperKey, rootCmd.Flags().Lookup("id")); err != nil {
		return err
	}

	rootCmd.Flags().String("socket", "", "Optional socket file for the client")
	if err := viper.BindPFlag(config.SBSH_CLIENT_SOCKET.ViperKey, rootCmd.Flags().Lookup("socket")); err != nil {
		return err
	}

	rootCmd.Flags().String("log-file", "", "Optional socket file for the client")
	if err := viper.BindPFlag(config.SBSH_CLIENT_LOG_FILE.ViperKey, rootCmd.Flags().Lookup("log-file")); err != nil {
		return err
	}

	rootCmd.Flags().String("log-level", "", "Optional log level for the client")
	if err := viper.BindPFlag(config.SBSH_CLIENT_LOG_LEVEL.ViperKey, rootCmd.Flags().Lookup("log-level")); err != nil {
		return err
	}

	rootCmd.Flags().BoolP("detach", "d", false, "Optional detach flag for the client")
	if err := viper.BindPFlag(config.SBSH_CLIENT_DETACH.ViperKey, rootCmd.Flags().Lookup("detach")); err != nil {
		return err
	}

	rootCmd.Flags().Bool("disable-detach", false, "Disable detach keystroke (^] twice)")
	if err := viper.BindPFlag(config.SBSH_CLIENT_DISABLE_DETACH_KEYSTROKE.ViperKey, rootCmd.Flags().Lookup("disable-detach")); err != nil {
		return err
	}

	return nil
}

func setTerminalFlags(rootCmd *cobra.Command) error {
	rootCmd.Flags().String("terminal-id", "", "Optional terminal ID (random if omitted)")
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_ID.ViperKey, rootCmd.Flags().Lookup("terminal-id")); err != nil {
		return err
	}

	rootCmd.Flags().String("terminal-command", "", "Optional command (default: /bin/bash)")
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_COMMAND.ViperKey, rootCmd.Flags().Lookup("terminal-command")); err != nil {
		return err
	}

	rootCmd.Flags().String("terminal-name", "", "Optional name for the terminal (must be unique across active terminals)")
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_NAME.ViperKey, rootCmd.Flags().Lookup("terminal-name")); err != nil {
		return err
	}

	rootCmd.Flags().String("terminal-capture-file", "", "Optional filename for the PTY transcript capture")
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_CAPTURE_FILE.ViperKey, rootCmd.Flags().Lookup("terminal-capture-file")); err != nil {
		return err
	}

	rootCmd.Flags().String("terminal-log-file", "", "Optional filename for the terminal log")
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_LOG_FILE.ViperKey, rootCmd.Flags().Lookup("terminal-log-file")); err != nil {
		return err
	}

	rootCmd.Flags().String("terminal-log-level", "", "Optional log level for the terminal")
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_LOG_LEVEL.ViperKey, rootCmd.Flags().Lookup("terminal-log-level")); err != nil {
		return err
	}

	rootCmd.Flags().StringP("terminal-profile", "p", "", "Optional profile for the terminal")
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_PROFILE.ViperKey, rootCmd.Flags().Lookup("terminal-profile")); err != nil {
		return err
	}
	if err := viper.BindEnv(config.SBSH_ROOT_TERM_PROFILE.ViperKey, config.SBSH_ROOT_TERM_PROFILE.EnvVar()); err != nil {
		return err
	}
	if err := setAutoCompleteProfile(rootCmd); err != nil {
		return err
	}

	rootCmd.Flags().String("terminal-socket", "", "Optional socket file for the terminal")
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_SOCKET.ViperKey, rootCmd.Flags().Lookup("terminal-socket")); err != nil {
		return err
	}

	rootCmd.Flags().String(
		"terminal-socket-mode",
		"",
		"Octal mode applied to the control socket after Listen (default 0600)",
	)
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_SOCKET_MODE.ViperKey, rootCmd.Flags().Lookup("terminal-socket-mode")); err != nil {
		return err
	}
	if err := viper.BindEnv(config.SBSH_ROOT_TERM_SOCKET_MODE.ViperKey, config.SBSH_ROOT_TERM_SOCKET_MODE.EnvVar()); err != nil {
		return err
	}

	rootCmd.Flags().Int(
		"terminal-socket-gid",
		-1,
		"Numeric GID applied to the control socket via chown after Listen; -1 leaves group unchanged",
	)
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_SOCKET_GID.ViperKey, rootCmd.Flags().Lookup("terminal-socket-gid")); err != nil {
		return err
	}
	if err := viper.BindEnv(config.SBSH_ROOT_TERM_SOCKET_GID.ViperKey, config.SBSH_ROOT_TERM_SOCKET_GID.EnvVar()); err != nil {
		return err
	}

	rootCmd.Flags().String(
		"terminal-capture-mode",
		"",
		"Octal mode applied to the capture file after Open (default 0600)",
	)
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_CAPTURE_MODE.ViperKey, rootCmd.Flags().Lookup("terminal-capture-mode")); err != nil {
		return err
	}
	if err := viper.BindEnv(config.SBSH_ROOT_TERM_CAPTURE_MODE.ViperKey, config.SBSH_ROOT_TERM_CAPTURE_MODE.EnvVar()); err != nil {
		return err
	}

	rootCmd.Flags().Int(
		"terminal-capture-gid",
		-1,
		"Numeric GID applied to the capture file via chown after Open; -1 leaves group unchanged",
	)
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_CAPTURE_GID.ViperKey, rootCmd.Flags().Lookup("terminal-capture-gid")); err != nil {
		return err
	}
	if err := viper.BindEnv(config.SBSH_ROOT_TERM_CAPTURE_GID.ViperKey, config.SBSH_ROOT_TERM_CAPTURE_GID.EnvVar()); err != nil {
		return err
	}

	rootCmd.Flags().String(
		"terminal-capture-format",
		"",
		`On-disk capture format: "raw" (default, byte-exact PTY bytes) or "asciicast" (asciicast v2 for replay/seek/portability)`,
	)
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_CAPTURE_FORMAT.ViperKey, rootCmd.Flags().Lookup("terminal-capture-format")); err != nil {
		return err
	}
	if err := viper.BindEnv(config.SBSH_ROOT_TERM_CAPTURE_FORMAT.ViperKey, config.SBSH_ROOT_TERM_CAPTURE_FORMAT.EnvVar()); err != nil {
		return err
	}

	rootCmd.Flags().String(
		"terminal-log-file-mode",
		"",
		"Octal mode applied to the log file after Open (default 0600)",
	)
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_LOG_FILE_MODE.ViperKey, rootCmd.Flags().Lookup("terminal-log-file-mode")); err != nil {
		return err
	}
	if err := viper.BindEnv(config.SBSH_ROOT_TERM_LOG_FILE_MODE.ViperKey, config.SBSH_ROOT_TERM_LOG_FILE_MODE.EnvVar()); err != nil {
		return err
	}

	rootCmd.Flags().Int(
		"terminal-log-file-gid",
		-1,
		"Numeric GID applied to the log file via chown after Open; -1 leaves group unchanged",
	)
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_LOG_FILE_GID.ViperKey, rootCmd.Flags().Lookup("terminal-log-file-gid")); err != nil {
		return err
	}
	if err := viper.BindEnv(config.SBSH_ROOT_TERM_LOG_FILE_GID.ViperKey, config.SBSH_ROOT_TERM_LOG_FILE_GID.EnvVar()); err != nil {
		return err
	}

	rootCmd.Flags().Bool("terminal-disable-set-prompt", false, "Disable setting the prompt")
	if err := viper.BindPFlag(config.SBSH_ROOT_TERM_DISABLE_SET_PROMPT.ViperKey, rootCmd.Flags().Lookup("terminal-disable-set-prompt")); err != nil {
		return err
	}

	return nil
}

func setupRootCmd(rootCmd *cobra.Command) error {
	// go http.ListenAndServe("127.0.0.1:6060", nil)
	// runtime.SetBlockProfileRate(1)     // sample ALL blocking events on chans/locks
	// runtime.SetMutexProfileFraction(1) // sample ALL mutex contention
	rootCmd.AddCommand(version.NewVersionCmd())
	rootCmd.AddCommand(terminal.NewTerminalCmd())
	rootCmd.AddCommand(autocomplete.NewAutoCompleteCmd(rootCmd))

	// Persistent flags
	if err := setPersistentLoggingFlags(rootCmd); err != nil {
		return err
	}

	// Bind Non-persistent Flags to Viper
	// Client flags
	if err := setClientFlags(rootCmd); err != nil {
		return err
	}

	// Terminals flags
	if err := setTerminalFlags(rootCmd); err != nil {
		return err
	}

	return nil
}

func runClient(
	ctx context.Context,
	cancel context.CancelFunc,
	logger *slog.Logger,
	ctrl api.ClientController,
	doc *api.ClientDoc,
) error {
	defer cancel()

	errCh := make(chan error, 1)

	logger.DebugContext(
		ctx,
		"starting client controller goroutine",
		"client_mode",
		doc.Spec.ClientMode,
		"run_path",
		doc.Spec.RunPath,
	)
	go func() {
		errCh <- ctrl.Run(doc)
		close(errCh)
		logger.DebugContext(ctx, "controller goroutine exited")
	}()

	logger.DebugContext(ctx, "waiting for controller to signal ready")
	if err := ctrl.WaitReady(); err != nil {
		// A signal that cancels ctx during the attach window makes WaitReady
		// return ctx.Err() before readiness. By this point the controller's
		// Attach has already put the parent terminal into raw mode (the IO
		// copiers are relaying the prompt), so returning straight away strands
		// the parent shell in raw mode — the failure #392 fixes for the
		// steady-state path, surfacing here on an earlier window. Drive the
		// same WaitClose/isCleanSessionEnd teardown the ctx.Done arm below
		// uses: the controller's Run goroutine runs Close once Attach unblocks
		// on the cancelled ctx, and Close restores the parent terminal before
		// closing the channel WaitClose blocks on. A handled signal
		// (context.Canceled) then exits 0 like the steady-state arm; a genuine
		// readiness failure still surfaces wrapped in ErrWaitOnReady. See #419.
		logger.DebugContext(ctx, "controller not ready, draining teardown", "error", err)
		if errC := ctrl.WaitClose(); errC != nil {
			err = fmt.Errorf("%w: %w", errdefs.ErrWaitOnClose, errC)
		}
		if !isCleanSessionEnd(err) {
			return fmt.Errorf("%w: %w", errdefs.ErrWaitOnReady, err)
		}
		return nil
	}

	logger.DebugContext(ctx, "controller ready, entering client event loop")
	select {
	case <-ctx.Done():
		logger.DebugContext(ctx, "context canceled, waiting for controller to exit")
		// ctx is signal.NotifyContext(...), so ctx.Err() is context.Canceled
		// on a SIGTERM/SIGINT. Route it through the same isCleanSessionEnd
		// filter the errCh arm uses so a handled signal exits 0 with no
		// spurious `Error: context has been cancelled` line — matching the
		// clean-session-end contract from #380/#381. A real WaitClose failure
		// during teardown is not clean and still surfaces.
		err := ctx.Err()
		if errC := ctrl.WaitClose(); errC != nil {
			err = fmt.Errorf("%w: %w", errdefs.ErrWaitOnClose, errC)
		}
		logger.DebugContext(ctx, "context canceled, controller exited")
		if err != nil && !isCleanSessionEnd(err) {
			return fmt.Errorf("%w: %w", errdefs.ErrContextDone, err)
		}
		return nil

	case err := <-errCh:
		logger.DebugContext(ctx, "controller stopped", "error", err)
		if err != nil && !isCleanSessionEnd(err) {
			err = fmt.Errorf("%w: %w", errdefs.ErrChildExit, err)
			if errC := ctrl.WaitClose(); errC != nil {
				err = fmt.Errorf("%w: %w: %w", err, errdefs.ErrWaitOnClose, errC)
			}
			logger.ErrorContext(ctx, "client controller exited with error", "error", err)
			return err
		}
	}
	return nil
}

// isCleanSessionEnd reports whether err signals an end-of-session that
// runClient must swallow rather than surface to cobra as `Error: …`:
// context cancellation, an operator-initiated detach (errdefs.ErrClientDetached
// — chained on the close cause by Controller.classifySessionEnd when
// detachRequested is set), or the remote workload closing the IO
// connection on a clean exit (errdefs.ErrPeerClosed). Mirrors pkg/attach.Run's
// classifySessionEnd remap and cmd/sb/attach's nil-return on those
// public sentinels; sbsh consumes the controller directly so the swallow
// has to live here. See issue #380.
func isCleanSessionEnd(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, errdefs.ErrClientDetached) ||
		errors.Is(err, errdefs.ErrPeerClosed)
}

// readRootSocketGID resolves --terminal-socket-gid: explicit flag wins,
// then SBSH_ROOT_TERM_SOCKET_GID env, otherwise nil (= leave group
// unchanged). The pointer return distinguishes "unset" from "explicit
// 0 = root", which is required because the runner would otherwise try
// to chown to gid 0 and fail under non-root callers.
func readRootSocketGID(cmd *cobra.Command) *int {
	return readRootGIDFlag(cmd, "terminal-socket-gid", config.SBSH_ROOT_TERM_SOCKET_GID.EnvVar())
}

// readRootCaptureGID mirrors readRootSocketGID for --terminal-capture-gid.
func readRootCaptureGID(cmd *cobra.Command) *int {
	return readRootGIDFlag(cmd, "terminal-capture-gid", config.SBSH_ROOT_TERM_CAPTURE_GID.EnvVar())
}

// readRootLogFileGID mirrors readRootSocketGID for --terminal-log-file-gid.
func readRootLogFileGID(cmd *cobra.Command) *int {
	return readRootGIDFlag(cmd, "terminal-log-file-gid", config.SBSH_ROOT_TERM_LOG_FILE_GID.EnvVar())
}

func readRootGIDFlag(cmd *cobra.Command, flagName, envName string) *int {
	if cmd.Flags().Changed(flagName) {
		if v, err := cmd.Flags().GetInt(flagName); err == nil {
			return &v
		}
	}
	if envStr, ok := os.LookupEnv(envName); ok && envStr != "" {
		if v, err := strconv.Atoi(envStr); err == nil {
			return &v
		}
	}
	return nil
}

// LoadConfig loads config.yaml from the given path or HOME/.sbsh.
func LoadConfig() error {
	configFile := viper.GetString(config.SBSH_ROOT_CONFIG_FILE.ViperKey)
	if configFile == "" {
		configFile = config.DefaultConfigFile()
	}
	_ = config.SBSH_ROOT_CONFIG_FILE.BindEnv()
	_ = config.SBSH_ROOT_CONFIG_FILE.Set(configFile)

	cfgDoc, err := config.LoadConfigurationDoc(configFile)
	if err != nil {
		return err
	}
	// Promote ConfigurationDoc values to env vars so subcommand helpers that
	// read os.Getenv directly also see them. User-set env vars are untouched.
	config.ApplyConfigurationDocEnv(cfgDoc)

	_ = config.SBSH_ROOT_RUN_PATH.BindEnv()
	runPath := config.DefaultRunPath()
	if cfgDoc != nil && cfgDoc.Spec.RunPath != "" {
		runPath = cfgDoc.Spec.RunPath
	}
	config.SBSH_ROOT_RUN_PATH.SetDefault(runPath)

	_ = config.SBSH_ROOT_PROFILES_DIR.BindEnv()
	profilesDir := config.DefaultProfilesDir()
	if cfgDoc != nil && cfgDoc.Spec.ProfilesDir != "" {
		profilesDir = cfgDoc.Spec.ProfilesDir
	}
	config.SBSH_ROOT_PROFILES_DIR.SetDefault(profilesDir)

	_ = config.SBSH_ROOT_LOG_LEVEL.BindEnv()
	logLevel := "info"
	if cfgDoc != nil && cfgDoc.Spec.LogLevel != "" {
		logLevel = cfgDoc.Spec.LogLevel
	}
	config.SBSH_ROOT_LOG_LEVEL.SetDefault(logLevel)

	return nil
}

func detachSelf() {
	// Evita bucle si ya estamos desprendidos
	if os.Getenv("SB_DETACHED") == "1" {
		return
	}

	// Rebuild args without --detach or -d so the child does not detach again
	rawArgs := os.Args[1:]
	args := make([]string, 0, len(rawArgs))
	for _, a := range rawArgs {
		if a == "--detach" || a == "-d" {
			continue
		}
		args = append(args, a)
	}

	//nolint:gosec,noctx // We want to re-execute same program with same args; we explicitly don't want to use context here to avoid killing the process on parent exit
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "SB_DETACHED=1")

	// Desacoplar stdio (o redirígilos a archivo si querés logs)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Crear nueva sesión → sin TTY controlador
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if err := cmd.Start(); err != nil {
		// si falla, preferible loguear y continuar en foreground
		// o salir con error según tu caso
		return
	}
	fmt.Printf("[%d] sbsh detached\n", cmd.Process.Pid)
	os.Exit(0) // el padre termina; el hijo queda en background
}

func setAutoCompleteProfile(rootCmd *cobra.Command) error {
	return rootCmd.RegisterFlagCompletionFunc(
		"terminal-profile",
		func(c *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			//nolint:mnd // 150ms is a good compromise between snappy completion and enough time to read files
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			profilesDir, err := config.GetProfilesDirFromEnvAndFlags(c, config.SBSH_ROOT_PROFILES_DIR.EnvVar())
			if err != nil {
				// fail silent to keep completion snappy
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			profs, err := config.AutoCompleteListProfileNames(ctx, nil, profilesDir)
			if err != nil {
				// fail silent to keep completion snappy
				return nil, cobra.ShellCompDirectiveNoFileComp
			}

			// Optionally add descriptions: "value\tpath" for nicer columns
			return profs, cobra.ShellCompDirectiveNoFileComp
		},
	)
}
