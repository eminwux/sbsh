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

const deleteSessionDir bool = false

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

	go sr.handleConnections()

	go sr.waitOnSession()

	return nil
}

func (sr *SessionRunnerExec) waitOnSession() {

	select {
	case err := <-sr.closeReqCh:
		slog.Debug("[session] sending EvSessionExited event\r\n")
		trySendEvent(sr.evCh, SessionRunnerEvent{ID: sr.id, Type: EvCmdExited, Err: err, When: time.Now()})
		return
	}

}

func (sr *SessionRunnerExec) Close(reason error) error {

	slog.Debug(fmt.Sprintf("[session-runner] closing session-runner on request, reason: %v\r\n", reason))

	sr.metadata.Status.State = api.SessionStatusExited
	sr.updateMetadata()

	sr.ctxCancel()

	slog.Debug("[session-runner] closed \r\n")

	// stop accepting
	if sr.listenerCtrl != nil {
		if err := sr.listenerCtrl.Close(); err != nil {
			slog.Debug(fmt.Sprintf("[session-runner] could not close IO listener: %v", err))
		}
	}
	// stop accepting
	if sr.listenerIO != nil {
		if err := sr.listenerIO.Close(); err != nil {
			slog.Debug(fmt.Sprintf("[session-runner] could not close IO listener: %v", err))
		}
	}

	// close clients
	sr.clientsMu.Lock()
	for _, c := range sr.clients {
		if err := c.conn.Close(); err != nil {
			slog.Debug(fmt.Sprintf("[session-runner] could not close connection: %v\r\n", err))
		}
	}

	sr.clients = nil
	sr.clientsMu.Unlock()

	// kill PTY child and close PTY master as needed
	if sr.cmd != nil && sr.cmd.Process != nil {
		if err := sr.cmd.Process.Kill(); err != nil {
			slog.Debug(fmt.Sprintf("[sesion] could not kill cmd: %v\r\n", err))
		}
	}
	if sr.pty != nil {
		var err error
		closePTY.Do(func() {
			err = sr.pty.Close()
		})
		if err != nil {
			slog.Debug(fmt.Sprintf("[sesion] could not close pty: %v\r\n", err))
		}
	}

	// remove sockets and dir
	if err := os.Remove(sr.socketIO); err != nil {
		slog.Debug(fmt.Sprintf("[session] couldn't remove IO socket: %s: %v\r\n", sr.socketIO, err))
	}

	// remove Ctrl socket
	if err := os.Remove(sr.socketCtrl); err != nil {
		slog.Debug(fmt.Sprintf("[sessionCtrl] couldn't remove Ctrl socket %s: %v\r\n", sr.socketCtrl, err))
	}

	if deleteSessionDir {
		if err := os.RemoveAll(filepath.Dir(sr.socketIO)); err != nil {
			slog.Debug(fmt.Sprintf("[session] couldn't remove session directory '%s': %v\r\n", sr.socketIO, err))
		}
	}

	close(sr.closedCh)
	return nil

}

func (sr *SessionRunnerExec) Resize(args api.ResizeArgs) {
	pty.Setsize(sr.pty, &pty.Winsize{
		Cols: uint16(args.Cols),
		Rows: uint16(args.Rows),
	})
}

// Write writes bytes to the session PTY (used by controller or Smart executor).
func (sr *SessionRunnerExec) Write(p []byte) (int, error) {
	return sr.pty.Write(p)
}

func (sr *SessionRunnerExec) Detach() error {
	// remove client pipe from multiwriterR
	// remove client pipe from stdin
	// close connection and clean-up

	return fmt.Errorf("error detaching")
}
