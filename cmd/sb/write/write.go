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

package write

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

type writeOpts struct {
	runPath string
	name    string
	raw     bool
	payload []byte
}

func NewWriteCmd() *cobra.Command {
	writeCmd := &cobra.Command{
		Use:   "write <name> [text|-]",
		Short: "Write bytes to a terminal's PTY input",
		Long: `Write bytes to a running terminal's PTY input.

By default, caret notation is expanded (^C, ^D, ^[, ^M, etc.) along with
\xHH hex escapes. Shell-style escapes like \n are NOT interpreted; use
^M for a carriage return or --raw to send bytes verbatim.

If the text argument is '-', stdin is read until EOF and sent as-is (caret
notation still applies unless --raw is set). No trailing newline is added;
submit with an explicit ^M.

Examples:
  sb write mybox "echo hello^M"
  sb write mybox --raw "$(printf '\x03')"
  cat script.sh | sb write mybox -`,
		SilenceUsage:      true,
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: get.CompleteTerminals,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errdefs.ErrLoggerNotFound
			}

			opts, err := resolveWriteOpts(args)
			if err != nil {
				return err
			}

			return runWrite(cmd.Context(), logger, opts)
		},
	}

	setupWriteFlags(writeCmd)
	return writeCmd
}

func setupWriteFlags(c *cobra.Command) {
	c.Flags().Bool("raw", false, "Send bytes verbatim; disable caret and \\x escape interpretation")
	_ = viper.BindPFlag(config.SB_WRITE_RAW.ViperKey, c.Flags().Lookup("raw"))
}

func resolveWriteOpts(args []string) (writeOpts, error) {
	opts := writeOpts{
		runPath: viper.GetString(config.SB_ROOT_RUN_PATH.ViperKey),
		raw:     viper.GetBool(config.SB_WRITE_RAW.ViperKey),
	}

	if len(args) == 0 {
		return opts, errdefs.ErrNoTerminalIdentifier
	}
	opts.name = args[0]
	if opts.name == "" {
		return opts, errdefs.ErrNoTerminalIdentifier
	}

	// Payload source: second positional or stdin ('-').
	var raw []byte
	switch {
	case len(args) == 1:
		// No payload argument. Treat as empty; bail early so we don't
		// invoke the RPC for zero bytes.
		raw = nil
	case args[1] == "-":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return opts, fmt.Errorf("read stdin: %w", err)
		}
		raw = data
	default:
		raw = []byte(args[1])
	}

	if opts.raw {
		opts.payload = raw
		return opts, nil
	}

	parsed, err := parseCaret(raw)
	if err != nil {
		return opts, fmt.Errorf("%w: %w", errdefs.ErrInvalidArgument, err)
	}
	opts.payload = parsed
	return opts, nil
}

func runWrite(ctx context.Context, logger *slog.Logger, opts writeOpts) error {
	if len(opts.payload) == 0 {
		logger.DebugContext(ctx, "write: empty payload, nothing to do")
		return nil
	}

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

	return rc.Write(ctx, &api.WriteRequest{Data: opts.payload})
}
