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
	Command      = "attach"
	CommandAlias = "a"
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
			id := viper.GetString("attach.id")
			name := viper.GetString("attach.name")
			runPath := viper.GetString(env.RUN_PATH.ViperKey)
			socketFile := viper.GetString("attach.socket")

			if id == "" && name == "" {
				return errors.New("either --id or --name must be defined")
			}
			if id != "" && name != "" {
				return errors.New("only one of --id or --name must be defined")
			}

			return run(cmd.Context(), id, name, runPath, socketFile)
		},
	}

	setupAttachCmdFlags(attachCmd)
	return attachCmd
}

func setupAttachCmdFlags(attachCmd *cobra.Command) {
	attachCmd.Flags().String("id", "", "Session ID, cannot be set together with --name")
	attachCmd.Flags().String("name", "", "Optional session name, cannot be set together with --id")
	attachCmd.Flags().String("socket", "", "Optional socket file for the session")
	attachCmd.Flags().String("run-path", "", "Run path directory")

	// Bind flags to viper keys
	_ = viper.BindPFlag("attach.id", attachCmd.Flags().Lookup("id"))
	_ = viper.BindPFlag("attach.name", attachCmd.Flags().Lookup("name"))
	_ = viper.BindPFlag("attach.socket", attachCmd.Flags().Lookup("socket"))
	_ = viper.BindPFlag(env.RUN_PATH.ViperKey, attachCmd.Flags().Lookup("run-path"))
}

func run(parentCtx context.Context, id string, name string, runPath string, socketFileInput string) error {
	// Top-level context also reacts to SIGINT/SIGTERM (nice UX)
	ctx, cancel := signal.NotifyContext(parentCtx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create a new Controller
	supCtrl := supervisor.NewSupervisorController(ctx)

	supervisorID := naming.RandomID()
	supervisorName := naming.RandomName()

	if socketFileInput == "" {
		socketFileInput = filepath.Join(runPath, "socket")
	}
	var spec *api.SupervisorSpec
	if id != "" && name == "" {
		spec = &api.SupervisorSpec{
			Kind:       api.AttachToSession,
			ID:         api.ID(supervisorID),
			Name:       supervisorName,
			RunPath:    runPath,
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
			RunPath:    runPath,
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
		spec.SessionSpec.SocketFile = filepath.Join(runPath, ".sbsh", "sessions", string(spec.SessionSpec.ID), "socket")
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
				slog.Error("attach error", "error", err)
			}

			return fmt.Errorf("%w: %w", errdefs.ErrChildExit, err)
		}
	}
	return nil
}
