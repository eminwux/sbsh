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
	"log/slog"
	"os"
	"path/filepath"

	"github.com/eminwux/sbsh/cmd/sb/attach"
	"github.com/eminwux/sbsh/cmd/sb/detach"
	"github.com/eminwux/sbsh/cmd/sb/profiles"
	"github.com/eminwux/sbsh/cmd/sb/sessions"
	"github.com/eminwux/sbsh/internal/common"
	"github.com/eminwux/sbsh/internal/env"
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

	rootCmd := NewSbRootCmd()
	rootCmd.SetContext(ctx)

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

// no package-level state to satisfy gochecknoglobals

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
			err := LoadConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Config error:", err)
				os.Exit(1)
			}

			// Set log level dynamically if present
			levelVar, ok := cmd.Context().Value("logLevelVar").(*slog.LevelVar)
			if ok && levelVar != nil {
				levelVar.Set(common.ParseLevel(viper.GetString("global.logLevel")))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger, ok := cmd.Context().Value("logger").(*slog.Logger)
			if !ok || logger == nil {
				return errors.New("logger not found in context")
			}
			logger.Info("sb", "args", cmd.Flags().Args())

			return cmd.Help()
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
	rootCmd.PersistentFlags().String("log-level", "", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("run-path", "", "Run path directory")

	// Bind flag to Viper
	if err := viper.BindPFlag("global.config", rootCmd.PersistentFlags().Lookup("config")); err != nil {
		slog.Warn("failed to bind flag", "flag", "config", "error", err)
	}
	if err := viper.BindPFlag("global.logLevel", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		slog.Warn("failed to bind flag", "flag", "log-level", "error", err)
	}
	if err := viper.BindPFlag("global.runPath", rootCmd.PersistentFlags().Lookup("run-path")); err != nil {
		slog.Warn("failed to bind flag", "flag", "run-path", "error", err)
	}
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
