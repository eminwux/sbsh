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

package terminalrunner

import (
	"context"
	"errors"
	"net"
	"net/rpc"

	"github.com/eminwux/sbsh/internal/terminal/terminalrpc"
	"github.com/eminwux/sbsh/pkg/api"
)

//nolint:gocognit // long start server
func (sr *Exec) StartServer(
	_ context.Context,
	sc *terminalrpc.TerminalControllerRPC,
	readyCh chan error,
	doneCh chan error,
) {
	// Ensure ln is closed and no leaks on exit
	defer func() {
		// Ensure ln is closed and no leaks on exit
		_ = sr.lnCtrl.Close()
	}()

	// stop accepting when ctx is canceled.
	go func() {
		<-sr.ctx.Done()
		_ = sr.lnCtrl.Close()
	}()

	srv := rpc.NewServer()
	if err := srv.RegisterName(api.TerminalService, sc); err != nil {
		// startup failed
		readyCh <- err
		close(readyCh)

		// also inform 'done' since we won't run
		select {
		case doneCh <- err:
		default:
		}
		close(doneCh)
		return
	}
	// Signal: the accept loop is about to run on an already-listening socket.
	readyCh <- nil
	close(readyCh)

	// stop accepting when ctx is canceled.

	for {
		conn, err := sr.lnCtrl.Accept()
		if err != nil {
			// Normal path: listener closed by ctx cancel
			if errors.Is(err, net.ErrClosed) || sr.ctx.Err() != nil {
				select {
				case doneCh <- errors.New("unknown rpc server error"):
				default:
				}
				close(doneCh)
				return
			}
			// Abnormal accept error
			select {
			case doneCh <- err:
			default:
			}
			close(doneCh)
			return
		}
		uconn, ok := conn.(*net.UnixConn)
		if !ok {
			errCast := errors.New("failed to cast connection to *net.UnixConn")
			if sr.logger != nil {
				sr.logger.ErrorContext(sr.ctx, "connection cast error", "error", errCast)
			}
			select {
			case doneCh <- errCast:
			default:
			}
			close(doneCh)
			return
		}
		go srv.ServeCodec(terminalrpc.NewUnixJSONServerCodec(uconn, sr.logger))
	}
}
