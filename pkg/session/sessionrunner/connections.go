package sessionrunner

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

func (s *SessionRunnerExec) handleConnections(pipeInR, pipeInW, pipeOutR, pipeOutW *os.File) error {

	cid := 0
	for {
		// New client connects
		slog.Debug("[session] waiting for new connection...\r\n")
		conn, err := s.listenerIO.Accept()
		if err != nil {
			slog.Debug("[session] closing IO listener routine\r\n")
			return err
		}
		slog.Debug("[session] client connected!\r\n")
		cid++
		cl := &ioClient{id: cid, conn: conn, pipeInR: pipeInR, pipeInW: pipeInW, pipeOutR: pipeOutR, pipeOutW: pipeOutW}

		s.addClient(cl)
		go s.handleClient(cl)
	}
}

func (s *SessionRunnerExec) handleClient(client *ioClient) {
	defer client.conn.Close()
	errCh := make(chan error, 2)

	// READ FROM CONN, WRITE TO PTY STDIN
	go func(chan error) {
		// conn writes to pipeInW
		w, err := io.Copy(client.pipeInW, client.conn)
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
		w, err := io.Copy(client.conn, client.pipeOutR)
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
	s.removeClient(client)

}

func (s *SessionRunnerExec) addClient(c *ioClient) {
	s.clientsMu.Lock()
	s.clients[c.id] = c
	s.clientsMu.Unlock()
}

func (s *SessionRunnerExec) removeClient(c *ioClient) {
	s.clientsMu.Lock()
	delete(s.clients, c.id)
	s.clientsMu.Unlock()
}
