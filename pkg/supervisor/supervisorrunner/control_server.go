package supervisorrunner

import (
	"context"
	"errors"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"sbsh/pkg/supervisor/supervisorrpc"
)

func (s *SupervisorRunnerExec) StartServer(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, doneCh chan error) {
	// Ensure ln is closed and no leaks on exit
	defer func() {
		_ = s.lnCtrl.Close()
	}()

	// stop accepting when ctx is canceled.
	go func() {
		<-ctx.Done()
		_ = s.lnCtrl.Close()
	}()

	srv := rpc.NewServer()
	if err := srv.RegisterName("SupervisorController", sc); err != nil {

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

	for {
		conn, err := s.lnCtrl.Accept()
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
