/*
Copyright © 2025 Emiliano Spinella (eminwux)
*/

package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sbsh/cmd/sb/sessions"
	"sbsh/pkg/common"
	"sbsh/pkg/env"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

		err := LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Config error:", err)
			os.Exit(1)
		}

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
	viper.AddConfigPath(filepath.Join(home, ".sbsh"))

	_ = env.RUN_PATH.BindEnv()
	_ = env.LOG_LEVEL.BindEnv()

	env.RUN_PATH.SetDefault(filepath.Join(home, ".sbsh", "run"))
	env.LOG_LEVEL.SetDefault("info")

	if err := viper.ReadInConfig(); err != nil {
		// File not found is OK if ENV is set
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {

		} else {
			return err // Config file was found but another error was produced
		}
	}

	return nil
}
