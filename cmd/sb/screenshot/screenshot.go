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

package screenshot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

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

type screenshotOpts struct {
	runPath string
	name    string
	plain   bool
}

func NewScreenshotCmd() *cobra.Command {
	screenshotCmd := &cobra.Command{
		Use:   "screenshot <name>",
		Short: "Print a terminal's current rendered screen",
		Long: `Print the current rendered screen of a terminal.

sbsh decodes the live PTY output into a screen model (the same grid an
attached client would see) and prints it. By default the output includes
SGR color sequences so colors are preserved; use --plain for the bare text
grid with no escape sequences.`,
		SilenceUsage:      true,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: get.CompleteTerminals,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errdefs.ErrLoggerNotFound
			}
			plain, _ := cmd.Flags().GetBool("plain")
			opts := screenshotOpts{
				runPath: viper.GetString(config.SB_ROOT_RUN_PATH.ViperKey),
				name:    args[0],
				plain:   plain,
			}
			if opts.name == "" {
				return errdefs.ErrNoTerminalIdentifier
			}
			return runScreenshot(cmd.Context(), logger, os.Stdout, opts)
		},
	}

	screenshotCmd.Flags().Bool("plain", false, "Print the bare text grid without SGR color escape sequences")
	return screenshotCmd
}

func runScreenshot(
	ctx context.Context,
	logger *slog.Logger,
	stdout io.Writer,
	opts screenshotOpts,
) error {
	doc, err := discovery.FindTerminalByName(ctx, logger, opts.runPath, opts.name)
	if err != nil {
		return fmt.Errorf("%w: %w", errdefs.ErrTerminalNotFound, err)
	}
	if doc == nil {
		return fmt.Errorf("%w: terminal %q", errdefs.ErrTerminalNotFound, opts.name)
	}

	socket := doc.Status.SocketFile
	if socket == "" {
		return errors.New("terminal metadata has no control socket recorded")
	}

	rc := rpcterminal.NewUnix(socket, logger)
	defer func() { _ = rc.Close() }()

	var result api.ScreenshotResult
	if scErr := rc.Screenshot(ctx, &api.ScreenshotArgs{}, &result); scErr != nil {
		return fmt.Errorf("screenshot: %w", scErr)
	}

	out := result.ANSI
	if opts.plain {
		out = result.Text
	}
	_, err = io.WriteString(stdout, out)
	return err
}
