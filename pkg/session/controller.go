package session

import (
	"context"
	"fmt"
	"log/slog"
	"sbsh/pkg/api"
	"sbsh/pkg/errdefs"
	"sbsh/pkg/session/sessionrpc"
	"sbsh/pkg/session/sessionrunner"
	"time"
)

type SessionController struct {
	ctx context.Context

	closedCh chan struct{}
	cancel   context.CancelFunc
}

var newSessionRunner = sessionrunner.NewSessionRunnerExec
var sr sessionrunner.SessionRunner
var rpcReadyCh chan error = make(chan error)
var rpcDoneCh chan error = make(chan error)
var ctrlReady chan struct{} = make(chan struct{}, 1)
var eventsCh chan sessionrunner.SessionRunnerEvent = make(chan sessionrunner.SessionRunnerEvent, 32)
var closeReqCh chan error = make(chan error, 1)

// NewSessionController wires the manager and the shared event channel from sessions.
func NewSessionController(ctx context.Context, cancel context.CancelFunc) api.SessionController {
	slog.Debug("[sessionCtrl] New controller is being created\r\n")

	c := &SessionController{
		ctx:      ctx,
		cancel:   cancel,
		closedCh: make(chan struct{}),
	}

	return c
}

func (c *SessionController) Status() string {
	return "RUNNING"
}

func (c *SessionController) WaitReady() error {
	select {
	case <-ctrlReady:
		close(ctrlReady)
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

func (c *SessionController) WaitClose() error {

	select {
	case <-c.closedCh:
		slog.Debug("[sessionCtrl] controller exited")
		return nil
	}
}

// Run is the main orchestration loop. It owns all mode transitions.
func (c *SessionController) Run(spec *api.SessionSpec) error {

	defer slog.Debug("[sessionCtrl] controller stopped\r\n")

	sr = newSessionRunner(c.ctx, spec)

	if len(spec.Command) == 0 {
		slog.Debug("empty command in SessionSpec")
		return errdefs.ErrSpecCmdMissing
	}

	slog.Debug("[sessionCtrl] Starting controller loop")

	err := sr.CreateMetadata()
	if err != nil {
		slog.Debug(fmt.Sprintf("could not write metadata file: %v", err))
		if err := c.Close(err); err != nil {
			err = fmt.Errorf("%w:%w", errdefs.ErrOnClose, err)
		}
		return fmt.Errorf("%w:%w", errdefs.ErrWriteMetadata, err)
	}

	err = sr.OpenSocketCtrl()
	if err != nil {
		slog.Debug(fmt.Sprintf("could not open control socket: %v", err))
		if err := c.Close(err); err != nil {
			err = fmt.Errorf("%w:%w", errdefs.ErrOnClose, err)
		}
		return fmt.Errorf("%w:%w", errdefs.ErrOpenSocketCtrl, err)
	}

	rpc := &sessionrpc.SessionControllerRPC{Core: c}
	go sr.StartServer(rpc, rpcReadyCh)
	// Wait for startup result
	if err := <-rpcReadyCh; err != nil {
		// failed to start — handle and return
		slog.Debug(fmt.Sprintf("failed to start server: %v", err))
		return fmt.Errorf("%w:%w", errdefs.ErrStartRPCServer, err)
	}

	if err := sr.StartSession(eventsCh); err != nil {
		slog.Debug(fmt.Sprintf("failed to start session: %v", err))
		if err := c.Close(err); err != nil {
			err = fmt.Errorf("%w:%w", errdefs.ErrOnClose, err)
		}
		return fmt.Errorf("%w:%w", errdefs.ErrStartSession, err)
	}

	ctrlReady <- struct{}{}

	for {
		select {
		case <-c.ctx.Done():
			var err error
			slog.Debug("[supervisor] parent context channel has been closed\r\n")
			if errC := c.Close(c.ctx.Err()); errC != nil {
				err = fmt.Errorf("%w:%w", errdefs.ErrOnClose, errC)
			}
			return fmt.Errorf("%w:%w", errdefs.ErrContextDone, err)

		case ev := <-eventsCh:
			slog.Debug(fmt.Sprintf("[sessionCtrl] received event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano)))
			c.handleEvent(ev)

		case err := <-rpcDoneCh:
			slog.Debug(fmt.Sprintf("[sessionCtrl] rpc server has failed: %v\r\n", err))
			if err := c.Close(err); err != nil {
				err = fmt.Errorf("%w:%w:%w", err, errdefs.ErrOnClose, err)
			}
			return fmt.Errorf("%w:%w", errdefs.ErrRPCServerExited, err)

		case err := <-closeReqCh:
			slog.Debug(fmt.Sprintf("[sessionCtrl] close request received: %v\r\n", err))
			return fmt.Errorf("%w:%w", errdefs.ErrCloseReq, err)
		}
	}

}

/* ---------- Event handlers ---------- */

func (c *SessionController) handleEvent(ev sessionrunner.SessionRunnerEvent) {
	switch ev.Type {

	case sessionrunner.EvError:
		slog.Debug(fmt.Sprintf("[sessionCtrl] session %s EvError error: %v\r\n", ev.ID, ev.Err))

	case sessionrunner.EvCmdExited:
		slog.Debug(fmt.Sprintf("[sessionCtrl] session %s EvSessionExited error: %v\r\n", ev.ID, ev.Err))
		c.onClosed(ev.Err)
	}
}

func (c *SessionController) Close(reason error) error {

	slog.Debug(fmt.Sprintf("[session] Close called: %v\r\n", reason))

	slog.Debug(fmt.Sprintf("[session] sent close order to session-runner, reason: %v\r\n", reason))
	sr.Close(reason)

	closeReqCh <- reason
	slog.Debug(fmt.Sprintf("[session] error sent to closingCh, reason: %v\r\n", reason))

	close(c.closedCh)

	return nil
}

func (c *SessionController) onClosed(err error) {
	slog.Debug("[sessionCtrl] onClosed triggered\r\n")
	c.Close(err)

}

func (c *SessionController) Resize(args api.ResizeArgs) {

	sr.Resize(args)

}

func (c *SessionController) Detach() error {

	return sr.Detach()

}
