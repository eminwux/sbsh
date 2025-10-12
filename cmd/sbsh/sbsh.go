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
	"log"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/eminwux/sbsh/cmd/sbsh/run"
	"github.com/eminwux/sbsh/internal/common"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/env"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/internal/profile"
	"github.com/eminwux/sbsh/internal/supervisor"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func main() {
	var levelVar slog.LevelVar
	// Default to info, can be changed at runtime
	levelVar.Set(slog.LevelInfo)
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: &levelVar})
	logger := slog.New(handler)

	// Store both logger and levelVar in context using struct keys
	//nolint:revive,staticcheck // ignore revive warning about context keys
	ctx := context.WithValue(context.Background(), "logger", logger)
	//nolint:revive,staticcheck // ignore revive warning about context keys
	ctx = context.WithValue(ctx, "logLevelVar", &levelVar)

	rootCmd := NewSbshRootCmd()
	rootCmd.SetContext(ctx)

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

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
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			err := LoadConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Config error:", err)
				os.Exit(1)
			}

			// Set log level dynamically if present
			levelVar, ok := cmd.Context().Value("logLevelVar").(*slog.LevelVar)
			if ok && levelVar != nil {
				levelVar.Set(common.ParseLevel(viper.GetString("sbsh.global.logLevel")))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger, ok := cmd.Context().Value("logger").(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
			}

			logger.DebugContext(cmd.Context(), "parameters received in sbsh",
				"runPath", viper.GetString(env.RUN_PATH.ViperKey),
				"configFile", viper.GetString(env.CONFIG_FILE.ViperKey),
				"logLevel", viper.GetString(env.LOG_LEVEL.ViperKey),
				"profilesFile", viper.GetString(env.PROFILES_FILE.ViperKey),
				"sessionID", viper.GetString("sbsh.session.id"),
				"sessionCommand", viper.GetString("sbsh.session.command"),
				"sessionName", viper.GetString("sbsh.session.name"),
				"sessionLogFilename", viper.GetString("sbsh.session.logFilename"),
				"sessionProfile", viper.GetString("sbsh.session.profile"),
				"sessionSocket", viper.GetString("sbsh.session.socket"),
				"supervisorSocket", viper.GetString("sbsh.supervisor.socket"),
				"detach", viper.GetBool("sbsh.supervisor.detach"),
			)
			if viper.GetBool("run.supervisor.detach") {
				detachSelf()
				return nil
			}

			// Build SessionSpec from command-line inputs and profile (if given)
			sessionSpec, err := profile.BuildSessionSpec(
				viper.GetString(env.RUN_PATH.ViperKey),
				viper.GetString(env.PROFILES_FILE.ViperKey),
				viper.GetString("sbsh.session.profile"),
				viper.GetString("sbsh.session.id"),
				viper.GetString("sbsh.session.name"),
				viper.GetString("sbsh.session.command"),
				viper.GetString("sbsh.session.logFilename"),
				viper.GetString("sbsh.session.socket"),
				os.Environ(),
				context.Background(),
			)
			if err != nil {
				log.Fatal(err)
			}

			// If a profile name was given, set the SBSH_SES_PROFILE env var
			profileNameInput := viper.GetString("session.profile")
			if profileNameInput != "" {
				if err := env.SES_PROFILE.Set(profileNameInput); err != nil {
					log.Fatal(err)
				}
				if err := env.SES_PROFILE.BindEnv(); err != nil {
					log.Fatal(err)
				}
			}

			supervisorID := naming.RandomID()
			supervisorName := naming.RandomName()
			socketFileInput := viper.GetString("session.socket")
			if socketFileInput == "" {
				socketFileInput = filepath.Join(
					viper.GetString(env.RUN_PATH.ViperKey),
					"supervisors",
					supervisorID,
					"socket",
				)
			}
			// Define a new SupervisorSpec
			spec := &api.SupervisorSpec{
				Kind:        api.RunNewSession,
				ID:          api.ID(supervisorID),
				Name:        supervisorName,
				Env:         os.Environ(),
				RunPath:     viper.GetString(env.RUN_PATH.ViperKey),
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
				if printErr := discovery.PrintSessionSpec(sessionSpec, os.Stdout); printErr != nil {
					logger.Warn("Failed to print session spec", "error", printErr)
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			ctrl := supervisor.NewSupervisorController(ctx, logger)

			return runSupervisor(ctx, cancel, logger, ctrl, spec)
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
	rootCmd.PersistentFlags().String("config", "", "config file (default is $HOME/.sbsh/config.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("profiles", "", "profiles manifests file")

	_ = viper.BindPFlag(env.RUN_PATH.ViperKey, rootCmd.PersistentFlags().Lookup("run-path"))
	_ = viper.BindPFlag(env.CONFIG_FILE.ViperKey, rootCmd.PersistentFlags().Lookup("config"))
	_ = viper.BindPFlag(env.LOG_LEVEL.ViperKey, rootCmd.PersistentFlags().Lookup("log-level"))
	_ = viper.BindPFlag(env.PROFILES_FILE.ViperKey, rootCmd.PersistentFlags().Lookup("profiles"))

	// Bind Non-persistent Flags to Viper
	// Supervisor flags
	rootCmd.Flags().String("supervisor.socket", "", "Optional socket file for the session")
	// Session flags
	rootCmd.Flags().String("id", "", "Optional session ID (random if omitted)")
	rootCmd.Flags().String("command", "", "Optional command (default: /bin/bash)")
	rootCmd.Flags().String("name", "", "Optional name for the session")
	rootCmd.Flags().String("log-filename", "", "Optional filename for the session log")
	rootCmd.Flags().String("profile", "", "Optional profile for the session")
	rootCmd.Flags().String("socket", "", "Optional socket file for the session")
	rootCmd.Flags().BoolP("detach", "d", false, "Optional socket file for the session")

	_ = viper.BindPFlag("sbsh.session.id", rootCmd.Flags().Lookup("id"))
	_ = viper.BindPFlag("sbsh.session.command", rootCmd.Flags().Lookup("command"))
	_ = viper.BindPFlag("sbsh.session.name", rootCmd.Flags().Lookup("name"))
	_ = viper.BindPFlag("sbsh.session.logFilename", rootCmd.Flags().Lookup("log-filename"))
	_ = viper.BindPFlag("sbsh.session.profile", rootCmd.Flags().Lookup("profile"))
	_ = viper.BindPFlag("sbsh.session.socket", rootCmd.Flags().Lookup("socket"))
	_ = viper.BindPFlag("sbsh.supervisor.socket", rootCmd.Flags().Lookup("supervisor.socket"))
	_ = viper.BindPFlag("sbsh.supervisor.detach", rootCmd.Flags().Lookup("detach"))
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
