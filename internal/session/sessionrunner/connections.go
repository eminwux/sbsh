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
	"net"

	"github.com/eminwux/sbsh/internal/dualcopier"
	"github.com/eminwux/sbsh/pkg/api"
)

func (sr *SessionRunnerExec) cleanupClient(client *ioClient) {
	sr.logger.Info("client connection handler exiting", "client", client.id)
	if cerr := client.conn.Close(); cerr != nil {
		sr.logger.Warn("error closing client connection", "err", cerr, "client", client.id)
	}
	if err := sr.updateSessionAttachers(); err != nil {
		sr.logger.Warn("failed to update metadata on client cleanup", "err", err)
	}
	sr.removeClient(client)
}

func (sr *SessionRunnerExec) handleClient(client *ioClient) {
	sr.logger.Info("client connection handler started", "client", client.id)

	_ = sr.setupPipes(client)

	uc, _ := client.conn.(*net.UnixConn)

	dc := dualcopier.NewCopier(sr.ctx, sr.logger)

	// WRITER: stdin -> socket
	go dc.RunCopier(uc, client.pipeOutR, client.conn)

	// READER: socket  -> stdin
	go dc.RunCopier(uc, client.conn, sr.ptyPipes.pipeInW)

	// MANAGER
	go dc.ConnectionManager(uc, func() {
		sr.cleanupClient(client)
		_ = sr.updateSessionAttachers()
	})

	if errAttach := sr.updateSessionAttachers(); errAttach != nil {
		sr.logger.Warn("failed to update metadata on attach", "err", errAttach)
	}

	log, err := sr.readLogFile()
	if err != nil {
		sr.logger.Warn("failed to read log file for client attach", "err", err)
	}

	if _, errWrite := client.conn.Write(log); err != nil {
		sr.logger.Error("error in pty->conn initial write", "err", errWrite, "client", client.id)
		return
	}
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
