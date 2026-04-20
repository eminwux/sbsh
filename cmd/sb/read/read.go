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

package read

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/sb/get"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/naming"
	"github.com/eminwux/sbsh/pkg/api"
	rpcterminal "github.com/eminwux/sbsh/pkg/rpcclient/terminal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type readOpts struct {
	runPath string
	name    string
	follow  bool
}

func NewReadCmd() *cobra.Command {
	readCmd := &cobra.Command{
		Use:   "read <name>",
		Short: "Print a terminal's capture log; with -f, follow live output",
		Long: `Print the full capture log for a terminal.

With -f/--follow, also stream live PTY output after replaying the file, like
'tail -f'. The follower subscribes to live output BEFORE replaying the capture
file, so no bytes can be dropped in the handoff; at most a small window of
bytes near the cutover may appear both in the replay and the live stream.

If the subscriber falls behind the PTY reader (default buffer: 1 MiB), the
stream is closed with a lagged-notice sentinel and this command exits
non-zero.`,
		SilenceUsage:      true,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: get.CompleteTerminals,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
			if !ok || logger == nil {
				return errdefs.ErrLoggerNotFound
			}
			opts := readOpts{
				runPath: viper.GetString(config.SB_ROOT_RUN_PATH.ViperKey),
				name:    args[0],
				follow:  viper.GetBool(config.SB_READ_FOLLOW.ViperKey),
			}
			if opts.name == "" {
				return errdefs.ErrNoTerminalIdentifier
			}
			return runRead(cmd.Context(), logger, os.Stdout, os.Stderr, opts)
		},
	}

	setupReadFlags(readCmd)
	return readCmd
}

func setupReadFlags(c *cobra.Command) {
	c.Flags().BoolP("follow", "f", false, "Replay capture log, then stream live output until interrupted")
	_ = viper.BindPFlag(config.SB_READ_FOLLOW.ViperKey, c.Flags().Lookup("follow"))
}

func runRead(
	ctx context.Context,
	logger *slog.Logger,
	stdout, stderr io.Writer,
	opts readOpts,
) error {
	doc, err := discovery.FindTerminalByName(ctx, logger, opts.runPath, opts.name)
	if err != nil {
		return fmt.Errorf("%w: %w", errdefs.ErrTerminalNotFound, err)
	}
	if doc == nil {
		return fmt.Errorf("%w: terminal %q", errdefs.ErrTerminalNotFound, opts.name)
	}

	capturePath := doc.Status.CaptureFile
	if capturePath == "" {
		return errors.New("terminal metadata has no capture file recorded")
	}

	if !opts.follow {
		return dumpCapture(capturePath, stdout)
	}

	socket := doc.Status.SocketFile
	if socket == "" {
		return errors.New("terminal metadata has no control socket recorded")
	}

	// SIGINT/SIGTERM unblock the streaming copy.
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	rc := rpcterminal.NewUnix(socket, logger)
	defer func() { _ = rc.Close() }()

	// Subscribe BEFORE reading the capture file so bytes written during the
	// replay are captured in the live stream rather than lost at the cutover.
	conn, err := rc.Subscribe(ctx, &api.SubscribeRequest{ClientID: api.ID(naming.RandomID())}, &api.Empty{})
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if err := dumpCapture(capturePath, stdout); err != nil {
		return err
	}

	// Close conn when ctx is done so io.Copy unblocks.
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	return streamLive(conn, stdout, stderr)
}

func dumpCapture(path string, stdout io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Empty capture is indistinguishable from "never written to";
			// treat absence as zero-byte output rather than an error.
			return nil
		}
		return fmt.Errorf("open capture file %q: %w", path, err)
	}
	defer f.Close()
	if _, err := io.Copy(stdout, f); err != nil {
		return fmt.Errorf("read capture file %q: %w", path, err)
	}
	return nil
}

// laggedNotice mirrors the bracketed core of the server sentinel (see
// internal/terminal/terminalrunner/subscriber.go). The server wraps this
// in \r\n framing; the needle here is the bare bracketed form so framing
// can evolve on the server without breaking client detection.
var laggedNotice = []byte("[sbsh: subscriber lagged, disconnecting]")

func streamLive(conn net.Conn, stdout, stderr io.Writer) error {
	buf := make([]byte, 4096)
	// A single server-side Write delivers the sentinel atomically, but
	// TCP/socket reads can still split it across chunk boundaries. Keep
	// a rolling tail of (len(laggedNotice)-1) bytes from the previous
	// chunk and scan tail+chunk so a split sentinel is still detected.
	tailCap := len(laggedNotice) - 1
	var tail []byte
	laggedSeen := false
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if !laggedSeen {
				scan := chunk
				if len(tail) > 0 {
					scan = append(append(make([]byte, 0, len(tail)+n), tail...), chunk...)
				}
				if bytes.Contains(scan, laggedNotice) {
					laggedSeen = true
				} else if tailCap > 0 {
					if len(scan) > tailCap {
						tail = append(tail[:0], scan[len(scan)-tailCap:]...)
					} else {
						tail = append(tail[:0], scan...)
					}
				}
			}
			if _, werr := stdout.Write(chunk); werr != nil {
				return fmt.Errorf("write stdout: %w", werr)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				if laggedSeen {
					fmt.Fprintln(stderr, "sb read: subscriber lagged, stream truncated")
					return errdefs.ErrSubscriberLagged
				}
				return nil
			}
			return fmt.Errorf("stream read: %w", err)
		}
	}
}
