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

package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/env"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/profile"
	"github.com/eminwux/sbsh/internal/session"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	Command      string = "run"
	CommandAlias string = "r"
)

func NewRunCmd() *cobra.Command {
	// runCmd represents the run command.
	runCmd := &cobra.Command{
		Use:     Command,
		Aliases: []string{CommandAlias},
		Short:   "Run a new sbsh session",
		Long: `Run a new sbsh session.
The session can be customized via command-line options or by specifying a profile.
If no profile is specified, a default profile is used with the provided command or /bin/bash.

Examples:
  sbsh run --name mysession --command "/bin/zsh"
  sbsh run --profile devprofile
  sbsh run --profile devprofile --name customname --command "/usr/bin/fish"

If no session name is provided, a random name will be generated.
If no command is provided, /bin/bash will be used by default.
If no log filename is provided, a default path under the run directory will be used.
`,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			// Print the values of viper parameters after binding

			// Print default values for debugging
			// fmt.Printf("Default flag values:\n")
			// fmt.Printf("  id: %v\n", runCmd.Flags().Lookup("id").DefValue)
			// fmt.Printf("  command: %v\n", runCmd.Flags().Lookup("command").DefValue)
			// fmt.Printf("  name: %v\n", runCmd.Flags().Lookup("name").DefValue)
			// fmt.Printf("  log-filename: %v\n", runCmd.Flags().Lookup("log-filename").DefValue)
			// fmt.Printf("  profile: %v\n", runCmd.Flags().Lookup("profile").DefValue)
			// fmt.Printf("  socket: %v\n", runCmd.Flags().Lookup("socket").DefValue)

			// fmt.Printf("session.id: %v\n", viper.GetString("session.id"))
			// fmt.Printf("session.command: %v\n", viper.GetString("session.command"))
			// fmt.Printf("session.name: %v\n", viper.GetString("session.name"))
			// fmt.Printf("session.logFilename: %v\n", viper.GetString("session.logFilename"))
			// fmt.Printf("session.profile: %v\n", viper.GetString("session.profile"))
			// fmt.Printf("session.socket: %v\n", viper.GetString("session.socket"))

			sessionSpec, err := profile.BuildSessionSpec(
				viper.GetString(env.RUN_PATH.ViperKey),
				viper.GetString(env.PROFILES_FILE.ViperKey),
				viper.GetString("run.session.profile"),
				viper.GetString("run.session.id"),
				viper.GetString("run.session.name"),
				viper.GetString("run.session.command"),
				viper.GetString("run.session.logFilename"),
				viper.GetString("run.session.socket"),
				os.Environ(),
				context.Background(),
			)
			if err != nil {
				return err
			}

			// If a profile name was given, set the SBSH_SES_PROFILE env var
			profileNameInput := viper.GetString("session.profile")
			if profileNameInput != "" {
				if err := env.SES_PROFILE.Set(profileNameInput); err != nil {
					return err
				}
				if err := env.SES_PROFILE.BindEnv(); err != nil {
					return err
				}
			}

			if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
				discovery.PrintSessionSpec(sessionSpec, os.Stdout)
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			ctrl := session.NewSessionController(ctx)

			err = runSession(ctx, cancel, ctrl, sessionSpec)
			if err != nil {
				return err
			}
			return nil
		},
	}

	setupRunCmdFlags(runCmd)
	return runCmd
}

func setupRunCmdFlags(runCmd *cobra.Command) {
	runCmd.Flags().String("id", "", "Optional session ID (random if omitted)")
	runCmd.Flags().String("command", "", "Optional command (default: /bin/bash)")
	runCmd.Flags().String("name", "", "Optional name for the session")
	runCmd.Flags().String("log-filename", "", "Optional filename for the session log")
	runCmd.Flags().String("profile", "", "Optional profile for the session")
	runCmd.Flags().String("socket", "", "Optional socket file for the session")

	_ = viper.BindPFlag("run.session.id", runCmd.Flags().Lookup("id"))
	_ = viper.BindPFlag("run.session.command", runCmd.Flags().Lookup("command"))
	_ = viper.BindPFlag("run.session.name", runCmd.Flags().Lookup("name"))
	_ = viper.BindPFlag("run.session.logFilename", runCmd.Flags().Lookup("log-filename"))
	_ = viper.BindPFlag("run.session.profile", runCmd.Flags().Lookup("profile"))
	_ = viper.BindPFlag("run.session.socket", runCmd.Flags().Lookup("socket"))
}

func runSession(
	ctx context.Context,
	cancel context.CancelFunc,
	ctrl api.SessionController,
	spec *api.SessionSpec,
) error {
	// Top-level context also reacts to SIGINT/SIGTERM (nice UX)
	defer cancel()

	// Create error channel
	errCh := make(chan error, 1)

	// Run controller
	go func() {
		errCh <- ctrl.Run(spec) // Run should return when ctx is canceled
		close(errCh)
		slog.Debug("[sbsh] controller stopped\r\n")
	}()

	// block until controller is ready (or ctx cancels)
	if err := ctrl.WaitReady(); err != nil {
		slog.Debug(fmt.Sprintf("controller not ready: %s\r\n", err))
		return fmt.Errorf("%w: %w", errdefs.ErrWaitOnReady, err)
	}
	select {
	case <-ctx.Done():
		var err error
		slog.Debug("[sbsh-session] context canceled, waiting on sessionCtrl to exit\r\n")
		if errC := ctrl.WaitClose(); errC != nil {
			err = fmt.Errorf("%w %w: %w", err, errdefs.ErrWaitOnClose, errC)
		}
		slog.Debug("[sbsh-session] context canceled, sessionCtrl exited\r\n")

		return fmt.Errorf("%w: %w", errdefs.ErrContextDone, err)

	case err := <-errCh:
		slog.Debug(fmt.Sprintf("[sbsh] controller stopped with error: %v\r\n", err))
		if err != nil && !errors.Is(err, context.Canceled) {
			err = fmt.Errorf("%w: %w", errdefs.ErrChildExit, err)
			if errC := ctrl.WaitClose(); errC != nil {
				err = fmt.Errorf("%w: %w: %w", err, errdefs.ErrWaitOnClose, errC)
			}
			slog.Debug("[sbsh-session] context canceled, sessionCtrl exited\r\n")

			return err
		}
	}
	return nil
}
