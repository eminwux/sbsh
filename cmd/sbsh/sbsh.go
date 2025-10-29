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
	"syscall"
	"time"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/sbsh/autocomplete"
	"github.com/eminwux/sbsh/cmd/sbsh/run"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/logging"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/internal/profile"
	"github.com/eminwux/sbsh/internal/supervisor"
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
		Long: `sbsh is a command line tool to manage sbsh sessions and profiles.

You can see available options and commands with:
  sbsh help

If you run sbsh with no options, a default session will start.

You can also use sbsh with parameters. For example:
  sbsh --log-level=debug
  sbsh run
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
			supervisorID := viper.GetString("sbsh.supervisor.id")
			if supervisorID == "" {
				supervisorID = naming.RandomID()
				viper.Set("sbsh.supervisor.id", supervisorID)
			}

			supLogLevel := viper.GetString("sbsh.supervisor.logLevel")
			if supLogLevel == "" {
				supLogLevel = "info"
			}

			sessionLogLevel := viper.GetString("sbsh.session.logLevel")
			if sessionLogLevel == "" {
				sessionLogLevel = supLogLevel
				viper.Set("sbsh.session.logLevel", sessionLogLevel)
			}

			supLogfile := viper.GetString("sbsh.supervisor.logFile")
			if supLogfile == "" {
				supLogfile = filepath.Join(
					viper.GetString(config.SBSH_RUN_PATH.ViperKey),
					"supervisors",
					supervisorID,
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

			if viper.GetBool("sbsh.supervisor.detach") {
				detachSelf()
				return nil
			}

			logger.DebugContext(
				cmd.Context(), "parameters received in sbsh",
				"runPath", viper.GetString(config.SBSH_RUN_PATH.ViperKey),
				"configFile", viper.GetString(config.CONFIG_FILE.ViperKey),
				"logLevel", viper.GetString(config.LOG_LEVEL.ViperKey),
				"profilesFile", viper.GetString(config.PROFILES_FILE.ViperKey),
				"sessionID", viper.GetString("sbsh.session.id"),
				"sessionCommand", viper.GetString("sbsh.session.command"),
				"sessionName", viper.GetString("sbsh.session.name"),
				"sessionLogFilename", viper.GetString("sbsh.session.logFile"),
				"sessionProfile", viper.GetString("sbsh.session.profile"),
				"sessionSocket", viper.GetString("sbsh.session.socket"),
				"supervisorSocket", viper.GetString("sbsh.supervisor.socket"),
				"detach", viper.GetBool("sbsh.supervisor.detach"),
			)

			// Set in PreRunE - shouldn't be null here
			supervisorID := viper.GetString("sbsh.supervisor.id")
			supLogfile := viper.GetString("sbsh.supervisor.logFile")

			supervisorName := viper.GetString("sbsh.supervisor.name")
			if supervisorName == "" {
				supervisorName = naming.RandomName()
				viper.Set("sbsh.supervisor.name", supervisorName)
			}

			socketFileInput := viper.GetString("sbsh.supervisor.socket")
			if socketFileInput == "" {
				socketFileInput = filepath.Join(
					viper.GetString(config.SBSH_RUN_PATH.ViperKey),
					"supervisors",
					supervisorID,
					"socket",
				)
			}

			sessionSpec, buildErr := profile.BuildSessionSpec(
				cmd.Context(),
				logger,
				&profile.BuildSessionSpecParams{
					SessionID:    viper.GetString("sbsh.session.id"),
					SessionName:  viper.GetString("sbsh.session.name"),
					SessionCmd:   viper.GetString("sbsh.session.command"),
					CaptureFile:  viper.GetString("sbsh.session.captureFile"),
					RunPath:      viper.GetString(config.SBSH_RUN_PATH.ViperKey),
					ProfilesFile: viper.GetString(config.PROFILES_FILE.ViperKey),
					ProfileName:  viper.GetString("sbsh.session.profile"),
					LogFile:      viper.GetString("sbsh.session.logFile"),
					LogLevel:     viper.GetString("sbsh.session.logLevel"),
					SocketFile:   viper.GetString("sbsh.session.socket"),
				},
			)

			if buildErr != nil {
				logger.Error("Failed to build session spec", "error", buildErr)
				return buildErr
			}

			logger.Debug("Built session spec", "sessionSpec", fmt.Sprintf("%+v", sessionSpec))

			// Define a new SupervisorSpec
			spec := &api.SupervisorSpec{
				Kind:        api.RunNewSession,
				ID:          api.ID(supervisorID),
				Name:        supervisorName,
				RunPath:     viper.GetString(config.SBSH_RUN_PATH.ViperKey),
				LogFile:     supLogfile,
				SockerCtrl:  socketFileInput,
				SessionSpec: sessionSpec,
			}

			logger.DebugContext(cmd.Context(), "SupervisorSpec values",
				"Kind", spec.Kind,
				"ID", spec.ID,
				"Name", spec.Name,
				"RunPath", spec.RunPath,
				"SockerCtrl", spec.SockerCtrl,
				"SessionSpec", spec.SessionSpec,
			)

			if logger.Enabled(cmd.Context(), slog.LevelDebug) {
				if printErr := discovery.PrintTerminalSpec(sessionSpec, logger); printErr != nil {
					logger.Warn("Failed to print session spec", "error", printErr)
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			ctrl := supervisor.NewSupervisorController(ctx, logger)

			return runSupervisor(ctx, cancel, logger, ctrl, spec)
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
	rootCmd.PersistentFlags().String("run-path", "", "Optional run path for the supervisor")
	if err := viper.BindPFlag(config.SBSH_RUN_PATH.ViperKey, rootCmd.PersistentFlags().Lookup("run-path")); err != nil {
		return err
	}

	rootCmd.PersistentFlags().String("config", "", "config file (default is $HOME/.sbsh/config.yaml)")
	if err := viper.BindPFlag(config.CONFIG_FILE.ViperKey, rootCmd.PersistentFlags().Lookup("config")); err != nil {
		return err
	}

	rootCmd.PersistentFlags().String("profiles", "", "profiles manifests file")
	if err := viper.BindPFlag(config.PROFILES_FILE.ViperKey, rootCmd.PersistentFlags().Lookup("profiles")); err != nil {
		return err
	}

	return nil
}

func setSupervisorFlags(rootCmd *cobra.Command) error {
	rootCmd.Flags().String("id", "", "Optional ID for the supervisor")
	if err := viper.BindPFlag("sbsh.supervisor.id", rootCmd.Flags().Lookup("id")); err != nil {
		return err
	}

	rootCmd.Flags().String("socket", "", "Optional socket file for the session")
	if err := viper.BindPFlag("sbsh.supervisor.socket", rootCmd.Flags().Lookup("socket")); err != nil {
		return err
	}

	rootCmd.Flags().String("log-file", "", "Optional socket file for the supervisor")
	if err := viper.BindPFlag("sbsh.supervisor.logFile", rootCmd.Flags().Lookup("log-file")); err != nil {
		return err
	}

	rootCmd.Flags().String("log-level", "", "Optional log level for the supervisor")
	if err := viper.BindPFlag("sbsh.supervisor.logLevel", rootCmd.Flags().Lookup("log-level")); err != nil {
		return err
	}

	rootCmd.Flags().BoolP("detach", "d", false, "Optional socket file for the session")
	if err := viper.BindPFlag("sbsh.supervisor.detach", rootCmd.Flags().Lookup("detach")); err != nil {
		return err
	}

	return nil
}

func setSessionFlags(rootCmd *cobra.Command) error {
	rootCmd.Flags().String("session-id", "", "Optional session ID (random if omitted)")
	if err := viper.BindPFlag("sbsh.session.id", rootCmd.Flags().Lookup("session-id")); err != nil {
		return err
	}

	rootCmd.Flags().String("session-command", "", "Optional command (default: /bin/bash)")
	if err := viper.BindPFlag("sbsh.session.command", rootCmd.Flags().Lookup("session-command")); err != nil {
		return err
	}

	rootCmd.Flags().String("session-name", "", "Optional name for the session")
	if err := viper.BindPFlag("sbsh.session.name", rootCmd.Flags().Lookup("session-name")); err != nil {
		return err
	}

	rootCmd.Flags().String("capture-file", "", "Optional filename for the session log")
	if err := viper.BindPFlag("sbsh.session.captureFile", rootCmd.Flags().Lookup("capture-file")); err != nil {
		return err
	}

	rootCmd.Flags().String("session-logfile", "", "Optional filename for the session log")
	if err := viper.BindPFlag("sbsh.session.logFile", rootCmd.Flags().Lookup("session-logfile")); err != nil {
		return err
	}

	rootCmd.Flags().String("session-loglevel", "", "Optional log level for the session")
	if err := viper.BindPFlag("sbsh.session.logLevel", rootCmd.Flags().Lookup("session-loglevel")); err != nil {
		return err
	}

	rootCmd.Flags().StringP("profile", "p", "", "Optional profile for the session")
	if err := viper.BindPFlag("sbsh.session.profile", rootCmd.Flags().Lookup("profile")); err != nil {
		return err
	}
	if err := viper.BindEnv("sbsh.session.profile", config.SES_PROFILE.EnvVar()); err != nil {
		return err
	}
	if err := setAutoCompleteProfile(rootCmd); err != nil {
		return err
	}

	rootCmd.Flags().String("session-socket", "", "Optional socket file for the session")
	if err := viper.BindPFlag("sbsh.session.socket", rootCmd.Flags().Lookup("session-socket")); err != nil {
		return err
	}

	return nil
}

func setupRootCmd(rootCmd *cobra.Command) error {
	// go http.ListenAndServe("127.0.0.1:6060", nil)
	// runtime.SetBlockProfileRate(1)     // sample ALL blocking events on chans/locks
	// runtime.SetMutexProfileFraction(1) // sample ALL mutex contention

	rootCmd.AddCommand(run.NewRunCmd())
	rootCmd.AddCommand(autocomplete.NewAutoCompleteCmd(rootCmd))

	// Persistent flags
	if err := setPersistentLoggingFlags(rootCmd); err != nil {
		return err
	}

	// Bind Non-persistent Flags to Viper
	// Supervisor flags
	if err := setSupervisorFlags(rootCmd); err != nil {
		return err
	}

	// Session flags
	if err := setSessionFlags(rootCmd); err != nil {
		return err
	}

	return nil
}

func runSupervisor(
	ctx context.Context,
	cancel context.CancelFunc,
	logger *slog.Logger,
	ctrl api.SupervisorController,
	spec *api.SupervisorSpec,
) error {
	defer cancel()

	errCh := make(chan error, 1)

	logger.DebugContext(
		ctx,
		"starting supervisor controller goroutine",
		"spec_kind",
		spec.Kind,
		"run_path",
		spec.RunPath,
	)
	go func() {
		errCh <- ctrl.Run(spec)
		close(errCh)
		logger.DebugContext(ctx, "controller goroutine exited")
	}()

	logger.DebugContext(ctx, "waiting for controller to signal ready")
	if err := ctrl.WaitReady(); err != nil {
		logger.DebugContext(ctx, "controller not ready", "error", err)
		return fmt.Errorf("%w: %w", errdefs.ErrWaitOnReady, err)
	}

	logger.DebugContext(ctx, "controller ready, entering supervisor event loop")
	select {
	case <-ctx.Done():
		var err error
		logger.DebugContext(ctx, "context canceled, waiting for controller to exit")
		if errC := ctrl.WaitClose(); errC != nil {
			err = fmt.Errorf("%w: %w: %w", err, errdefs.ErrWaitOnClose, errC)
		}
		logger.DebugContext(ctx, "context canceled, controller exited")
		return fmt.Errorf("%w: %w", errdefs.ErrContextDone, err)

	case err := <-errCh:
		logger.DebugContext(ctx, "controller stopped", "error", err)
		if err != nil && !errors.Is(err, context.Canceled) {
			err = fmt.Errorf("%w: %w", errdefs.ErrChildExit, err)
			if errC := ctrl.WaitClose(); err != nil {
				err = fmt.Errorf("%w: %w: %w", err, errdefs.ErrWaitOnClose, errC)
			}
			logger.ErrorContext(ctx, "supervisor controller exited with error", "error", err)
			logger.DebugContext(ctx, "controller exited after error")

			// return nothing to avoid polluting the terminal with errors
			return nil
		}
	}
	return nil
}

// LoadConfig loads config.yaml from the given path or HOME/.sbsh.
func LoadConfig() error {
	var configFile string
	if viper.GetString(config.CONFIG_FILE.ViperKey) == "" {
		configFile = config.DefaultConfigFile()
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		// Add the directory containing the config file
		viper.AddConfigPath(filepath.Dir(configFile))
	}
	_ = config.CONFIG_FILE.BindEnv()
	_ = config.CONFIG_FILE.Set(configFile)

	var runPath string
	if viper.GetString(config.SBSH_RUN_PATH.ViperKey) == "" {
		runPath = config.DefaultRunPath()
	}
	_ = config.SBSH_RUN_PATH.BindEnv()
	config.SBSH_RUN_PATH.SetDefault(runPath)

	var profilesFile string
	if viper.GetString(config.PROFILES_FILE.ViperKey) == "" {
		profilesFile = config.DefaultProfilesFile()
	}
	_ = config.PROFILES_FILE.BindEnv()
	config.PROFILES_FILE.SetDefault(profilesFile)

	_ = config.LOG_LEVEL.BindEnv()
	config.LOG_LEVEL.SetDefault("info")

	if err := viper.ReadInConfig(); err != nil {
		// File not found is OK if ENV is set
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return err // Config file was found but another error was produced
		}
	}

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
		"profile",
		func(c *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			//nolint:mnd // 150ms is a good compromise between snappy completion and enough time to read files
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			profilesFile, err := config.GetProfilesFileFromEnvAndFlags(c)
			if err != nil {
				// fail silent to keep completion snappy
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			profs, err := config.AutoCompleteListProfileNames(ctx, nil, profilesFile)
			if err != nil {
				// fail silent to keep completion snappy
				return nil, cobra.ShellCompDirectiveNoFileComp
			}

			// Optionally add descriptions: "value\tpath" for nicer columns
			return profs, cobra.ShellCompDirectiveNoFileComp
		},
	)
}
