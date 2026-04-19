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

package stop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"syscall"
	"time"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/sb/get"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/pkg/api"
	rpcterminal "github.com/eminwux/sbsh/pkg/rpcclient/terminal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	defaultStopTimeout = 10 * time.Second
	//nolint:mnd // poll interval while waiting for the process to exit
	stopPollInterval = 100 * time.Millisecond
	//nolint:mnd // short RPC dial deadline (network is a Unix socket)
	stopRPCDialTimeout = 3 * time.Second
)

type stopOpts struct {
	runPath        string
	name           string
	all            bool
	force          bool
	timeout        time.Duration
	killAfter      bool
	ignoreNotFound bool
}

func NewStopTerminalsCmd() *cobra.Command {
	stopTerminalsCmd := &cobra.Command{
		Use:     "terminal [name]",
		Aliases: []string{"terminals", "terms", "term", "t"},
		Short:   "Stop a running terminal",
		Long: `Stop a running terminal by name.

By default, sends a Stop RPC to the terminal controller (graceful). If the RPC
is unreachable, falls back to SIGTERM on the recorded PID. After signalling,
waits up to --timeout for the process to exit. With --force, sends SIGKILL
immediately. With --kill-after, escalates to SIGKILL when the timeout elapses.

Metadata is left in place so 'sb prune terminals' can clean it up.`,
		SilenceUsage:      true,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: get.CompleteTerminals,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errdefs.ErrLoggerNotFound
			}

			opts, err := resolveStopOpts(cmd, args)
			if err != nil {
				return err
			}

			if err := runStopTerminals(cmd.Context(), logger, os.Stdout, os.Stderr, opts); err != nil {
				logger.DebugContext(cmd.Context(), "stop terminals failed", "error", err)
				return err
			}
			return nil
		},
	}

	setupStopTerminalsCmd(stopTerminalsCmd)
	return stopTerminalsCmd
}

func setupStopTerminalsCmd(c *cobra.Command) {
	c.Flags().Bool("all", false, "Stop every active terminal")
	_ = viper.BindPFlag(config.SB_STOP_ALL.ViperKey, c.Flags().Lookup("all"))

	c.Flags().Bool("force", false, "Send SIGKILL immediately instead of a graceful stop")
	_ = viper.BindPFlag(config.SB_STOP_FORCE.ViperKey, c.Flags().Lookup("force"))

	c.Flags().Duration("timeout", defaultStopTimeout, "How long to wait for the terminal to exit")
	_ = viper.BindPFlag(config.SB_STOP_TIMEOUT.ViperKey, c.Flags().Lookup("timeout"))

	c.Flags().Bool("kill-after", false, "Escalate to SIGKILL if the terminal is still alive after --timeout")
	_ = viper.BindPFlag(config.SB_STOP_KILL_AFTER.ViperKey, c.Flags().Lookup("kill-after"))

	c.Flags().Bool("ignore-not-found", false, "Exit 0 if the terminal cannot be found")
	_ = viper.BindPFlag(config.SB_STOP_IGNORE_NOT_FOUND.ViperKey, c.Flags().Lookup("ignore-not-found"))
}

func resolveStopOpts(cmd *cobra.Command, args []string) (stopOpts, error) {
	opts := stopOpts{
		runPath:        viper.GetString(config.SB_ROOT_RUN_PATH.ViperKey),
		all:            viper.GetBool(config.SB_STOP_ALL.ViperKey),
		force:          viper.GetBool(config.SB_STOP_FORCE.ViperKey),
		timeout:        viper.GetDuration(config.SB_STOP_TIMEOUT.ViperKey),
		killAfter:      viper.GetBool(config.SB_STOP_KILL_AFTER.ViperKey),
		ignoreNotFound: viper.GetBool(config.SB_STOP_IGNORE_NOT_FOUND.ViperKey),
	}

	if len(args) == 1 {
		opts.name = args[0]
	}

	if opts.name == "" && !opts.all {
		return opts, errdefs.ErrNoTerminalIdentifier
	}
	if opts.name != "" && opts.all {
		return opts, fmt.Errorf(
			"%w: --all cannot be combined with a positional terminal name",
			errdefs.ErrInvalidFlag,
		)
	}
	if opts.timeout <= 0 {
		return opts, fmt.Errorf("%w: --timeout must be greater than zero", errdefs.ErrInvalidFlag)
	}

	// cmd is only needed to keep a parallel signature with other commands
	// that read flags directly; viper already has everything we need.
	_ = cmd
	return opts, nil
}

func runStopTerminals(
	ctx context.Context,
	logger *slog.Logger,
	stdout, stderr io.Writer,
	opts stopOpts,
) error {
	targets, err := resolveTargets(ctx, logger, opts)
	if err != nil {
		if errors.Is(err, errdefs.ErrTerminalNotFound) && opts.ignoreNotFound {
			fmt.Fprintf(stdout, "terminal %q not found (ignored)\n", opts.name)
			return nil
		}
		return err
	}

	if len(targets) == 0 {
		fmt.Fprintln(stdout, "no active terminals found")
		return nil
	}

	var stopped, alreadyExited, failed int
	for i := range targets {
		doc := &targets[i]
		res := stopOneTerminal(ctx, logger, stdout, stderr, doc, opts)
		switch res {
		case resultStopped:
			stopped++
		case resultAlreadyExited:
			alreadyExited++
		case resultFailed:
			failed++
		}
	}

	if opts.all || len(targets) > 1 {
		fmt.Fprintf(stdout,
			"stopped %d terminal(s), %d already exited, %d error(s)\n",
			stopped, alreadyExited, failed)
	}
	if failed > 0 {
		return fmt.Errorf("%w: %d terminal(s) failed to stop", errdefs.ErrStopTerminal, failed)
	}
	return nil
}

type stopResult int

const (
	resultStopped stopResult = iota
	resultAlreadyExited
	resultFailed
)

func resolveTargets(
	ctx context.Context,
	logger *slog.Logger,
	opts stopOpts,
) ([]api.TerminalDoc, error) {
	if opts.all {
		terminals, err := discovery.ScanTerminals(ctx, logger, opts.runPath)
		if err != nil {
			return nil, err
		}
		discovery.ReconcileTerminals(ctx, logger, opts.runPath, terminals)
		active := make([]api.TerminalDoc, 0, len(terminals))
		for _, t := range terminals {
			if t.Status.State != api.Exited {
				active = append(active, t)
			}
		}
		return active, nil
	}

	doc, err := discovery.FindTerminalByName(ctx, logger, opts.runPath, opts.name)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errdefs.ErrTerminalNotFound, err)
	}
	if doc == nil {
		return nil, fmt.Errorf("%w: terminal %q", errdefs.ErrTerminalNotFound, opts.name)
	}
	return []api.TerminalDoc{*doc}, nil
}

func stopOneTerminal(
	ctx context.Context,
	logger *slog.Logger,
	stdout, stderr io.Writer,
	doc *api.TerminalDoc,
	opts stopOpts,
) stopResult {
	name := doc.Spec.Name
	if name == "" {
		name = string(doc.Spec.ID)
	}

	if len(doc.Status.Attachers) > 0 {
		fmt.Fprintf(stderr,
			"warning: terminal %q currently has %d attacher(s); they will be disconnected\n",
			name, len(doc.Status.Attachers))
	}

	// Refresh liveness: ReconcileTerminals mutates state, but when called
	// through FindTerminalByName we only see the on-disk snapshot. Check PID
	// directly.
	if doc.Status.State == api.Exited || !processAlive(doc.Status.Pid) {
		fmt.Fprintf(stdout, "terminal %q already exited\n", name)
		return resultAlreadyExited
	}

	if opts.force {
		if err := sendSignal(doc.Status.Pid, syscall.SIGKILL); err != nil {
			fmt.Fprintf(stderr, "failed to SIGKILL terminal %q: %v\n", name, err)
			return resultFailed
		}
		if !waitForExit(ctx, doc.Status.Pid, opts.timeout) {
			fmt.Fprintf(stderr, "terminal %q did not exit after SIGKILL within %s\n", name, opts.timeout)
			return resultFailed
		}
		fmt.Fprintf(stdout, "terminal %q stopped (SIGKILL)\n", name)
		return resultStopped
	}

	// Graceful path: try RPC first, fall back to SIGTERM.
	if err := stopViaRPC(ctx, logger, doc); err != nil {
		logger.DebugContext(ctx, "Stop RPC failed, falling back to SIGTERM",
			"name", name, "error", err)
		if errSig := sendSignal(doc.Status.Pid, syscall.SIGTERM); errSig != nil {
			fmt.Fprintf(stderr, "failed to signal terminal %q: %v\n", name, errSig)
			return resultFailed
		}
	}

	if waitForExit(ctx, doc.Status.Pid, opts.timeout) {
		fmt.Fprintf(stdout, "terminal %q stopped\n", name)
		return resultStopped
	}

	if !opts.killAfter {
		fmt.Fprintf(stderr, "terminal %q did not exit within %s (use --kill-after to escalate)\n",
			name, opts.timeout)
		return resultFailed
	}

	fmt.Fprintf(stdout, "terminal %q did not exit within %s; escalating to SIGKILL\n", name, opts.timeout)
	if err := sendSignal(doc.Status.Pid, syscall.SIGKILL); err != nil {
		fmt.Fprintf(stderr, "failed to SIGKILL terminal %q: %v\n", name, err)
		return resultFailed
	}
	if !waitForExit(ctx, doc.Status.Pid, opts.timeout) {
		fmt.Fprintf(stderr, "terminal %q did not exit after SIGKILL within %s\n", name, opts.timeout)
		return resultFailed
	}
	fmt.Fprintf(stdout, "terminal %q stopped (SIGKILL)\n", name)
	return resultStopped
}

func stopViaRPC(
	ctx context.Context,
	logger *slog.Logger,
	doc *api.TerminalDoc,
) error {
	socket := doc.Status.SocketFile
	if socket == "" {
		return errors.New("terminal metadata has no control socket recorded")
	}

	dialCtx, cancel := context.WithTimeout(ctx, stopRPCDialTimeout)
	defer cancel()

	rc := rpcterminal.NewUnix(socket, logger, rpcterminal.WithDialTimeout(stopRPCDialTimeout))
	defer func() { _ = rc.Close() }()

	return rc.Stop(dialCtx, &api.StopArgs{Reason: "sb stop"})
}

func sendSignal(pid int, sig syscall.Signal) error {
	if pid <= 0 {
		return fmt.Errorf("%w: invalid pid %d", errdefs.ErrSignalProcess, pid)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("%w: %w", errdefs.ErrSignalProcess, err)
	}
	if err := proc.Signal(sig); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			// Process is already gone — not an error for our purposes.
			return nil
		}
		return fmt.Errorf("%w: %w", errdefs.ErrSignalProcess, err)
	}
	return nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	// EPERM: process exists but we lack permission. Treat as alive.
	return errors.Is(err, syscall.EPERM)
}

func waitForExit(ctx context.Context, pid int, timeout time.Duration) bool {
	if pid <= 0 {
		return true
	}
	deadline := time.Now().Add(timeout)
	for {
		if !processAlive(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return !processAlive(pid)
		case <-time.After(stopPollInterval):
		}
	}
}
