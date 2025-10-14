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

package sb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/eminwux/sbsh/cmd/sb/attach"
	"github.com/eminwux/sbsh/cmd/sb/detach"
	"github.com/eminwux/sbsh/cmd/sb/profiles"
	"github.com/eminwux/sbsh/cmd/sb/sessions"
	"github.com/eminwux/sbsh/internal/env"
	"github.com/eminwux/sbsh/internal/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewSbRootCmd() *cobra.Command {
	// rootCmd represents the base command when called without any subcommands.
	rootCmd := &cobra.Command{
		Use:   "sb",
		Short: "sb command line tool",
		Long: `sb is a command line tool to manage sbsh sessions and profiles.

You can see available options and commands with:
	sb help

Examples:
	sb sessions list
	sb sessions prune
	sb detach
	sb attach --id abcdf0
	sb profiles list
`,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			logger, ok := cmd.Context().Value(logging.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
			}
			logger.DebugContext(cmd.Context(), "loading config in PersistentPreRunE")
			err := LoadConfig()
			if err != nil {
				logger.DebugContext(cmd.Context(), "config error", "error", err)
				return fmt.Errorf("config error: %w", err)
			}

			// Set log level dynamically if present
			levelVar, ok := cmd.Context().Value(logging.CtxLevelVar).(*slog.LevelVar)
			if ok && levelVar != nil {
				logger.DebugContext(
					cmd.Context(),
					"setting log level from viper",
					"level",
					viper.GetString("sb.global.logLevel"),
				)
				levelVar.Set(logging.ParseLevel(viper.GetString("sb.global.logLevel")))
			}

			if !viper.GetBool("sb.global.verbose") {
				logger.DebugContext(
					cmd.Context(),
					"enabling verbose",
					"log-level",
					viper.GetString("sb.global.logLevel"),
				)
				handler, hOk := cmd.Context().Value(logging.CtxHandler).(*logging.ReformatHandler)
				if !hOk || handler == nil {
					return errors.New("logger handler not found in context")
				}
				devNull, errC := os.OpenFile("/dev/null", os.O_WRONLY, 0)
				if errC != nil {
					return fmt.Errorf("failed to open /dev/null: %w", errC)
				}
				handler.Inner = slog.NewTextHandler(devNull, &slog.HandlerOptions{Level: levelVar})
				handler.Writer = devNull

				ctx := cmd.Context()
				ctx = context.WithValue(ctx, logging.CtxLevelVar, levelVar)
				ctx = context.WithValue(ctx, logging.CtxCloser, devNull)
				cmd.SetContext(ctx)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger, ok := cmd.Context().Value(logging.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
			}
			logger.DebugContext(cmd.Context(), "sb root command invoked", "args", cmd.Flags().Args())
			return cmd.Help()
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
	rootCmd.AddCommand(sessions.NewSessionsCmd())
	rootCmd.AddCommand(detach.NewDetachCmd())
	rootCmd.AddCommand(profiles.NewProfilesCmd())
	rootCmd.AddCommand(attach.NewAttachCmd())

	rootCmd.PersistentFlags().String("config", "", "config file (default is $HOME/.sbsh/config.yaml)")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("log-level", "", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("run-path", "", "Run path directory")

	_ = viper.BindPFlag("sb.global.config", rootCmd.PersistentFlags().Lookup("config"))
	_ = viper.BindPFlag("sb.global.verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	_ = viper.BindPFlag("sb.global.logLevel", rootCmd.PersistentFlags().Lookup("log-level"))
	_ = viper.BindPFlag("sb.global.runPath", rootCmd.PersistentFlags().Lookup("run-path"))
}

func LoadConfig() error {
	var configFile string
	if viper.GetString(env.CONFIG_FILE.ViperKey) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user home dir: %w", err)
		}
		configPath := filepath.Join(home, ".sbsh")
		configFile = filepath.Join(configPath, "config.yaml")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		// Add the directory containing the config file
		viper.AddConfigPath(configPath)
	}
	_ = env.CONFIG_FILE.BindEnv()
	if err := env.CONFIG_FILE.Set(configFile); err != nil {
		return fmt.Errorf("failed to set config file: %w", err)
	}

	var runPath string
	if viper.GetString(env.RUN_PATH.ViperKey) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user home dir: %w", err)
		}
		runPath = filepath.Join(home, ".sbsh", "run")
	}
	_ = env.RUN_PATH.BindEnv()
	env.RUN_PATH.SetDefault(runPath)

	var profilesFile string
	if viper.GetString(env.PROFILES_FILE.ViperKey) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user home dir: %w", err)
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
