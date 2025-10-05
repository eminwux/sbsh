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
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sbsh/cmd/sb/detach"
	"sbsh/cmd/sb/profiles"
	"sbsh/cmd/sb/sessions"
	"sbsh/pkg/common"
	"sbsh/pkg/env"
)

func main() {
	Execute()
}

var (
	logLevel string
	runPath  string
	cfgFile  string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sb",
	Short: "A brief description of your application",
	Long:  `A longer description ...`,
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

///////////////////////////////////////////////

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(sessions.SessionsCmd)
	rootCmd.AddCommand(detach.DetachCmd)
	rootCmd.AddCommand(profiles.ProfilesCmd)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.sbsh/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&runPath, "run-path", "$HOME/.sbsh/run", "Log level (debug, info, warn, error)")

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

// LoadConfig loads config.yaml from the given path or HOME/.sbsh
func LoadConfig() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("err: %v", err)
	}
	configFile := filepath.Join(home, ".sbsh")
	viper.AddConfigPath(configFile)

	_ = env.RUN_PATH.BindEnv()
	_ = env.LOG_LEVEL.BindEnv()

	env.RUN_PATH.SetDefault(filepath.Join(home, ".sbsh", "run"))
	env.LOG_LEVEL.SetDefault("info")

	if err := viper.ReadInConfig(); err != nil {
		// File not found is OK if ENV is set
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			slog.Debug("no config file found")
		} else {
			return err
		}
	}

	return nil
}
