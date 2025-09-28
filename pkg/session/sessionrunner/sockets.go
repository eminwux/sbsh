package sessionrunner

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
)

func (sr *SessionRunnerExec) openSocketIO() error {

	runPath := filepath.Join(sr.runPath, string(sr.id))
	if err := os.MkdirAll(runPath, 0o700); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	sr.socketIO = filepath.Join(runPath, "io.sock")
	slog.Debug(fmt.Sprintf("[session] IO socket: %s", sr.socketIO))

	// Remove socket if already exists
	if err := os.Remove(sr.socketIO); err != nil {
		slog.Debug(fmt.Sprintf("[session] couldn't remove stale IO socket: %s\r\n", sr.socketIO))
	}

	// Listen to IO SOCKET
	ioLn, err := net.Listen("unix", sr.socketIO)
	if err != nil {
		return fmt.Errorf("listen io: %w", err)
	}
	if err := os.Chmod(sr.socketIO, 0o600); err != nil {
		_ = ioLn.Close()
		return err
	}

	sr.clientsMu.Lock()
	sr.clients = make(map[int]*ioClient)
	sr.clientsMu.Unlock()

	sr.listenerIO = ioLn

	return nil
}

func (sr *SessionRunnerExec) OpenSocketCtrl() error {

	sr.socketCtrl = filepath.Join(sr.getSessionDir(), "ctrl.sock")
	slog.Debug(fmt.Sprintf("[sessionCtrl] CTRL socket: %s", sr.socketCtrl))

	// Remove sockets if they already exist
	// remove sockets and dir
	if err := os.Remove(sr.socketCtrl); err != nil {
		slog.Debug(fmt.Sprintf("[sessionCtrl] couldn't remove stale CTRL socket: %s\r\n", sr.socketCtrl))
	}

	// Listen to CONTROL SOCKET
	ctrlLn, err := net.Listen("unix", sr.socketCtrl)
	if err != nil {
		return fmt.Errorf("listen ctrl: %w", err)
	}

	sr.listenerCtrl = ctrlLn

	if err := os.Chmod(sr.socketCtrl, 0o600); err != nil {
		ctrlLn.Close()
		return err
	}

	// keep references for Close()

	return nil
}
