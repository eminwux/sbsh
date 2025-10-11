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

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/eminwux/sbsh/cmd/sbsh/attach"
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
	rootCmd := NewRootCmd()

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func NewRootCmd() *cobra.Command {
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

			h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: common.ParseLevel(viper.GetString(env.LOG_LEVEL.ViperKey)),
			})
			slog.SetDefault(slog.New(h))
			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			// Build SessionSpec from command-line inputs and profile (if given)
			sessionSpec, err := profile.BuildSessionSpec(
				viper.GetString(env.RUN_PATH.ViperKey),
				viper.GetString(env.PROFILES_FILE.ViperKey),
				viper.GetString("main.session.profile"),
				viper.GetString("main.session.id"),
				viper.GetString("main.session.name"),
				viper.GetString("main.session.command"),
				viper.GetString("main.session.logFilename"),
				viper.GetString("main.session.socket"),
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

			slog.Debug("SupervisorSpec values",
				"Kind", spec.Kind,
				"ID", spec.ID,
				"Name", spec.Name,
				"Env", spec.Env,
				"RunPath", spec.RunPath,
				"SockerCtrl", spec.SockerCtrl,
				"SessionSpec", spec.SessionSpec,
			)

			if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
				discovery.PrintSessionSpec(sessionSpec, os.Stdout)
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			ctrl := supervisor.NewSupervisorController(ctx)

			return runSupervisor(ctx, cancel, ctrl, spec)
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
	rootCmd.AddCommand(attach.NewAttachCmd())

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
	rootCmd.Flags().String("socket", "", "Optional socket file for the session")
	// Session flags
	rootCmd.Flags().String("session-id", "", "Optional session ID (random if omitted)")
	rootCmd.Flags().String("session-command", "/bin/bash", "Optional command (default: /bin/bash)")
	rootCmd.Flags().String("session-name", "", "Optional name for the session")
	rootCmd.Flags().String("session-log-filename", "", "Optional filename for the session log")
	rootCmd.Flags().String("session-profile", "", "Optional profile for the session")
	rootCmd.Flags().String("session-socket", "", "Optional socket file for the session")

	_ = viper.BindPFlag("main.session.id", rootCmd.Flags().Lookup("session-id"))
	_ = viper.BindPFlag("main.session.command", rootCmd.Flags().Lookup("session-command"))
	_ = viper.BindPFlag("main.session.name", rootCmd.Flags().Lookup("session-name"))
	_ = viper.BindPFlag("main.session.logFilename", rootCmd.Flags().Lookup("session-log-filename"))
	_ = viper.BindPFlag("main.session.profile", rootCmd.Flags().Lookup("session-profile"))
	_ = viper.BindPFlag("main.session.socket", rootCmd.Flags().Lookup("session-socket"))
}

func runSupervisor(
	ctx context.Context,
	cancel context.CancelFunc,
	ctrl api.SupervisorController,
	spec *api.SupervisorSpec,
) error {
	defer cancel()

	// Create error channel
	errCh := make(chan error, 1)

	// Run controller
	go func() {
		errCh <- ctrl.Run(spec) // Run should return when ctx is canceled
		close(errCh)
		slog.Debug("[sbsh] controller stopped\r\n")
	}()

	// block until controller is ready (or ctx cancels)
	if err := ctrl.WaitReady(); err != nil {
		slog.Debug(fmt.Sprintf("controller not ready: %s", err))
		return fmt.Errorf("%w: %w", errdefs.ErrWaitOnReady, err)
	}

	select {
	case <-ctx.Done():
		var err error
		slog.Debug("[sbsh] context canceled, waiting on sessionCtrl to exit\r\n")
		if errC := ctrl.WaitClose(); errC != nil {
			err = fmt.Errorf("%w: %w: %w", err, errdefs.ErrWaitOnClose, errC)
		}
		slog.Debug("[sbsh] context canceled, sessionCtrl exited\r\n")

		return fmt.Errorf("%w: %w", errdefs.ErrContextDone, err)

	case err := <-errCh:
		slog.Debug(fmt.Sprintf("[sbsh] controller stopped with error: %v\r\n", err))
		if err != nil && !errors.Is(err, context.Canceled) {
			err = fmt.Errorf("%w: %w", errdefs.ErrChildExit, err)
			if errC := ctrl.WaitClose(); err != nil {
				err = fmt.Errorf("%w: %w: %w", err, errdefs.ErrWaitOnClose, errC)
			}
			slog.Debug("[sbsh-session] context canceled, sessionCtrl exited\r\n")

			return err
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
