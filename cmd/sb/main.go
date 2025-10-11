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
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/eminwux/sbsh/cmd/sb/detach"
	"github.com/eminwux/sbsh/cmd/sb/profiles"
	"github.com/eminwux/sbsh/cmd/sb/sessions"
	"github.com/eminwux/sbsh/internal/common"
	"github.com/eminwux/sbsh/internal/env"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func main() {
	err := newRootCmd().Execute()
	if err != nil {
		os.Exit(1)
	}
}

var (
	logLevelInput string
	runPathInput  string
	cfgFileInput  string
)

func newRootCmd() *cobra.Command {
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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := LoadConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Config error:", err)
				os.Exit(1)
			}

			h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: common.ParseLevel(viper.GetString("global.logLevel")),
			})
			slog.SetDefault(slog.New(h))
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
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

	rootCmd.PersistentFlags().StringVar(&cfgFileInput, "config", "", "config file (default is $HOME/.sbsh/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&logLevelInput, "log-level", "", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&runPathInput, "run-path", "", "Log level (debug, info, warn, error)")

	// Bind flag to Viper
	if err := viper.BindPFlag("global.config", rootCmd.PersistentFlags().Lookup("config")); err != nil {
		log.Fatal(err)
	}
	if err := viper.BindPFlag("global.logLevel", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		log.Fatal(err)
	}
	if err := viper.BindPFlag("global.runPath", rootCmd.PersistentFlags().Lookup("run-path")); err != nil {
		log.Fatal(err)
	}
}

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
	if err := env.CONFIG_FILE.Set(configFile); err != nil {
		return fmt.Errorf("failed to set config file: %w", err)
	}

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
