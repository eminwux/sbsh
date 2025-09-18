package session

import (
	"context"
	"errors"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"path/filepath"
	"sbsh/pkg/api"
	"sbsh/pkg/session/sessionrpc"
	"sbsh/pkg/session/sessionrunner"
	"time"
)

type SessionController struct {
	ready  chan struct{}
	events chan api.SessionEvent // fan-in from all sessions
	ctx    context.Context

	close   chan error
	closing chan struct{}

	socketCtrl   string
	listenerCtrl net.Listener
}

// var newSessionRunner = sessionrunner.NewSessionRunnerExec
var newSessionRunner func(spec *api.SessionSpec) sessionrunner.SessionRunner
var sr sessionrunner.SessionRunner

// NewSessionController wires the manager and the shared event channel from sessions.
func NewSessionController(ctx context.Context, exit chan error) api.SessionController {
	log.Printf("[sessionCtrl] New controller is being created\r\n")

	events := make(chan api.SessionEvent, 32)

	c := &SessionController{
		ready:   make(chan struct{}),
		events:  events,
		close:   exit,
		ctx:     ctx,
		closing: make(chan struct{}, 1),
	}

	return c
}

func (c *SessionController) Status() string {
	return "RUNNING"
}

func (c *SessionController) WaitReady() error {
	select {
	case <-c.ready:
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

func (c *SessionController) WaitClose() {

	select {
	case <-c.close:
		log.Printf("[sessionCtrl] controller exited")
		return
	case <-c.closing:
		log.Printf("[sessionCtrl] controller closing")
	}
}

// Run is the main orchestration loop. It owns all mode transitions.
func (c *SessionController) Run() {

	defer log.Printf("[sessionCtrl] controller stopped\r\n")

	log.Println("[sessionCtrl] Starting controller loop")

	if sr == nil {
		log.Printf("no session added")
		return
	}

	ctrlLn, err := sr.OpenSocketCtrl()
	if err != nil {
		log.Printf("could not open control socket: %v", err)
		c.Close()
		return
	}
	c.listenerCtrl = ctrlLn

	rpc := &sessionrpc.SessionControllerRPC{Core: c}
	readyCh := make(chan error)
	rpcDoneCh := make(chan error)
	go sr.StartServer(c.ctx, c.listenerCtrl, rpc, readyCh, rpcDoneCh)
	// Wait for startup result
	if err := <-readyCh; err != nil {
		// failed to start — handle and return
		log.Printf("failed to start server: %v", err)
		return
	}

	if err := sr.StartSession(c.ctx, c.events); err != nil {
		log.Printf("failed to start session: %v", err)
		c.Close()
		return
	}

	close(c.ready)

	for {
		select {
		case <-c.ctx.Done():
			log.Printf("[sessionCtrl] parent context channel has been closed\r\n")
			c.Close()
			return

		case ev := <-c.events:
			log.Printf("[sessionCtrl] received event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano))
			c.handleEvent(ev)

		case err := <-rpcDoneCh:
			log.Printf("[sessionCtrl] rpc server has failed: %v\r\n", err)
			c.Close()
			return

		}
	}

}

/* ---------- Event handlers ---------- */

func (c *SessionController) handleEvent(ev api.SessionEvent) {
	switch ev.Type {
	case api.EvClosed:
		log.Printf("[sessionCtrl] session %s EvClosed error: %v\r\n", ev.ID, ev.Err)
		c.onClosed(ev.Err)

	case api.EvError:
		// log.Printf("[sessionCtrl] session %s EvError error: %v\r\n", ev.ID, ev.Err)

	case api.EvData:
		// optional metrics hook

	case api.EvSessionExited:
		log.Printf("[sessionCtrl] session %s EvSessionExited error: %v\r\n", ev.ID, ev.Err)
		// c.onClosed(ev.Err)
	}
}

func (c *SessionController) Close() error {
	c.closing <- struct{}{}

	// remove Ctrl socket
	if err := os.Remove(c.socketCtrl); err != nil {
		log.Printf("[sessionCtrl] couldn't remove Ctrl socket %s: %v\r\n", c.socketCtrl, err)
	}

	// remove whole session dir
	dir := filepath.Dir(c.socketCtrl)
	if err := os.RemoveAll(dir); err != nil {
		log.Printf("[sessionCtrl] couldn't remove directory: %s, error: %v\r\n", dir, err)
	}

	close(c.close)

	return nil
}

func (c *SessionController) onClosed(err error) {

	if err != nil {
		log.Printf("[sessionCtrl] session %s closed with error: %v\r\n", sr.ID(), err)
	} else {
		log.Printf("[sessionCtrl] session %s closed with unknown error\r\n", sr.ID())
	}

	sr.Close()

	c.Close()

}

func (c *SessionController) StartServer() error {

	// Start the Session Socket CTRL Loop
	go func() {
		srv := rpc.NewServer()
		_ = srv.RegisterName("SessionController", &sessionrpc.SessionControllerRPC{Core: c})
		for {
			conn, err := c.listenerCtrl.Accept()
			if err != nil {
				// listener closed -> exit loop
				if _, ok := err.(net.Error); ok {
					continue
				}
				return
			}
			go srv.ServeCodec(jsonrpc.NewServerCodec(conn))
		}
	}()
	// TODO implement channel to close server
	return nil
}

func (s *SessionController) AddSession(spec *api.SessionSpec) error {
	sr = newSessionRunner(spec)

	if len(spec.Command) == 0 {
		return errors.New("empty command in SessionSpec")
	}

	return nil
}

func (c *SessionController) Resize(args api.ResizeArgs) {

	sr.Resize(args)

}
