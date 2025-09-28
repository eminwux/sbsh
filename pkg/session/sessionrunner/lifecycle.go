package sessionrunner

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sbsh/pkg/api"
	"time"

	"github.com/creack/pty"
)

func (sr *SessionRunnerExec) StartSession(evCh chan<- SessionRunnerEvent) error {

	sr.evCh = evCh

	if err := sr.openSocketIO(); err != nil {
		err = fmt.Errorf("failed to open IO socket for session %s: %w", sr.id, err)
		return err
	}

	if err := sr.prepareSessionCommand(); err != nil {
		err = fmt.Errorf("failed to run session command for session %s: %v", sr.id, err)
		return err
	}

	if err := sr.startPTY(); err != nil {
		err = fmt.Errorf("failed to start PTY for session %s: %v", sr.id, err)
		return err
	}

	go sr.waitOnSession()

	return nil
}

func (s *SessionRunnerExec) waitOnSession() {

	select {
	case err := <-s.closeReqCh:
		slog.Debug("[session] sending EvSessionExited event\r\n")
		trySendEvent(s.evCh, SessionRunnerEvent{ID: s.id, Type: EvCmdExited, Err: err, When: time.Now()})
		return
	}

}

func (s *SessionRunnerExec) Close(reason error) error {

	slog.Debug(fmt.Sprintf("[session-runner] closing session-runner on request, reason: %v\r\n", reason))
	slog.Debug("[session-runner] sent 'closingCh' signal\r\n")

	// stop terminalManager, 2 messages needed, one for writer/reader
	close(finishTermMgr)
	slog.Debug("[session-runner] closed 'finishTermMgr' \r\n")

	// stop accepting
	if s.listenerCtrl != nil {
		if err := s.listenerCtrl.Close(); err != nil {
			slog.Debug(fmt.Sprintf("[session-runner] could not close IO listener: %v", err))
			// return err
		}
	}
	// stop accepting
	if s.listenerIO != nil {
		if err := s.listenerIO.Close(); err != nil {
			slog.Debug(fmt.Sprintf("[session-runner] could not close IO listener: %v", err))
			// return err
		}
	}

	// close clients
	s.clientsMu.Lock()
	for _, c := range s.clients {
		if err := c.conn.Close(); err != nil {
			slog.Debug(fmt.Sprintf("[session-runner] could not close connection: %v\r\n", err))
			// return err
		}
	}

	s.clients = nil
	s.clientsMu.Unlock()

	// kill PTY child and close PTY master as needed
	if s.cmd != nil && s.cmd.Process != nil {
		if err := s.cmd.Process.Kill(); err != nil {
			slog.Debug(fmt.Sprintf("[sesion] could not kill cmd: %v\r\n", err))
			// return err
		}
	}
	if s.pty != nil {
		if err := s.pty.Close(); err != nil {
			slog.Debug(fmt.Sprintf("[sesion] could not close pty: %v\r\n", err))
			// return err
		}
	}

	// remove sockets and dir
	if err := os.Remove(s.socketIO); err != nil {
		slog.Debug(fmt.Sprintf("[session] couldn't remove IO socket: %s: %v\r\n", s.socketIO, err))
		// return err
	}

	// remove Ctrl socket
	if err := os.Remove(s.socketCtrl); err != nil {
		slog.Debug(fmt.Sprintf("[sessionCtrl] couldn't remove Ctrl socket %s: %v\r\n", s.socketCtrl, err))
	}

	if deleteSessionDir {
		if err := os.RemoveAll(filepath.Dir(s.socketIO)); err != nil {
			slog.Debug(fmt.Sprintf("[session] couldn't remove session directory '%s': %v\r\n", s.socketIO, err))
		}
	}

	close(s.closedCh)
	return nil

}

func (sr *SessionRunnerExec) Resize(args api.ResizeArgs) {
	pty.Setsize(sr.pty, &pty.Winsize{
		Cols: uint16(args.Cols),
		Rows: uint16(args.Rows),
	})
}

// Write writes bytes to the session PTY (used by controller or Smart executor).
func (s *SessionRunnerExec) Write(p []byte) (int, error) {
	return s.pty.Write(p)
}

func (s *SessionRunnerExec) Detach() error {
	return nil
}
