package sessionrunner

import (
	"fmt"
	"io"
	"log/slog"
	"os"
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

	client.pipeOutR, client.pipeOutW, _ = os.Pipe()
	sr.ptyPipes.multiOutW.Add(client.pipeOutW)

	log, _ := readFileBytes(sr.spec.LogFilename)

	errCh := make(chan error, 2)

	// READ FROM CONN, WRITE TO PTY STDIN
	go func(errCh chan error) {
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
				if rerr != io.EOF {
					errCh <- fmt.Errorf("error in conn->pty copy pipe: %w", rerr)
				} else if total == 0 {
					errCh <- fmt.Errorf("EOF in conn->pty copy pipe")
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
					errCh <- fmt.Errorf("EOF in pty->conn copy pipe")
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
