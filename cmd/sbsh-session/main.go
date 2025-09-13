/*
Copyright © 2025 Emiliano Spinella (eminwux)
*/

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sbsh/pkg/api"
	"sbsh/pkg/common"
	"sbsh/pkg/session"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	sessionID  string
	sessionCmd string
)

func main() {
	Execute()
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sbsh-session",
	Short: "A brief description of your application",
	Long:  `A longer description ...`,
	Run: func(cmd *cobra.Command, args []string) {

		err := LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Config error:", err)
			os.Exit(1)
		}

		if sessionID == "" {
			sessionID = common.RandomID()
		}
		if sessionCmd == "" {
			sessionCmd = "/bin/bash"
		}

		// Split into args for exec
		cmdArgs := []string{}

		runSession(sessionID, sessionCmd, cmdArgs)

	},
}

func runSession(sessionID string, sessionCmd string, cmdArgs []string) {
	// Top-level context also reacts to SIGINT/SIGTERM (nice UX)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Define a new Session
	spec := api.SessionSpec{
		ID:          api.SessionID(sessionID),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     sessionCmd,
		CommandArgs: cmdArgs,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
	}

	// Create error channel
	exit := make(chan error, 1)

	// Create a new Controller
	var sessionCtrl api.SessionController

	sessionCtrl = session.NewSessionController(ctx, exit)

	// Add new session
	if err := sessionCtrl.AddSession(&spec); err != nil {
		log.Fatalf("failed to start session: %v", err)
		return
	}

	go sessionCtrl.Run()

	// block until controller is ready (or ctx cancels)
	if err := sessionCtrl.WaitReady(); err != nil {
		log.Printf("controller not ready: %s", err)
		return
	}

	select {
	case <-ctx.Done():
		log.Printf("[sbsh-session] context done\r\n")
		return
	case err := <-exit:
		if err != nil {
			log.Printf("[sbsh-session] controller stopped with error: %v\r\n", err)
		}
		log.Printf("[sbsh-session] normal shutdown\r\n")

		return
	}
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVar(&sessionID, "id", "", "Optional session ID (random if omitted)")
	rootCmd.Flags().StringVar(&sessionCmd, "command", "", "Optional command (default: bash -i)")

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

	if err := viper.ReadInConfig(); err != nil {
		// File not found is OK if ENV is set
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {

		} else {
			return err // Config file was found but another error was produced
		}
	}
	return nil
}
