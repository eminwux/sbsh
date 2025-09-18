package sessionrunner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"path/filepath"
	"sbsh/pkg/api"
	"sbsh/pkg/common"
	"sbsh/pkg/session/sessionrpc"
)

type SessionRunner interface {
	OpenSocketCtrl(sessionID api.SessionID) (net.Listener, error)
	StartServer(ctx context.Context, ln net.Listener, sc *sessionrpc.SessionControllerRPC, readyCh chan error, errCh chan error)
}

type SessionRunnerExec struct {
}

func NewSessionRunnerExec() SessionRunner {
	return &SessionRunnerExec{}
}

func (sr *SessionRunnerExec) OpenSocketCtrl(sessionID api.SessionID) (net.Listener, error) {

	// Set up sockets
	base, err := common.RuntimeBaseSessions()
	if err != nil {
		return nil, err
	}

	runPath := filepath.Join(base, string(sessionID))
	if err := os.MkdirAll(runPath, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir session dir: %w", err)
	}

	socketCtrl := filepath.Join(runPath, "ctrl.sock")
	log.Printf("[sessionCtrl] CTRL socket: %s", socketCtrl)

	// Remove sockets if they already exist
	// remove sockets and dir
	if err := os.Remove(socketCtrl); err != nil {
		log.Printf("[sessionCtrl] couldn't remove stale CTRL socket: %s\r\n", socketCtrl)
	}

	// Listen to CONTROL SOCKET
	ctrlLn, err := net.Listen("unix", socketCtrl)
	if err != nil {
		return nil, fmt.Errorf("listen ctrl: %w", err)
	}
	if err := os.Chmod(socketCtrl, 0o600); err != nil {
		ctrlLn.Close()
		return nil, err
	}

	// keep references for Close()

	return ctrlLn, nil
}
func (sr *SessionRunnerExec) StartServer(ctx context.Context, ln net.Listener, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error) {
	defer func() {
		// Ensure ln is closed and no leaks on exit
		_ = ln.Close()
	}()

	srv := rpc.NewServer()
	if err := srv.RegisterName("SessionController", sc); err != nil {

		// startup failed
		readyCh <- err
		close(readyCh)

		// also inform 'done' since we won't run
		select {
		case doneCh <- err:
		default:
		}
		close(doneCh)
		return
	}
	// Signal: the accept loop is about to run on an already-listening socket.
	readyCh <- nil
	close(readyCh)

	// stop accepting when ctx is canceled.
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			// Normal path: listener closed by ctx cancel
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				select {
				case doneCh <- nil:
				default:
				}
				close(doneCh)
				return
			}
			// Abnormal accept error
			select {
			case doneCh <- err:
			default:
			}
			close(doneCh)
			return
		}
		go srv.ServeCodec(jsonrpc.NewServerCodec(conn))
	}
}
