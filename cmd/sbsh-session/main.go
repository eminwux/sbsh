/*
Copyright © 2025 Emiliano Spinella (eminwux)
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sbsh/pkg/api"
	"sbsh/pkg/common"
	"sbsh/pkg/session"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	_ "net/http/pprof"
)

var (
	sessionID  string
	sessionCmd string
)

var newSessionController = session.NewSessionController
var ctx context.Context
var cancel context.CancelFunc

func main() {
	Execute()
}

var logLevel string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sbsh-session",
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

func runSession(sessionID string, sessionCmd string, cmdArgs []string) error {
	go http.ListenAndServe("127.0.0.1:6060", nil)
	// Top-level context also reacts to SIGINT/SIGTERM (nice UX)
	ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	runtime.SetBlockProfileRate(1)     // sample ALL blocking events on chans/locks
	runtime.SetMutexProfileFraction(1) // sample ALL mutex contention
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

	// Create a new Controller
	var sessionCtrl api.SessionController

	sessionCtrl = newSessionController(ctx, cancel)

	// Create error channel
	errCh := make(chan error, 1)

	// Run controller
	go func() {
		errCh <- sessionCtrl.Run(&spec) // Run should return when ctx is canceled
		slog.Debug("[sbsh] controller stopped\r\n")
	}()

	// block until controller is ready (or ctx cancels)
	if err := sessionCtrl.WaitReady(); err != nil {
		slog.Debug(fmt.Sprintf("controller not ready: %s\r\n", err))
		return fmt.Errorf("%w: %w", ErrWaitOnReady, err)
	}
	select {
	case <-ctx.Done():
		slog.Debug("[sbsh-session] context canceled, waiting on sessionCtrl to exit\r\n")
		if err := sessionCtrl.WaitClose(); err != nil {
			return fmt.Errorf("%w: %w", ErrWaitOnClose, err)
		}
		slog.Debug("[sbsh-session] context canceled, sessionCtrl exited\r\n")

		return ErrContextDone
	case err := <-errCh:
		slog.Debug(fmt.Sprintf("[sbsh-sesion] controller stopped with error: %v\r\n", err))
		if err != nil && !errors.Is(err, context.Canceled) {
			if err := sessionCtrl.WaitClose(); err != nil {
				return fmt.Errorf("%w: %w", ErrWaitOnClose, err)
			}
			slog.Debug("[sbsh-session] context canceled, sessionCtrl exited\r\n")
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
	rootCmd.Flags().StringVar(&sessionID, "id", "", "Optional session ID (random if omitted)")
	rootCmd.Flags().StringVar(&sessionCmd, "command", "", "Optional command (default: bash -i)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
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
