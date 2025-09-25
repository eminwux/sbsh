/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sbsh/pkg/api"
	"sbsh/pkg/common"
	"sbsh/pkg/errdefs"
	"sbsh/pkg/session"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	sessionID            string
	sessionCmd           string
	ctx                  context.Context
	cancel               context.CancelFunc
	newSessionController = session.NewSessionController
)

// runCmd represents the run command
var RunCmd = &cobra.Command{
	Use:   "run",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {

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

func init() {
	RunCmd.Flags().StringVar(&sessionID, "id", "", "Optional session ID (random if omitted)")
	RunCmd.Flags().StringVar(&sessionCmd, "command", "", "Optional command (default: bash -i)")

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
		ID:          api.ID(sessionID),
		Kind:        api.SessLocal,
		Name:        "default",
		Command:     sessionCmd,
		CommandArgs: cmdArgs,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
		RunPath:     viper.GetString("global.runPath"),
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
		return fmt.Errorf("%w: %w", errdefs.ErrWaitOnReady, err)
	}
	select {
	case <-ctx.Done():
		slog.Debug("[sbsh-session] context canceled, waiting on sessionCtrl to exit\r\n")
		if err := sessionCtrl.WaitClose(); err != nil {
			return fmt.Errorf("%w: %w", errdefs.ErrWaitOnClose, err)
		}
		slog.Debug("[sbsh-session] context canceled, sessionCtrl exited\r\n")

		return errdefs.ErrContextDone
	case err := <-errCh:
		slog.Debug(fmt.Sprintf("[sbsh-sesion] controller stopped with error: %v\r\n", err))
		if err != nil && !errors.Is(err, context.Canceled) {
			if err := sessionCtrl.WaitClose(); err != nil {
				return fmt.Errorf("%w: %w", errdefs.ErrWaitOnClose, err)
			}
			slog.Debug("[sbsh-session] context canceled, sessionCtrl exited\r\n")
			return fmt.Errorf("%w: %w", errdefs.ErrChildExit, err)
		}
	}
	return nil
}
