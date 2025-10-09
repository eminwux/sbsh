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

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sbsh/cmd/sbsh/attach"
	"sbsh/cmd/sbsh/run"
	"sbsh/pkg/api"
	"sbsh/pkg/common"
	"sbsh/pkg/env"
	"sbsh/pkg/naming"
	"sbsh/pkg/profile"
	"sbsh/pkg/supervisor"
)

var (
	newSupervisorController = supervisor.NewSupervisorController
	ctx                     context.Context
	cancel                  context.CancelFunc
)

var (
	logLevelInput           string
	runPathInput            string
	cfgFileInput            string
	profilesFileInput       string
	sessionProfileNameInput string
	sessionIDInput          string
	sessionNameInput        string
	sessionCmdInput         string
	sessionLogFilenameInput string
	socketFileInput         string
	sessionSocketFileInput  string
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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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
		Run: func(cmd *cobra.Command, args []string) {
			// Build SessionSpec from command-line inputs and profile (if given)
			sessionSpec, err := profile.BuildSessionSpec(
				viper.GetString(env.RUN_PATH.ViperKey),
				viper.GetString(env.PROFILES_FILE.ViperKey),
				sessionProfileNameInput,
				sessionIDInput,
				sessionNameInput,
				sessionCmdInput,
				sessionLogFilenameInput,
				sessionSocketFileInput,
				os.Environ(),
				ctx,
			)
			if err != nil {
				log.Fatal(err)
			}

			// If a profile name was given, set the SBSH_SES_PROFILE env var
			if sessionProfileNameInput != "" {
				if err := env.SES_PROFILE.Set(sessionProfileNameInput); err != nil {
					log.Fatal(err)
				}
				if env.SES_PROFILE.BindEnv() != nil {
					log.Fatal(err)
				}
			}

			supervisorID := naming.RandomID()
			supervisorName := naming.RandomName()

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

			// discovery.PrintSessionSpec(sessionSpec, os.Stdout)
			runSupervisor(spec)
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
	rootCmd.PersistentFlags().StringVar(&runPathInput, "run-path", "", "Optional run path for the supervisor")
	rootCmd.PersistentFlags().StringVar(&cfgFileInput, "config", "", "config file (default is $HOME/.sbsh/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&logLevelInput, "log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().
		StringVar(&profilesFileInput, "profiles", "", "profiles manifests file")

	if err := viper.BindPFlag(env.RUN_PATH.ViperKey, rootCmd.PersistentFlags().Lookup("run-path")); err != nil {
		log.Fatal(err)
	}

	if err := viper.BindPFlag(env.CONFIG_FILE.ViperKey, rootCmd.PersistentFlags().Lookup("config")); err != nil {
		log.Fatal(err)
	}

	if err := viper.BindPFlag(env.LOG_LEVEL.ViperKey, rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		log.Fatal(err)
	}

	if err := viper.BindPFlag(env.PROFILES_FILE.ViperKey, rootCmd.PersistentFlags().Lookup("profiles")); err != nil {
		log.Fatal(err)
	}

	// Bind Non-persistent Flags to Viper
	// Supervisor flags
	rootCmd.Flags().StringVar(&socketFileInput, "socket", "", "Optional socket file for the session")
	// Session flags
	rootCmd.Flags().StringVar(&sessionIDInput, "session-id", "", "Optional session ID (random if omitted)")
	rootCmd.Flags().StringVar(&sessionCmdInput, "session-command", "/bin/bash", "Optional command (default: /bin/bash)")
	rootCmd.Flags().StringVar(&sessionNameInput, "session-name", "", "Optional name for the session")
	rootCmd.Flags().
		StringVar(&sessionLogFilenameInput, "session-log-filename", "", "Optional filename for the session log")
	rootCmd.Flags().StringVar(&sessionProfileNameInput, "session-profile", "", "Optional profile for the session")
	rootCmd.Flags().StringVar(&sessionSocketFileInput, "session-socket", "", "Optional socket file for the session")

	if err := viper.BindPFlag("session.logFilename", rootCmd.Flags().Lookup("session-log-filename")); err != nil {
		log.Fatal(err)
	}
}

func runSupervisor(spec *api.SupervisorSpec) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create a new Controller
	ctrl := newSupervisorController(ctx)

	// Create error channel
	errCh := make(chan error, 1)

	// Run controller
	go func() {
		errCh <- ctrl.Run(spec) // Run should return when ctx is canceled
		slog.Debug("[sbsh] controller stopped\r\n")
	}()

	// block until controller is ready (or ctx cancels)
	if err := ctrl.WaitReady(); err != nil {
		slog.Debug(fmt.Sprintf("controller not ready: %s", err))
		return fmt.Errorf("%w: %w", ErrWaitOnReady, err)
	}

	select {
	case <-ctx.Done():
		var err error
		slog.Debug("[sbsh] context canceled, waiting on sessionCtrl to exit\r\n")
		if e := ctrl.WaitClose(); e != nil {
			err = fmt.Errorf("%w: %w", ErrWaitOnClose, e)
		}
		slog.Debug("[sbsh] context canceled, sessionCtrl exited\r\n")
		return fmt.Errorf("%w: %w", ErrContextDone, err)

	case err := <-errCh:
		slog.Debug(fmt.Sprintf("[sbsh] controller stopped with error: %v\r\n", err))
		if err != nil && !errors.Is(err, context.Canceled) {
			err = fmt.Errorf("%w: %w", ErrChildExit, err)
			if errC := ctrl.WaitClose(); err != nil {
				err = fmt.Errorf("%w: %w: %w", err, ErrWaitOnClose, errC)
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
