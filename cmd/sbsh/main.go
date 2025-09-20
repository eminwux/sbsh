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
	"os/signal"
	"path/filepath"
	"sbsh/pkg/api"
	"sbsh/pkg/supervisor"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var newSupervisorController = supervisor.NewSupervisorController
var ctx context.Context
var cancel context.CancelFunc

func main() {
	Execute()
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sbsh",
	Short: "A brief description of your application",
	Long:  `A longer description ...`,
	Run: func(cmd *cobra.Command, args []string) {

		err := LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Config error:", err)
			os.Exit(1)
		}
		runSupervisor()

	},
}

func runSupervisor() error {
	ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create a new Controller
	var ctrl api.SupervisorController

	ctrl = newSupervisorController(ctx)

	// Create error channel
	errCh := make(chan error, 1)

	// Run controller
	go func() {
		errCh <- ctrl.Run(ctx) // Run should return when ctx is canceled
		log.Printf("[sbsh] controller stopped\r\n")
	}()

	// block until controller is ready (or ctx cancels)
	if err := ctrl.WaitReady(ctx); err != nil {
		log.Printf("controller not ready: %s", err)
		return fmt.Errorf("%w: %w", ErrWaitOnReady, err)
	}

	select {
	case <-ctx.Done():
		log.Printf("[sbsh-session] context canceled, waiting on sessionCtrl to exit\r\n")
		if err := ctrl.WaitClose(); err != nil {
			return fmt.Errorf("%w: %w", ErrWaitOnClose, err)
		}
		log.Printf("[sbsh-session] context canceled, sessionCtrl exited\r\n")

		return ErrContextDone
	case err := <-errCh:
		log.Printf("[sbsh] controller stopped with error: %v\r\n", err)
		if err != nil && !errors.Is(err, context.Canceled) {
			if err := ctrl.WaitClose(); err != nil {
				return fmt.Errorf("%w: %w", ErrWaitOnClose, err)
			}
			log.Printf("[sbsh-session] context canceled, sessionCtrl exited\r\n")
			return fmt.Errorf("%w: %w", ErrChildExit, err)
		}
	}

	return nil
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
