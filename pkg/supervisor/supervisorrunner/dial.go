package supervisorrunner

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"os"
	"sbsh/pkg/api"
	"sbsh/pkg/rpcclient/session"
	"syscall"
	"time"
)

func (sr *SupervisorRunnerExec) dialSessionCtrlSocket() error {

	slog.Debug(fmt.Sprintf("[supervisor] %s session on  %d trying to connect to %s\r\n", sr.session.Id, sr.session.Pid, sr.session.SockerCtrl))

	sr.sessionClient = session.NewUnix(sr.session.SockerCtrl)
	defer sr.sessionClient.Close()

	ctx, cancel := context.WithTimeout(sr.ctx, 3*time.Second)
	defer cancel()

	var status api.SessionStatusMessage
	if err := sr.sessionClient.Status(ctx, &status); err != nil {
		return fmt.Errorf("status failed: %w", err)
	}

	slog.Debug(fmt.Sprintf("[supervisor] rpc->session (Status): %+v\r\n", status))

	return nil

}

func (sr *SupervisorRunnerExec) attachIOSocket() error {

	var conn net.Conn
	var err error

	// Dial the Unix domain socket
	for range 3 {
		conn, err = net.Dial("unix", sr.session.SocketIO)
		if err == nil {
			break // success
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err != nil {
		log.Fatalf("failed to connect to io.sock after 3 retries: %v", err)
		return err
	}

	// Connected, now we enable raw mode
	if err := sr.toBashUIMode(); err != nil {
		slog.Debug(fmt.Sprintf("[supervisor] initial raw mode failed: %v", err))
	}

	// We want half-closes; UnixConn exposes CloseRead/CloseWrite
	uc, _ := conn.(*net.UnixConn)

	errCh := make(chan error, 2)

	// WRITER stdin -> socket
	go func() {
		_, e := io.Copy(conn, os.Stdin)
		// tell peer we're done sending (but still willing to read)
		if uc != nil {
			_ = uc.CloseWrite()

		}
		// send event (EOF or error while copying stdin -> socket)
		if e == io.EOF {
			slog.Debug("[supervisor] stdin reached EOF\r\n")
			trySendEvent(sr.events, SupervisorRunnerEvent{ID: sr.session.Id, Type: EvCmdExited, Err: err, When: time.Now()})
		} else if e != nil {
			slog.Debug(fmt.Sprintf("[supervisor] stdin->socket error: %v\r\n", e))
			trySendEvent(sr.events, SupervisorRunnerEvent{ID: sr.session.Id, Type: EvError, Err: err, When: time.Now()})
		}

		errCh <- e
	}()

	// READER socket -> stdout
	go func() {
		_, e := io.Copy(os.Stdout, conn)
		// we won't read further; let the other goroutine finish
		if uc != nil {
			_ = uc.CloseRead()
		}
		// send event (EOF or error while copying socket -> stdout)
		if e == io.EOF {
			slog.Debug("[supervisor] socket closed (EOF)\r\n")
			trySendEvent(sr.events, SupervisorRunnerEvent{ID: sr.session.Id, Type: EvCmdExited, Err: err, When: time.Now()})
		} else if e != nil {
			slog.Debug(fmt.Sprintf("[supervisor] socket->stdout error: %v\r\n", e))
			trySendEvent(sr.events, SupervisorRunnerEvent{ID: sr.session.Id, Type: EvError, Err: err, When: time.Now()})
		}

		errCh <- e
	}()

	// Force resize
	syscall.Kill(syscall.Getpid(), syscall.SIGWINCH)

	go func() error {
		// Wait for either context cancel or one side finishing
		select {
		case <-sr.ctx.Done():
			slog.Debug("[supervisor-runner] context done\r\n")
			_ = conn.Close() // unblock goroutines
			<-errCh
			<-errCh
			return sr.ctx.Err()
		case e := <-errCh:
			// one direction ended; close and wait for the other
			_ = conn.Close()
			<-errCh
			// treat EOF as normal detach
			if e == io.EOF || e == nil {
				return nil
			}
			return e
		}
	}()

	return nil
}
