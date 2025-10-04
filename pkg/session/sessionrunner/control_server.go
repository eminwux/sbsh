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

package sessionrunner

import (
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"sbsh/pkg/api"
	"sbsh/pkg/session/sessionrpc"
)

func (sr *SessionRunnerExec) StartServer(sc *sessionrpc.SessionControllerRPC, readyCh chan error) {
	defer func() {
		// Ensure ln is closed and no leaks on exit
		_ = sr.listenerCtrl.Close()
	}()

	srv := rpc.NewServer()
	if err := srv.RegisterName(api.SessionService, sc); err != nil {

		// startup failed
		readyCh <- err
		close(readyCh)

		// also inform 'done' since we won't run
		select {
		case sr.closeReqCh <- err:
		default:
		}
		return
	}
	// Signal: the accept loop is about to run on an already-listening socket.
	readyCh <- nil
	close(readyCh)

	// stop accepting when ctx is canceled.

	for {
		conn, err := sr.listenerCtrl.Accept()
		if err != nil {
			// Normal path: listener closed by ctx cancel
			if errors.Is(err, net.ErrClosed) || sr.ctx.Err() != nil {
				select {
				case sr.closeReqCh <- fmt.Errorf("unknown rpc server error"):
				default:
				}
				return
			}
			// Abnormal accept error
			select {
			case sr.closeReqCh <- err:
			default:
			}

			return
		}
		// go srv.ServeCodec(jsonrpc.NewServerCodec(conn))
		uconn := conn.(*net.UnixConn)
		go srv.ServeCodec(sessionrpc.NewUnixJSONServerCodec(uconn))

	}
}
