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
	"log/slog"
	"os"

	"github.com/eminwux/sbsh/pkg/api"
)

func (sr *SessionRunnerExec) handleClient(client *ioClient) {
	defer client.conn.Close()
	sr.metadata.Status.State = api.SessionStatusAttached
	_ = sr.updateMetadata()

	client.pipeOutR, client.pipeOutW, _ = os.Pipe()
	sr.ptyPipes.multiOutW.Add(client.pipeOutW)

	log, _ := readFileBytes(sr.metadata.Status.LogFile)

	//nolint:mnd // channel buffer size
	errCh := make(chan error, 2)

	// READ FROM CONN, WRITE TO PTY STDIN
	go func(errCh chan error) {
		//nolint:mnd // event channel buffer size
		buf := make([]byte, 32*1024) // 32 KiB buffer, same as io.Copy
		var total int64

		for {
			slog.Debug("conn->pty pre-read")
			n, rerr := client.conn.Read(buf)
			slog.Debug(fmt.Sprintf("conn->pty post-read: %d", n))
			if n > 0 {
				written := 0
				for written < n {
					slog.Debug("conn->pty pre-write")
					m, werr := sr.ptyPipes.pipeInW.Write(buf[written:n])
					slog.Debug(fmt.Sprintf("conn->pty post-write: %d", m))
					if werr != nil {
						errCh <- fmt.Errorf("error in conn->pty copy pipe: %w", werr)
						return
					}
					written += m
					total += int64(m)
				}
			}

			if rerr != nil {
				if !errors.Is(rerr, io.EOF) {
					errCh <- fmt.Errorf("error in conn->pty copy pipe: %w", rerr)
				} else if total == 0 {
					errCh <- errors.New("EOF in conn->pty copy pipe")
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
			return
		}

		//nolint:mnd // buffer size
		buf := make([]byte, 32*1024) // similar buffer size to io.Copy
		var total int64

		for {
			slog.Debug("pty->conn pre-read")
			n, rerr := client.pipeOutR.Read(buf)
			slog.Debug(fmt.Sprintf("pty->conn post-read: %d", n))
			if n > 0 {
				written := 0
				for written < n {
					slog.Debug("pty->conn pre-write")
					m, werr := client.conn.Write(buf[written:n])
					slog.Debug(fmt.Sprintf("pty->conn post-write: %d", m))
					if werr != nil {
						errCh <- fmt.Errorf("error in pty->conn copy pipe: %w", werr)
						return
					}
					written += m
					total += int64(m)
				}
			}

			if rerr != nil {
				if rerr != io.EOF {
					errCh <- fmt.Errorf("error in pty->conn copy pipe: %w", rerr)
				} else if total == 0 {
					errCh <- errors.New("EOF in pty->conn copy pipe")
				}
				return
			}
		}
	}(errCh)

	err := <-errCh
	if err != nil {
		slog.Debug(fmt.Sprintf("[session-runner] error in copy pipes: %v\r\n", err))
	}
	client.conn.Close()
	sr.removeClient(client)
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
