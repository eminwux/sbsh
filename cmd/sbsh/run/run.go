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
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sbsh/pkg/api"
	"sbsh/pkg/discovery"
	"sbsh/pkg/env"
	"sbsh/pkg/errdefs"
	"sbsh/pkg/naming"
	"sbsh/pkg/profile"
	"sbsh/pkg/session"
)

var (
	sessionIDInput       string
	sessionNameInput     string
	sessionCmdInput      string
	logFilenameInput     string
	profileNameInput     string
	ctx                  context.Context
	cancel               context.CancelFunc
	newSessionController = session.NewSessionController
)

// RunCmd represents the run command.
var RunCmd = &cobra.Command{
	Use:   "run",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		if sessionIDInput == "" {
			sessionIDInput = naming.RandomID()
		}

		if sessionNameInput == "" {
			sessionNameInput = naming.RandomSessionName()
		}

		if sessionCmdInput == "" {
			sessionCmdInput = "/bin/bash"
		}

		if logFilenameInput == "" {
			logFilenameInput = filepath.Join(
				viper.GetString(env.RUN_PATH.ViperKey),
				"sessions",
				string(sessionIDInput),
				"session.log",
			)
		}

		var sessionSpec *api.SessionSpec
		if profileNameInput == "" {
			// Split into args for exec
			cmdArgs := []string{}

			// Define a new Session
			sessionSpec = &api.SessionSpec{
				ID:          api.ID(sessionIDInput),
				Kind:        api.SessionLocal,
				Name:        sessionNameInput,
				Command:     sessionCmdInput,
				CommandArgs: cmdArgs,
				Env:         os.Environ(),
				RunPath:     viper.GetString(env.RUN_PATH.ViperKey),
				LogFilename: logFilenameInput,
			}
		} else {
			profileSpec, err := discovery.FindProfileByName(ctx, viper.GetString(env.PROFILES_FILE.ViperKey), profileNameInput)
			if err != nil {
				log.Fatal(err)
			}
			sessionSpec, err = profile.CreateSessionFromProfile(profileSpec)
			sessionSpec.ID = api.ID(sessionIDInput)
			sessionSpec.RunPath = viper.GetString(env.RUN_PATH.ViperKey)
			sessionSpec.LogFilename = logFilenameInput
			sessionSpec.Env = append(sessionSpec.Env, os.Environ()...)

			if err != nil {
				log.Fatal(err)
			}
		}
		discovery.PrintSessionSpec(sessionSpec, os.Stdout)
		runSession(sessionSpec)
	},
}

func init() {
	RunCmd.Flags().StringVar(&sessionIDInput, "id", "", "Optional session ID (random if omitted)")
	RunCmd.Flags().StringVar(&sessionCmdInput, "command", "", "Optional command (default: /bin/bash)")
	RunCmd.Flags().StringVar(&sessionNameInput, "name", "", "Optional name for the session")
	RunCmd.Flags().StringVar(&logFilenameInput, "log-filename", "", "Optional filename for the session log")
	RunCmd.Flags().StringVar(&profileNameInput, "profile", "", "Optional profile for the session")

	if err := viper.BindPFlag("session.logFilename", RunCmd.Flags().Lookup("log-filename")); err != nil {
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
