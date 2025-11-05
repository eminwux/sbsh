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
	"net"

	"github.com/eminwux/sbsh/internal/dualcopier"
	"github.com/eminwux/sbsh/pkg/api"
)

func (sr *Exec) cleanupClient(client *ioClient) {
	sr.logger.Info("client connection handler exiting", "client", client.id)
	if cerr := client.conn.Close(); cerr != nil {
		sr.logger.Warn("error closing client connection", "err", cerr, "client", client.id)
	}
	if err := sr.updateTerminalAttachers(); err != nil {
		sr.logger.Warn("failed to update metadata on client cleanup", "err", err)
	}
	sr.removeClient(client)
}

func (sr *Exec) handleClient(client *ioClient) {
	sr.logger.Info("client connection handler started", "client", client.id)

	if err := sr.setupPipes(client); err != nil {
		sr.logger.Error("failed to setup pipes for client", "err", err, "client", client.id)
		return
	}

	uc, ok := client.conn.(*net.UnixConn)
	if !ok {
		sr.logger.Error("client connection is not a UnixConn", "client", client.id)
		return
	}

	dc := dualcopier.NewCopier(sr.ctx, sr.logger)

	// WRITER: stdout -> socket
	readyWriter := make(chan struct{})
	go dc.RunCopier(client.pipeOutR, client.conn, readyWriter, func() {
		if uc != nil {
			sr.logger.Debug("closing UnixConn write side", "client", client.id)
			_ = uc.CloseWrite()
		}
	}, nil)

	// READER: socket  -> stdin
	readyReader := make(chan struct{})
	go dc.RunCopier(client.conn, sr.ptyPipes.pipeInW, readyReader, func() {
		if uc != nil {
			sr.logger.Debug("closing UnixConn read side", "client", client.id)
			_ = uc.CloseRead()
		}
	}, nil)

	<-readyWriter
	<-readyReader

	// MANAGER
	go dc.CopierManager(uc, func() {
		sr.cleanupClient(client)
		_ = sr.updateTerminalAttachers()
	})

	if errAttach := sr.updateTerminalAttachers(); errAttach != nil {
		sr.logger.Warn("failed to update metadata on attach", "err", errAttach)
		return
	}

	log, errLog := sr.readLogFile()
	if errLog != nil {
		sr.logger.Warn("failed to read log file for client attach", "err", errLog)
		return
	}

	if _, errWrite := client.conn.Write(log); errWrite != nil {
		sr.logger.Error("error in pty->conn initial write", "err", errWrite, "client", client.id)
		return
	}
}

func (sr *Exec) addClient(c *ioClient) {
	sr.clientsMu.Lock()
	sr.clients[*c.id] = c
	sr.clientsMu.Unlock()
}

func (sr *Exec) removeClient(c *ioClient) {
	sr.clientsMu.Lock()
	delete(sr.clients, *c.id)
	sr.clientsMu.Unlock()
}

func (sr *Exec) getClient(id api.ID) (*ioClient, bool) {
	sr.clientsMu.RLock()
	defer sr.clientsMu.RUnlock()

	c, ok := sr.clients[id]
	return c, ok
}
