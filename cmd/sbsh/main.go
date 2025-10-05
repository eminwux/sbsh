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

package main

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
	"sbsh/cmd/sbsh/attach"
	"sbsh/cmd/sbsh/run"
	"sbsh/pkg/api"
	"sbsh/pkg/common"
	"sbsh/pkg/env"
	"sbsh/pkg/naming"
	"sbsh/pkg/supervisor"
)

var (
	newSupervisorController = supervisor.NewSupervisorController
	ctx                     context.Context
	cancel                  context.CancelFunc
)

var (
	logLevel string
	runPath  string
	cfgFile  string
)

func main() {
	Execute()
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sbsh",
	Short: "A brief description of your application",
	Long:  `A longer description ...`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		err := LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Config error:", err)
			os.Exit(1)
		}

		h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: common.ParseLevel(viper.GetString("global.logLevel")),
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
		runSupervisor()
	},
}

func runSupervisor() error {
	ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	supervisorID := naming.RandomID()
	// Define a new Supervisor
	spec := api.SupervisorSpec{
		Kind:    api.RunNewSession,
		ID:      api.ID(supervisorID),
		Name:    naming.RandomSessionName(),
		Env:     os.Environ(),
		LogDir:  "/tmp/sbsh-logs/s0",
		RunPath: viper.GetString("global.runPath"),
	}
	// Create a new Controller
	var ctrl api.SupervisorController

	ctrl = newSupervisorController(ctx)

	// Create error channel
	errCh := make(chan error, 1)

	// Run controller
	go func() {
		errCh <- ctrl.Run(&spec) // Run should return when ctx is canceled
		slog.Debug("[sbsh] controller stopped\r\n")
	}()

	// block until controller is ready (or ctx cancels)
	if err := ctrl.WaitReady(); err != nil {
		slog.Debug(fmt.Sprintf("controller not ready: %s", err))
		return fmt.Errorf("%w: %v", ErrWaitOnReady, err)
	}

	select {
	case <-ctx.Done():
		var err error
		slog.Debug("[sbsh] context canceled, waiting on sessionCtrl to exit\r\n")
		if e := ctrl.WaitClose(); e != nil {
			err = fmt.Errorf("%w: %v", ErrWaitOnClose, e)
		}
		slog.Debug("[sbsh] context canceled, sessionCtrl exited\r\n")
		return fmt.Errorf("%w: %v", ErrContextDone, err)

	case err := <-errCh:
		slog.Debug(fmt.Sprintf("[sbsh] controller stopped with error: %v\r\n", err))
		if err != nil && !errors.Is(err, context.Canceled) {
			err = fmt.Errorf("%w: %v", ErrChildExit, err)
			if err := ctrl.WaitClose(); err != nil {
				err = fmt.Errorf("%w: %v", ErrWaitOnClose, err)
			}
			slog.Debug("[sbsh-session] context canceled, sessionCtrl exited\r\n")
			return err
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
	// go http.ListenAndServe("127.0.0.1:6060", nil)
	// runtime.SetBlockProfileRate(1)     // sample ALL blocking events on chans/locks
	// runtime.SetMutexProfileFraction(1) // sample ALL mutex contention
	rootCmd.AddCommand(run.RunCmd)
	rootCmd.AddCommand(attach.AttachCmd)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.sbsh/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&runPath, "run-path", "$HOME/.sbsh/run", "Log level (debug, info, warn, error)")

	// Bind flag to Viper
	if err := viper.BindPFlag("global.config", rootCmd.PersistentFlags().Lookup("config")); err != nil {
		log.Fatal(err)
	}
	if err := viper.BindPFlag("global.logLevel", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		log.Fatal(err)
	}
	if err := viper.BindPFlag("global.runPath", rootCmd.PersistentFlags().Lookup("run-path")); err != nil {
		log.Fatal(err)
	}
}

// LoadConfig loads config.yaml from the given path or HOME/.sbsh
func LoadConfig() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("err: %v", err)
	}
	viper.AddConfigPath(filepath.Join(home, ".sbsh"))

	_ = env.RUN_PATH.BindEnv()
	_ = env.LOG_LEVEL.BindEnv()

	env.RUN_PATH.SetDefault(filepath.Join(home, ".sbsh", "run"))
	env.LOG_LEVEL.SetDefault("info")

	if err := viper.ReadInConfig(); err != nil {
		// File not found is OK if ENV is set
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		} else {
			return err // Config file was found but another error was produced
		}
	}

	return nil
}
