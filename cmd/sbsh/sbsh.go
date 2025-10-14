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
	"log"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/eminwux/sbsh/cmd/sbsh/run"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/env"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/logging"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/internal/profile"
	"github.com/eminwux/sbsh/internal/supervisor"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewSbshRootCmd() *cobra.Command {
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
  sbsh attach --id abcdef0
`,
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

			supLogfile := viper.GetString("sbsh.supervisor.logFile")
			if supLogfile == "" {
				supLogfile = filepath.Join(
					viper.GetString(env.RUN_PATH.ViperKey),
					"supervisors",
					supervisorID,
					"log",
				)
			}

			supLogLevel := viper.GetString("sbsh.supervisor.logLevel")
			if supLogLevel == "" {
				supLogLevel = "info"
			}

			if supLogfile != "" {
				if err := os.MkdirAll(filepath.Dir(supLogfile), 0o700); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
					os.Exit(1)
				}

				f, err := os.OpenFile(supLogfile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
					os.Exit(1)
				}

				// Create a new logger that writes to the file with the specified log level
				lv := new(slog.LevelVar)
				lv.Set(logging.ParseLevel(supLogLevel))

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

			if viper.GetBool("sbsh.supervisor.detach") {
				detachSelf()
				return nil
			}

			logger.DebugContext(
				cmd.Context(), "parameters received in sbsh",
				"runPath", viper.GetString(env.RUN_PATH.ViperKey),
				"configFile", viper.GetString(env.CONFIG_FILE.ViperKey),
				"logLevel", viper.GetString(env.LOG_LEVEL.ViperKey),
				"profilesFile", viper.GetString(env.PROFILES_FILE.ViperKey),
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
			if supervisorName != "" {
				supervisorName = naming.RandomName()
				viper.Set("sbsh.supervisor.name", supervisorName)
			}

			socketFileInput := viper.GetString("sbsh.supervisor.socket")
			if socketFileInput == "" {
				socketFileInput = filepath.Join(
					viper.GetString(env.RUN_PATH.ViperKey),
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
					RunPath:      viper.GetString(env.RUN_PATH.ViperKey),
					ProfilesFile: viper.GetString(env.PROFILES_FILE.ViperKey),
					ProfileName:  viper.GetString("sbsh.session.profile"),
					LogFile:      viper.GetString("sbsh.session.logFile"),
					LogLevel:     viper.GetString("sbsh.session.logLevel"),
					SocketFile:   viper.GetString("sbsh.session.socket"),
					EnvVars:      os.Environ(),
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
				Env:         os.Environ(),
				RunPath:     viper.GetString(env.RUN_PATH.ViperKey),
				LogFile:     supLogfile,
				SockerCtrl:  socketFileInput,
				SessionSpec: sessionSpec,
			}

			logger.DebugContext(cmd.Context(), "SupervisorSpec values",
				"Kind", spec.Kind,
				"ID", spec.ID,
				"Name", spec.Name,
				"Env", spec.Env,
				"RunPath", spec.RunPath,
				"SockerCtrl", spec.SockerCtrl,
				"SessionSpec", spec.SessionSpec,
			)

			if logger.Enabled(cmd.Context(), slog.LevelDebug) {
				if printErr := discovery.PrintSessionSpec(sessionSpec, logger); printErr != nil {
					logger.Warn("Failed to print session spec", "error", printErr)
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			ctrl := supervisor.NewSupervisorController(ctx, logger)

			return runSupervisor(ctx, cancel, logger, ctrl, spec)
		},
		PostRunE: func(cmd *cobra.Command, _ []string) error {
			if c, _ := cmd.Context().Value(logging.CtxCloser).(io.Closer); c != nil {
				_ = c.Close()
			}
			return nil
		},
	}

	setupRootCmd(rootCmd)

	return rootCmd
}

func setupRootCmd(rootCmd *cobra.Command) {
	// go http.ListenAndServe("127.0.0.1:6060", nil)
	// runtime.SetBlockProfileRate(1)     // sample ALL blocking events on chans/locks
	// runtime.SetMutexProfileFraction(1) // sample ALL mutex contention

	rootCmd.AddCommand(run.NewRunCmd())

	// Persistent flags
	rootCmd.PersistentFlags().String("run-path", "", "Optional run path for the supervisor")
	_ = viper.BindPFlag(env.RUN_PATH.ViperKey, rootCmd.PersistentFlags().Lookup("run-path"))

	rootCmd.PersistentFlags().String("config", "", "config file (default is $HOME/.sbsh/config.yaml)")
	_ = viper.BindPFlag(env.CONFIG_FILE.ViperKey, rootCmd.PersistentFlags().Lookup("config"))

	rootCmd.PersistentFlags().String("profiles", "", "profiles manifests file")
	_ = viper.BindPFlag(env.PROFILES_FILE.ViperKey, rootCmd.PersistentFlags().Lookup("profiles"))

	// Bind Non-persistent Flags to Viper
	// Supervisor flags
	rootCmd.Flags().String("id", "", "Optional ID for the supervisor")
	_ = viper.BindPFlag("sbsh.supervisor.id", rootCmd.Flags().Lookup("id"))

	rootCmd.Flags().String("socket", "", "Optional socket file for the session")
	_ = viper.BindPFlag("sbsh.supervisor.socket", rootCmd.Flags().Lookup("socket"))

	rootCmd.Flags().String("log-file", "", "Optional socket file for the supervisor")
	_ = viper.BindPFlag("sbsh.supervisor.logFile", rootCmd.Flags().Lookup("log-file"))

	rootCmd.Flags().String("log-level", "", "Optional log level for the supervisor")
	_ = viper.BindPFlag("sbsh.supervisor.logLevel", rootCmd.Flags().Lookup("log-level"))

	rootCmd.Flags().BoolP("detach", "d", false, "Optional socket file for the session")
	_ = viper.BindPFlag("sbsh.supervisor.detach", rootCmd.Flags().Lookup("detach"))

	// Session flags
	rootCmd.Flags().String("session-id", "", "Optional session ID (random if omitted)")
	_ = viper.BindPFlag("sbsh.session.id", rootCmd.Flags().Lookup("session-id"))

	rootCmd.Flags().String("session-command", "", "Optional command (default: /bin/bash)")
	_ = viper.BindPFlag("sbsh.session.command", rootCmd.Flags().Lookup("session-command"))

	rootCmd.Flags().String("session-name", "", "Optional name for the session")
	_ = viper.BindPFlag("sbsh.session.name", rootCmd.Flags().Lookup("session-name"))

	rootCmd.Flags().String("capture-file", "", "Optional filename for the session log")
	_ = viper.BindPFlag("sbsh.session.captureFile", rootCmd.Flags().Lookup("capture-file"))

	rootCmd.Flags().String("session-logfile", "", "Optional filename for the session log")
	_ = viper.BindPFlag("sbsh.session.logFile", rootCmd.Flags().Lookup("session-logfile"))

	rootCmd.Flags().String("session-loglevel", "", "Optional log level for the session")
	_ = viper.BindPFlag("sbsh.session.logLevel", rootCmd.Flags().Lookup("session-loglevel"))

	rootCmd.Flags().StringP("profile", "p", "", "Optional profile for the session")
	_ = viper.BindPFlag("sbsh.session.profile", rootCmd.Flags().Lookup("profile"))
	_ = viper.BindEnv("sbsh.session.profile", env.SES_PROFILE.EnvVar())

	rootCmd.Flags().String("session-socket", "", "Optional socket file for the session")
	_ = viper.BindPFlag("sbsh.session.socket", rootCmd.Flags().Lookup("session-socket"))
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
	if viper.GetString(env.CONFIG_FILE.ViperKey) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("err: %v", err)
		}
		configPath := filepath.Join(home, ".sbsh")
		configFile = filepath.Join(configPath, "config.yaml")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(configFile)
	}
	_ = env.CONFIG_FILE.BindEnv()
	env.CONFIG_FILE.Set(configFile)

	var runPath string
	if viper.GetString(env.RUN_PATH.ViperKey) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("err: %v", err)
		}
		runPath = filepath.Join(home, ".sbsh", "run")
	}
	_ = env.RUN_PATH.BindEnv()
	env.RUN_PATH.SetDefault(runPath)

	var profilesFile string
	if viper.GetString(env.PROFILES_FILE.ViperKey) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("err: %v", err)
		}
		profilesFile = filepath.Join(home, ".sbsh", "profiles.yaml")
	}
	_ = env.PROFILES_FILE.BindEnv()
	env.PROFILES_FILE.SetDefault(profilesFile)

	_ = env.LOG_LEVEL.BindEnv()
	env.LOG_LEVEL.SetDefault("info")

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
