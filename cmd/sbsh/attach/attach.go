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

package attach

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/eminwux/sbsh/internal/env"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/internal/supervisor"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	Command      string = "attach"
	CommandAlias string = "a"
)

var (
	sessionID               string
	sessionName             string
	socketFileInput         string
	runPathInput            string
	ctx                     context.Context
	cancel                  context.CancelFunc
	newSupervisorController = supervisor.NewSupervisorController
)

func NewAttachCmd() *cobra.Command {
	// runCmd represents the run command
	attachCmd := &cobra.Command{
		Use:     Command,
		Aliases: []string{CommandAlias},
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

	setupAttachCmdFlags(attachCmd)
	return attachCmd
}

func setupAttachCmdFlags(attachCmd *cobra.Command) {
	attachCmd.Flags().StringVar(&sessionID, "id", "", "Session ID, cannot be set together with --name")
	attachCmd.Flags().StringVar(&sessionName, "name", "", "Optional session name, cannot be set together with --id")
	attachCmd.Flags().StringVar(&socketFileInput, "socket", "", "Optional socket file for the session")
	attachCmd.Flags().StringVar(&runPathInput, "run-path", "", "Optional socket file for the session")
}

func run(id string, name string) error {
	// Top-level context also reacts to SIGINT/SIGTERM (nice UX)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create a new Controller
	supCtrl := newSupervisorController(ctx)

	supervisorID := naming.RandomID()
	supervisorName := naming.RandomName()

	if socketFileInput == "" {
		socketFileInput = filepath.Join(
			runPathInput,
			"socket",
		)
	}
	var spec *api.SupervisorSpec
	if id != "" && name == "" {
		spec = &api.SupervisorSpec{
			Kind:       api.AttachToSession,
			ID:         api.ID(supervisorID),
			Name:       supervisorName,
			RunPath:    viper.GetString(env.RUN_PATH.ViperKey),
			SockerCtrl: socketFileInput,
			AttachID:   api.ID(id),
			SessionSpec: &api.SessionSpec{
				ID: api.ID(id),
			},
		}
		slog.Debug("Attach spec created",
			"Kind", spec.Kind,
			"ID", spec.ID,
			"Name", spec.Name,
			"RunPath", spec.RunPath,
			"AttachID", spec.AttachID,
			"SessionSpec.ID", spec.SessionSpec.ID,
		)
	}

	if id == "" && name != "" {
		spec = &api.SupervisorSpec{
			Kind:       api.AttachToSession,
			ID:         api.ID(naming.RandomID()),
			Name:       naming.RandomName(),
			LogDir:     "/tmp/sbsh-logs/s0",
			RunPath:    viper.GetString(env.RUN_PATH.ViperKey),
			SockerCtrl: socketFileInput,
			SessionSpec: &api.SessionSpec{
				Name: name,
			},
		}
		slog.Debug("Attach spec created",
			"Kind", spec.Kind,
			"ID", spec.ID,
			"Name", spec.Name,
			"LogDir", spec.LogDir,
			"RunPath", spec.RunPath,
			"SessionMetadata.Spec.Name", spec.SessionSpec.Name,
		)
	}

	if socketFileInput != "" {
		spec.SessionSpec.SocketFile = socketFileInput
	} else {
		spec.SessionSpec.SocketFile = filepath.Join(viper.GetString("global.runPath"), ".sbsh", "sessions", string(spec.SessionSpec.ID), "socket")
	}

	// Run controller
	errCh := make(chan error, 1)
	go func() {
		errCh <- supCtrl.Run(spec) // Run should return when ctx is canceled
		close(errCh)
		slog.Debug("[sbsh] controller stopped")
	}()

	// block until controller is ready (or ctx cancels)
	if err := supCtrl.WaitReady(); err != nil {
		slog.Debug(fmt.Sprintf("controller not ready: %s\r\n", err))
		return fmt.Errorf("%w: %w", errdefs.ErrWaitOnReady, err)
	}
	select {
	case <-ctx.Done():
		slog.Debug("[sbsh] context canceled, waiting on sessionCtrl to exit\r\n")
		if err := supCtrl.WaitClose(); err != nil {
			return fmt.Errorf("%w: %w", errdefs.ErrWaitOnClose, err)
		}
		slog.Debug("[sbsh] context canceled, sessionCtrl exited\r\n")

		return errdefs.ErrContextDone
	case err := <-errCh:
		slog.Debug(fmt.Sprintf("[sbsh] controller stopped with error: %v\r\n", err))
		if err != nil && !errors.Is(err, context.Canceled) {
			if err := supCtrl.WaitClose(); err != nil {
				return fmt.Errorf("%w: %w", errdefs.ErrWaitOnClose, err)
			}
			slog.Debug("[sbsh] context canceled, sessionCtrl exited\r\n")

			if errors.Is(err, errdefs.ErrAttach) {
				fmt.Printf("%v\n", err)
			}

			return fmt.Errorf("%w: %w", errdefs.ErrChildExit, err)
		}
	}
	return nil
}
