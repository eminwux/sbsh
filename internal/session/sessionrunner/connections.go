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
	"io"
	"os"

	"github.com/eminwux/sbsh/pkg/api"
)

func (sr *SessionRunnerExec) cleanupClient(client *ioClient) {
	sr.logger.Info("client connection handler exiting", "client", client.id)
	if cerr := client.conn.Close(); cerr != nil {
		sr.logger.Warn("error closing client connection", "err", cerr, "client", client.id)
	}
	if err := sr.updateSessionState(api.SessionStatusDetached); err != nil {
		sr.logger.Error("failed to update session state on detach", "err", err, "client", client.id)
	}
	sr.removeClient(client)
}

func (sr *SessionRunnerExec) handleClient(client *ioClient) {
	defer client.conn.Close()
	sr.logger.Info("client connection handler started", "client", client.id)

	if err := sr.updateSessionState(api.SessionStatusAttached); err != nil {
		sr.logger.Warn("failed to update metadata on attach", "err", err)
	}

	client.pipeOutR, client.pipeOutW, _ = os.Pipe()
	sr.ptyPipes.multiOutW.Add(client.pipeOutW)

	log, err := readFileBytes(sr.metadata.Status.CaptureFile)
	if err != nil {
		sr.logger.Warn("failed to read log file for client attach", "err", err)
	}

	//nolint:mnd // channel buffer size
	errCh := make(chan error, 2)

	// READ FROM CONN, WRITE TO PTY STDIN
	go func(errCh chan error) {
		//nolint:mnd // buffer size, 32 KiB buffer, same as io.Copy
		buf := make([]byte, 32*1024)
		var total int64

		for {
			sr.logger.Debug("conn->pty pre-read", "client", client.id)
			n, rerr := client.conn.Read(buf)
			sr.logger.Debug("conn->pty post-read", "client", client.id, "n", n)
			if n > 0 {
				written := 0
				for written < n {
					sr.logger.Debug("conn->pty pre-write", "client", client.id)
					m, werr := sr.ptyPipes.pipeInW.Write(buf[written:n])
					sr.logger.Debug("conn->pty post-write", "client", client.id, "m", m)
					if werr != nil {
						errCh <- fmt.Errorf("error in conn->pty copy pipe: %w", werr)
						sr.logger.Error(
							"error in conn->pty copy pipe",
							"err",
							werr,
							"client",
							client.id,
						)
						return
					}
					written += m
					total += int64(m)
				}
			}

			if rerr != nil {
				if !errors.Is(rerr, io.EOF) {
					errCh <- fmt.Errorf("error in conn->pty copy pipe: %w", rerr)
					sr.logger.Error("error in conn->pty read", "err", rerr, "client", client.id)
				} else if total == 0 {
					errCh <- errors.New("EOF in conn->pty copy pipe")
					sr.logger.Warn("EOF in conn->pty copy pipe", "client", client.id)
				}
				return
			}
		}
	}(errCh)

	// READ FROM PTY STDOUT, WRITE TO CONN
	go func(errCh chan error) {
		// optional initial write
		if _, err := client.conn.Write(log); err != nil {
			errCh <- fmt.Errorf("error in pty->conn initial write: %w", err)
			sr.logger.Error("error in pty->conn initial write", "err", err, "client", client.id)
			return
		}

		//nolint:mnd // buffer size, 32 KiB buffer, same as io.Copy
		buf := make([]byte, 32*1024)
		var total int64

		for {
			sr.logger.Debug("pty->conn pre-read", "client", client.id)
			n, rerr := client.pipeOutR.Read(buf)
			sr.logger.Debug("pty->conn post-read", "client", client.id, "n", n)
			if n > 0 {
				written := 0
				for written < n {
					sr.logger.Debug("pty->conn pre-write", "client", client.id)
					m, werr := client.conn.Write(buf[written:n])
					sr.logger.Debug("pty->conn post-write", "client", client.id, "m", m)
					if werr != nil {
						errCh <- fmt.Errorf("error in pty->conn copy pipe: %w", werr)
						sr.logger.Error(
							"error in pty->conn copy pipe",
							"err",
							werr,
							"client",
							client.id,
						)
						return
					}
					written += m
					total += int64(m)
				}
			}

			if rerr != nil {
				if rerr != io.EOF {
					errCh <- fmt.Errorf("error in pty->conn copy pipe: %w", rerr)
					sr.logger.Error("error in pty->conn read", "err", rerr, "client", client.id)
				} else if total == 0 {
					errCh <- errors.New("EOF in pty->conn copy pipe")
					sr.logger.Warn("EOF in pty->conn copy pipe", "client", client.id)
				}
				return
			}
		}
	}(errCh)

	err = <-errCh
	if err != nil {
		sr.logger.Error("error in copy pipes", "err", err, "client", client.id)
	}

	sr.cleanupClient(client)
}

func (sr *SessionRunnerExec) addClient(c *ioClient) {
	sr.clientsMu.Lock()
	sr.clients[*c.id] = c
	sr.clientsMu.Unlock()
}

func (sr *SessionRunnerExec) removeClient(c *ioClient) {
	sr.clientsMu.Lock()
	delete(sr.clients, *c.id)
	sr.clientsMu.Unlock()
}

func (sr *SessionRunnerExec) getClient(id api.ID) (*ioClient, bool) {
	sr.clientsMu.RLock()
	defer sr.clientsMu.RUnlock()

	c, ok := sr.clients[id]
	return c, ok
}
