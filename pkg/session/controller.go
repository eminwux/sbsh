package session

import (
	"context"
	"fmt"
	"log"
	"sbsh/pkg/api"
	"sbsh/pkg/session/sessionrpc"
	"sbsh/pkg/session/sessionrunner"
	"time"
)

type SessionController struct {
	ctx context.Context

	closedCh  chan struct{}
	closingCh chan error
	cancel    context.CancelFunc
}

var newSessionRunner = sessionrunner.NewSessionRunnerExec
var sr sessionrunner.SessionRunner
var rpcReadyCh chan error = make(chan error)
var rpcDoneCh chan error = make(chan error)
var ctrlReady chan struct{} = make(chan struct{})
var eventsCh chan sessionrunner.SessionRunnerEvent = make(chan sessionrunner.SessionRunnerEvent, 32)
var closeReqCh chan error = make(chan error, 1)

// NewSessionController wires the manager and the shared event channel from sessions.
func NewSessionController(ctx context.Context, cancel context.CancelFunc) api.SessionController {
	log.Printf("[sessionCtrl] New controller is being created\r\n")

	c := &SessionController{
		ctx:       ctx,
		cancel:    cancel,
		closedCh:  make(chan struct{}),
		closingCh: make(chan error, 1),
	}

	return c
}

func (c *SessionController) Status() string {
	return "RUNNING"
}

func (c *SessionController) WaitReady() error {
	select {
	case <-ctrlReady:
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

func (c *SessionController) WaitClose() error {

	select {
	case <-c.closedCh:
		log.Printf("[sessionCtrl] controller exited")
		return nil
	case err := <-c.closingCh:
		log.Printf("[sessionCtrl] controller closing: %v", err)
	}
	return nil
}

// Run is the main orchestration loop. It owns all mode transitions.
func (c *SessionController) Run(spec *api.SessionSpec) error {

	defer log.Printf("[sessionCtrl] controller stopped\r\n")

	sr = newSessionRunner(spec)

	if len(spec.Command) == 0 {
		log.Printf("empty command in SessionSpec")
		return ErrSpecCmdMissing
	}

	log.Println("[sessionCtrl] Starting controller loop")

	_, err := sr.OpenSocketCtrl()
	if err != nil {
		log.Printf("could not open control socket: %v", err)
		if err := c.Close(err); err != nil {
			return fmt.Errorf("%w:%w", ErrOnClose, err)
		}
		return fmt.Errorf("%w:%w", ErrOpenSocketCtrl, err)
	}

	// c.listenerCtrl = ctrlLn

	rpc := &sessionrpc.SessionControllerRPC{Core: c}
	go sr.StartServer(c.ctx, rpc, rpcReadyCh, rpcDoneCh)
	// Wait for startup result
	if err := <-rpcReadyCh; err != nil {
		// failed to start — handle and return
		log.Printf("failed to start server: %v", err)
		return fmt.Errorf("%w:%w", ErrStartRPCServer, err)
	}

	if err := sr.StartSession(c.ctx, eventsCh); err != nil {
		log.Printf("failed to start session: %v", err)
		if err := c.Close(err); err != nil {
			return fmt.Errorf("%w:%w", ErrOnClose, err)
		}
		return fmt.Errorf("%w:%w", ErrStartSession, err)
	}

	close(ctrlReady)

	for {
		select {
		case <-c.ctx.Done():
			var err error
			log.Printf("[supervisor] parent context channel has been closed\r\n")
			if errC := c.Close(c.ctx.Err()); errC != nil {
				err = fmt.Errorf("%w:%w", ErrOnClose, errC)
			}
			return fmt.Errorf("%w:%w", ErrContextDone, err)

		case ev := <-eventsCh:
			log.Printf("[sessionCtrl] received event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano))
			c.handleEvent(ev)

		case err := <-rpcDoneCh:
			log.Printf("[sessionCtrl] rpc server has exited: %v\r\n", err)
			if err := c.Close(err); err != nil {
				return fmt.Errorf("%w:%w", ErrOnClose, err)
			}
			return fmt.Errorf("%w:%w", ErrRPCServerExited, c.ctx.Err())

		case err := <-rpcDoneCh:
			log.Printf("[sessionCtrl] rpc server has failed: %v\r\n", err)
			if err := c.Close(err); err != nil {
				err = fmt.Errorf("%w:%w:%w", err, ErrOnClose, err)
			}
			return fmt.Errorf("%w:%w", ErrRPCServerExited, err)

		case err := <-closeReqCh:
			log.Printf("[sessionCtrl] close request received: %v\r\n", err)
			return fmt.Errorf("%w:%w", ErrCloseReq, err)
		}
	}

}

/* ---------- Event handlers ---------- */

func (c *SessionController) handleEvent(ev sessionrunner.SessionRunnerEvent) {
	switch ev.Type {

	case sessionrunner.EvError:
		log.Printf("[sessionCtrl] session %s EvError error: %v\r\n", ev.ID, ev.Err)

	case sessionrunner.EvCmdExited:
		log.Printf("[sessionCtrl] session %s EvSessionExited error: %v\r\n", ev.ID, ev.Err)
		c.onClosed(ev.Err)
	}
}

func (c *SessionController) Close(reason error) error {

	log.Printf("[session] Close called: %v\r\n", reason)
	c.closingCh <- reason

	log.Printf("[session] sent close order to session-runner, reason: %v\r\n", reason)
	sr.Close(reason)

	closeReqCh <- reason
	log.Printf("[session] error sent to closingCh, reason: %v\r\n", reason)

	close(c.closedCh)

	return nil
}

func (c *SessionController) onClosed(err error) {
	log.Printf("[sessionCtrl] onClosed triggered\r\n")
	c.Close(err)

}

func (c *SessionController) Resize(args api.ResizeArgs) {

	sr.Resize(args)

}
