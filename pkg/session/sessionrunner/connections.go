package sessionrunner

import (
	"fmt"
	"io"
	"log/slog"
	"sbsh/pkg/api"
)

func (sr *SessionRunnerExec) handleConnections() error {

	cid := 0
	for {
		// New client connects
		slog.Debug("[session] waiting for new connection...\r\n")
		conn, err := sr.listenerIO.Accept()
		if err != nil {
			slog.Debug("[session] closing IO listener routine\r\n")
			return err
		}
		slog.Debug("[session] client connected!\r\n")
		cid++
		cl := &ioClient{id: cid, conn: conn}

		sr.addClient(cl)
		go sr.handleClient(cl)
	}
}

func (sr *SessionRunnerExec) handleClient(client *ioClient) {
	defer client.conn.Close()
	sr.status.State = api.SessionStatusAttached
	_ = sr.updateMetadata()
	errCh := make(chan error, 2)

	// READ FROM CONN, WRITE TO PTY STDIN
	go func(chan error) {
		// conn writes to pipeInW
		w, err := io.Copy(sr.ptyPipes.pipeInW, client.conn)
		if err != nil {
			errCh <- fmt.Errorf("error in conn->pty copy pipe: %w", err)
		}
		if w == 0 {
			errCh <- fmt.Errorf("EOF in conn->pty copy pipe: %w", err)
		}
	}(errCh)

	// READ FROM PTY STDOUT, WRITE TO CONN
	go func(chan error) {
		// conn reads from pipeOutR
		w, err := io.Copy(client.conn, sr.ptyPipes.pipeOutR)
		if err != nil {
			errCh <- fmt.Errorf("error in pty->conn copy pipe: %w", err)
		}
		if w == 0 {
			errCh <- fmt.Errorf("EOF in pty->conn copy pipe: %w", err)
		}
	}(errCh)

	err := <-errCh
	if err != nil {
		slog.Debug(fmt.Sprintf("[session-runner] error in copy pipes: %v\r\n", err))
	}
	client.conn.Close()
	close(errCh)
	sr.removeClient(client)

}

func (sr *SessionRunnerExec) addClient(c *ioClient) {
	sr.clientsMu.Lock()
	sr.clients[c.id] = c
	sr.clientsMu.Unlock()
}

func (sr *SessionRunnerExec) removeClient(c *ioClient) {
	sr.clientsMu.Lock()
	delete(sr.clients, c.id)
	sr.clientsMu.Unlock()
}
