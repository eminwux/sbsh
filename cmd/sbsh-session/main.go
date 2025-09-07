/*
Copyright © 2025 Emiliano Spinella (eminwux)
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sbsh/pkg/api"
	"sbsh/pkg/session"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
		runSession()

	},
}

func runSession() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Define a new Session
	spec := api.SessionSpec{
		ID:      api.SessionID("s0"),
		Kind:    api.SessLocal,
		Label:   "default",
		Command: []string{"bash", "-i"},
		Env:     os.Environ(),
		LogDir:  "/tmp/sbsh-logs/s0",
	}

	// Create a new Controller
	var sessionCtrl api.SessionController

	sessionCtrl = session.NewSessionController()

	// Create error channel
	errCh := make(chan error, 1)

	// Run controller
	go func() {
		errCh <- sessionCtrl.Run(ctx) // Run should return when ctx is canceled
	}()

	// block until controller is ready (or ctx cancels)
	if err := sessionCtrl.WaitReady(ctx); err != nil {
		log.Printf("controller not ready: %s", err)
		return
	}

	// Start new session
	if err := sessionCtrl.StartSession(&spec); err != nil {
		log.Fatalf("failed to start session: %v", err)
	}

	select {
	case <-ctx.Done():
		// graceful shutdown path
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("controller stopped with error: %v\r\n", err)
		}
	}
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
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
