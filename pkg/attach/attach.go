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
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/eminwux/sbsh/internal/client"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/pkg/api"
)

// Options configures a single Run invocation.
type Options struct {
	// SocketPath is the absolute path to the target terminal's control
	// socket (the same path "sb attach --socket" accepts). Required.
	SocketPath string

	// Stdin is the user-facing input handle. It must be a TTY-backed
	// *os.File: the attach loop puts it in raw mode and reads the
	// initial / SIGWINCH-triggered window size from it. Defaults to
	// os.Stdin if nil.
	Stdin *os.File

	// Stdout is where the remote terminal output is written. Defaults
	// to os.Stdout if nil.
	Stdout *os.File

	// Stderr receives any out-of-band diagnostics emitted by the
	// embedded client (currently nothing under the happy path; reserved
	// for future use). Defaults to os.Stderr if nil.
	Stderr *os.File

	// DisableDetachKeystroke turns off the in-band ^]^] detach
	// shortcut. When false (default), the loop scans Stdin for the
	// escape sequence and triggers a clean detach when it fires.
	DisableDetachKeystroke bool

	// FullCapture, when true, replays the entire raw capture buffer on
	// attach instead of the default bounded repaint of the current screen.
	// It flows through ClientSpec.FullCapture to the terminal server.
	FullCapture bool

	// Logger is a structured logger for diagnostics. Defaults to a
	// discard logger when nil so embedders aren't forced to wire one
	// in.
	Logger *slog.Logger
}

// Run connects to opts.SocketPath, runs the interactive attach loop
// against the supplied stdio handles, and returns when the session
// ends. The loop terminates cleanly on:
//   - context cancellation (returns ctx.Err wrapped in
//     errdefs.ErrContextDone),
//   - the remote terminal closing the connection,
//   - the embedded client's detach keystroke firing (when enabled),
//   - any unrecoverable error from the underlying controller (returned
//     directly so callers can errors.Is against errdefs.* sentinels).
//
// Run is safe to call multiple times sequentially from the same
// process; concurrent calls each get their own private control socket
// under os.TempDir().
func Run(ctx context.Context, opts Options) error {
	if opts.SocketPath == "" {
		return ErrSocketPathRequired
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	stdin := opts.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	// Each attach gets its own ephemeral run dir for the embedded
	// client's RPC ctrl socket. The socket file itself is removed by
	// the runner on shutdown; we mop up the surrounding tmp dir.
	runDir, err := os.MkdirTemp("", "sbsh-attach-*")
	if err != nil {
		return fmt.Errorf("pkg/attach: create temp run dir: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(runDir); rmErr != nil {
			logger.WarnContext(ctx, "pkg/attach: failed to remove temp run dir", "dir", runDir, "error", rmErr)
		}
	}()

	clientID := naming.RandomID()
	clientCtrlSocket := filepath.Join(runDir, "client.sock")

	doc := &api.ClientDoc{
		APIVersion: api.APIVersionV1Beta1,
		Kind:       api.KindClient,
		Metadata: api.ClientMetadata{
			Name:        naming.RandomName(),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: api.ClientSpec{
			ID:              api.ID(clientID),
			RunPath:         runDir,
			SockerCtrl:      clientCtrlSocket,
			ClientMode:      api.AttachToTerminal,
			DetachKeystroke: !opts.DisableDetachKeystroke,
			FullCapture:     opts.FullCapture,
			TerminalSpec: &api.TerminalSpec{
				SocketFile: opts.SocketPath,
			},
		},
	}

	ctrl := client.NewClientControllerWithIO(ctx, logger, stdin, stdout, stderr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- ctrl.Run(doc)
		close(errCh)
	}()

	if waitErr := ctrl.WaitReady(); waitErr != nil {
		// WaitReady only returns non-nil when ctx fires before the
		// controller signals readiness — i.e. ctx-cancel during setup.
		// Actively close so the goroutine can exit even if a setup step
		// is blocked on something that doesn't observe ctx; without
		// this, <-errCh would have to trust every downstream RPC to
		// honour ctx-cancel. Return the same ErrContextDone shape the
		// post-setup path uses so callers branching on errors.Is see
		// one sentinel for any ctx-cancel.
		_ = ctrl.Close(waitErr)
		<-errCh
		return fmt.Errorf("%w: %w", errdefs.ErrContextDone, waitErr)
	}

	select {
	case <-ctx.Done():
		// Force shutdown for the same reason as the WaitReady path, and
		// so WaitClose (which blocks on closedCh, which only Close
		// closes) doesn't depend on the controller reaching its own
		// ctx.Done observer before exiting.
		ctxErr := ctx.Err()
		_ = ctrl.Close(ctxErr)
		_ = ctrl.WaitClose()
		<-errCh
		return fmt.Errorf("%w: %w", errdefs.ErrContextDone, ctxErr)

	case ctrlErr := <-errCh:
		if ctrlErr == nil {
			return nil
		}
		// Drain WaitClose to release internal resources, then map the
		// controller-level session-end sentinel to the matching public
		// sentinel so embedders can branch with errors.Is. ctx-cancel
		// surfaces here as errdefs.ErrContextDone chained with ctx.Err
		// — pass it through unchanged so the two arms of this select
		// return the same shape regardless of which one wins the race.
		_ = ctrl.WaitClose()
		return classifySessionEnd(ctrlErr)
	}
}

// classifySessionEnd maps a controller-level error from Run to the
// matching public sentinel. errdefs.ErrClientDetached → ErrDetached;
// errdefs.ErrPeerClosed → ErrPeerClosed; anything else is returned
// unchanged so existing errdefs.* matches keep working.
func classifySessionEnd(err error) error {
	switch {
	case errors.Is(err, errdefs.ErrClientDetached):
		return fmt.Errorf("%w: %w", ErrDetached, err)
	case errors.Is(err, errdefs.ErrPeerClosed):
		return fmt.Errorf("%w: %w", ErrPeerClosed, err)
	default:
		return err
	}
}
