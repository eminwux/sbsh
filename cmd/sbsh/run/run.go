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
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sbsh/pkg/api"
	"sbsh/pkg/discovery"
	"sbsh/pkg/env"
	"sbsh/pkg/errdefs"
	"sbsh/pkg/profile"
	"sbsh/pkg/session"
)

const (
	Command      string = "run"
	CommandAlias string = "r"
)

var (
	sessionIDInput       string
	sessionNameInput     string
	sessionCmdInput      string
	logFilenameInput     string
	profileNameInput     string
	socketFileInput      string
	ctx                  context.Context
	cancel               context.CancelFunc
	newSessionController = session.NewSessionController
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
		Run: func(cmd *cobra.Command, args []string) {
			sessionSpec, err := profile.BuildSessionSpec(
				viper.GetString(env.RUN_PATH.ViperKey),
				viper.GetString(env.PROFILES_FILE.ViperKey),
				profileNameInput,
				sessionIDInput,
				sessionNameInput,
				sessionCmdInput,
				logFilenameInput,
				socketFileInput,
				os.Environ(),
				ctx,
			)
			if err != nil {
				log.Fatal(err)
			}

			// If a profile name was given, set the SBSH_SES_PROFILE env var
			if profileNameInput != "" {
				env.SES_PROFILE.Set(profileNameInput)
				if env.SES_PROFILE.BindEnv() != nil {
					log.Fatal(err)
				}
				if env.SES_PROFILE.Set(profileNameInput) != nil {
					log.Fatal(err)
				}
			}

			if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
				discovery.PrintSessionSpec(sessionSpec, os.Stdout)
			}
			runSession(sessionSpec)
		},
	}

	setupRunCmdFlags(runCmd)
	return runCmd
}

func setupRunCmdFlags(runCmd *cobra.Command) {
	runCmd.Flags().StringVar(&sessionIDInput, "id", "", "Optional session ID (random if omitted)")
	runCmd.Flags().StringVar(&sessionCmdInput, "command", "", "Optional command (default: /bin/bash)")
	runCmd.Flags().StringVar(&sessionNameInput, "name", "", "Optional name for the session")
	runCmd.Flags().StringVar(&logFilenameInput, "log-filename", "", "Optional filename for the session log")
	runCmd.Flags().StringVar(&profileNameInput, "profile", "", "Optional profile for the session")
	runCmd.Flags().StringVar(&socketFileInput, "socket", "", "Optional socket file for the session")

	if err := viper.BindPFlag("session.logFilename", runCmd.Flags().Lookup("log-filename")); err != nil {
		log.Fatal(err)
	}
}

func runSession(spec *api.SessionSpec) error {
	// Top-level context also reacts to SIGINT/SIGTERM (nice UX)
	ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create a new Controller
	var sessionCtrl api.SessionController

	sessionCtrl = newSessionController(ctx, cancel)

	// Create error channel
	errCh := make(chan error, 1)

	// Run controller
	go func() {
		errCh <- sessionCtrl.Run(spec) // Run should return when ctx is canceled
		slog.Debug("[sbsh] controller stopped\r\n")
	}()

	// block until controller is ready (or ctx cancels)
	if err := sessionCtrl.WaitReady(); err != nil {
		slog.Debug(fmt.Sprintf("controller not ready: %s\r\n", err))
		return fmt.Errorf("%w: %v", errdefs.ErrWaitOnReady, err)
	}
	select {
	case <-ctx.Done():
		slog.Debug("[sbsh-session] context canceled, waiting on sessionCtrl to exit\r\n")
		if err := sessionCtrl.WaitClose(); err != nil {
			return fmt.Errorf("%w: %v", errdefs.ErrWaitOnClose, err)
		}
		slog.Debug("[sbsh-session] context canceled, sessionCtrl exited\r\n")

		return errdefs.ErrContextDone
	case err := <-errCh:
		slog.Debug(fmt.Sprintf("[sbsh] controller stopped with error: %v\r\n", err))
		if err != nil && !errors.Is(err, context.Canceled) {
			if err := sessionCtrl.WaitClose(); err != nil {
				return fmt.Errorf("%w: %v", errdefs.ErrWaitOnClose, err)
			}
			slog.Debug("[sbsh-session] context canceled, sessionCtrl exited\r\n")
			return fmt.Errorf("%w: %v", errdefs.ErrChildExit, err)
		}
	}
	return nil
}
