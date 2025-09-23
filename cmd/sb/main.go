/*
Copyright © 2025 Emiliano Spinella (eminwux)
*/

package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sbsh/cmd/sb/sessions"
	"sbsh/pkg/client"
	"sbsh/pkg/common"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func main() {
	Execute()
}

var logLevel string

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "sb",
	Short: "A brief description of your application",
	Long:  `A longer description ...`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		viper.BindEnv("SBSH_LOG_LEVEL")
		logLevel = viper.GetString("SBSH_LOG_LEVEL")
		h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: common.ParseLevel(logLevel),
		})
		slog.SetDefault(slog.New(h))
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {

		err := LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Config error:", err)
			os.Exit(1)
		}
		run()

	},
}

func run() {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new Controller
	// var ctrl api.Controller

	_, err := client.NewController("/home/inwx/.sbsh/socket")
	if err != nil {
		log.Printf("[client] Supervisor not running: %v ", err)
		return
	} else {
		log.Printf("[client] Supervisor is running: %v ", err)

	}

}

///////////////////////////////////////////////

func Execute() {
	err := RootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	RootCmd.AddCommand(sessions.SessionsCmd)
}

// LoadConfig loads config.yaml from the given path or HOME/.sbsh
func LoadConfig() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	// Then ~/.sbsh
	home, err := os.UserHomeDir()
	if err == nil {
		viper.AddConfigPath(filepath.Join(home, ".sbsh"))
	}

	// Bind ENV variable as a fallback
	// viper.BindEnv("SBSH_LOG_LEVEL")

	if err := viper.ReadInConfig(); err != nil {
		// File not found is OK if ENV is set
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {

		} else {
			return err // Config file was found but another error was produced
		}
	}
	return nil
}
