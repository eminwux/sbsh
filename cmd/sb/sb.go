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

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/sb/attach"
	"github.com/eminwux/sbsh/cmd/sb/autocomplete"
	"github.com/eminwux/sbsh/cmd/sb/detach"
	"github.com/eminwux/sbsh/cmd/sb/get"
	"github.com/eminwux/sbsh/cmd/sb/profiles"
	"github.com/eminwux/sbsh/cmd/sb/prune"
	"github.com/eminwux/sbsh/internal/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewSbRootCmd() (*cobra.Command, error) {
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
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			var logger *slog.Logger
			if viper.GetBool("sb.global.verbose") {
				logLevel := viper.GetString("sb.global.logLevel")
				if logLevel == "" {
					logLevel = "info"
				}

				// Create a new logger that writes to the file with the specified log level
				levelVar := new(slog.LevelVar)
				levelVar.Set(logging.ParseLevel(logLevel))

				textHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: levelVar})
				handler := &logging.ReformatHandler{Inner: textHandler, Writer: os.Stdout}
				logger = slog.New(handler)

				// Store both logger and levelVar in context using struct keys
				ctx := cmd.Context()
				ctx = context.WithValue(ctx, logging.CtxLogger, logger)
				ctx = context.WithValue(ctx, logging.CtxLevelVar, &levelVar)
				ctx = context.WithValue(ctx, logging.CtxHandler, handler)
				cmd.SetContext(ctx)
				logger.DebugContext(
					cmd.Context(),
					"enabling verbose",
					"log-level",
					viper.GetString("sb.global.logLevel"),
				)
			}

			err := LoadConfig()
			if err != nil {
				logger.DebugContext(cmd.Context(), "config error", "error", err)
				return fmt.Errorf("config error: %w", err)
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

	err := setupRootCmd(rootCmd)
	if err != nil {
		return nil, err
	}
	return rootCmd, nil
}

func setupRootCmd(rootCmd *cobra.Command) error {
	rootCmd.AddCommand(autocomplete.NewAutoCompleteCmd(rootCmd))
	rootCmd.AddCommand(prune.NewPruneCmd())
	rootCmd.AddCommand(detach.NewDetachCmd())
	rootCmd.AddCommand(profiles.NewProfilesCmd())
	rootCmd.AddCommand(attach.NewAttachCmd())
	rootCmd.AddCommand(get.NewGetCmd())

	rootCmd.PersistentFlags().String("config", "", "config file (default is $HOME/.sbsh/config.yaml)")
	if err := viper.BindPFlag("sb.global.config", rootCmd.PersistentFlags().Lookup("config")); err != nil {
		return err
	}

	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Log level (debug, info, warn, error)")
	if err := viper.BindPFlag("sb.global.verbose", rootCmd.PersistentFlags().Lookup("verbose")); err != nil {
		return err
	}

	rootCmd.PersistentFlags().String("log-level", "", "Log level (debug, info, warn, error)")
	if err := viper.BindPFlag("sb.global.logLevel", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		return err
	}

	rootCmd.PersistentFlags().String("run-path", "", "Run path directory")
	if err := viper.BindPFlag("sb.global.runPath", rootCmd.PersistentFlags().Lookup("run-path")); err != nil {
		return err
	}

	return nil
}

func LoadConfig() error {
	var configFile string
	if viper.GetString(config.CONFIG_FILE.ViperKey) == "" {
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
	_ = config.CONFIG_FILE.BindEnv()
	if err := config.CONFIG_FILE.Set(configFile); err != nil {
		return fmt.Errorf("failed to set config file: %w", err)
	}

	var runPath string
	if viper.GetString(config.RUN_PATH.ViperKey) == "" {
		var err error
		runPath, err = config.DefaultRunPath()
		if err != nil {
			return errors.New("cannot determine default run path: " + err.Error())
		}
	}
	_ = config.RUN_PATH.BindEnv()
	config.RUN_PATH.SetDefault(runPath)

	var profilesFile string
	if viper.GetString(config.PROFILES_FILE.ViperKey) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user home dir: %w", err)
		}
		profilesFile = filepath.Join(home, ".sbsh", "profiles.yaml")
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
