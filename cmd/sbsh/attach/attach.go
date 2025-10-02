package attach

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sbsh/pkg/api"
	"sbsh/pkg/errdefs"
	"sbsh/pkg/naming"
	"sbsh/pkg/supervisor"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	sessionID               string
	sessionName             string
	ctx                     context.Context
	cancel                  context.CancelFunc
	newSupervisorController = supervisor.NewSupervisorController
)

// runCmd represents the run command
var AttachCmd = &cobra.Command{
	Use:     "attach",
	Aliases: []string{"a"},
	Short:   "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	RunE: func(cmd *cobra.Command, args []string) error {

		if sessionID == "" && sessionName == "" {
			fmt.Println("Either --id or --name must be defined")
			os.Exit(1)
		}

		if sessionID != "" && sessionName != "" {
			fmt.Println("Only --id or --name must be defined")
			os.Exit(1)
		}

		if sessionID != "" {
			run(sessionID, "")
		} else {
			run("", sessionName)
		}

		return nil

	},
}

func init() {
	AttachCmd.Flags().StringVar(&sessionID, "id", "", "Session ID, cannot be set together with --name")
	AttachCmd.Flags().StringVar(&sessionName, "name", "", "Optional session name, cannot be set together with --id")
}

func run(id string, name string) error {
	// Top-level context also reacts to SIGINT/SIGTERM (nice UX)
	ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create a new Controller
	var supCtrl api.SupervisorController

	supCtrl = newSupervisorController(ctx)

	var spec *api.SupervisorSpec
	if id != "" && name == "" {
		spec = &api.SupervisorSpec{
			Kind:     api.AttachToSession,
			ID:       api.ID(naming.RandomID()),
			Name:     naming.RandomSessionName(),
			LogDir:   "/tmp/sbsh-logs/s0",
			RunPath:  viper.GetString("global.runPath"),
			AttachID: api.ID(id),
		}
	}

	if id == "" && name != "" {
		spec = &api.SupervisorSpec{
			Kind:       api.AttachToSession,
			ID:         api.ID(naming.RandomID()),
			Name:       naming.RandomSessionName(),
			LogDir:     "/tmp/sbsh-logs/s0",
			RunPath:    viper.GetString("global.runPath"),
			AttachName: name,
		}
	}

	// Create error channel
	errCh := make(chan error, 1)

	// Run controller
	go func() {
		errCh <- supCtrl.Run(spec) // Run should return when ctx is canceled
		slog.Debug("[sbsh] controller stopped")
	}()

	// block until controller is ready (or ctx cancels)
	if err := supCtrl.WaitReady(); err != nil {
		slog.Debug(fmt.Sprintf("controller not ready: %s\r\n", err))
		return fmt.Errorf("%w: %v", errdefs.ErrWaitOnReady, err)
	}
	select {
	case <-ctx.Done():
		slog.Debug("[sbsh] context canceled, waiting on sessionCtrl to exit\r\n")
		if err := supCtrl.WaitClose(); err != nil {
			return fmt.Errorf("%w: %v", errdefs.ErrWaitOnClose, err)
		}
		slog.Debug("[sbsh] context canceled, sessionCtrl exited\r\n")

		return errdefs.ErrContextDone
	case err := <-errCh:
		slog.Debug(fmt.Sprintf("[sbsh] controller stopped with error: %v\r\n", err))
		if err != nil && !errors.Is(err, context.Canceled) {
			if err := supCtrl.WaitClose(); err != nil {
				return fmt.Errorf("%w: %v", errdefs.ErrWaitOnClose, err)
			}
			slog.Debug("[sbsh] context canceled, sessionCtrl exited\r\n")
			return fmt.Errorf("%w: %v", errdefs.ErrChildExit, err)
		}
	}
	return nil
}
