package session

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
	"syscall"
	"time"
)

type SessionController struct {
	ready  chan struct{}
	events chan api.SessionEvent // fan-in from all sessions
	ctx    context.Context
	exit   chan struct{}

	socketCTRL string

	ctrlLn  net.Listener
	session *Session
}

var sessionDir string

// NewSessionController wires the manager and the shared event channel from sessions.
func NewSessionController() *SessionController {
	log.Printf("[sessionCtrl] New controller is being created\r\n")

	events := make(chan api.SessionEvent, 32) // buffered so PTY readers never block

	// mgr := NewSessionManager()

	c := &SessionController{
		ready: make(chan struct{}),
		// mgr:    mgr,
		events: events,
	}
	// signal.Notify(c.resizeSig, syscall.SIGWINCH)
	return c
}

func (c *SessionController) Status() string {
	return "RUNNING"
}

func (c *SessionController) WaitReady(ctx context.Context) error {
	select {
	case <-c.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run is the main orchestration loop. It owns all mode transitions.
func (c *SessionController) Run(ctx context.Context) error {
	c.ctx = ctx
	c.exit = make(chan struct{})
	log.Println("[sessionCtrl] Starting controller loop")
	defer log.Printf("[sessionCtrl] controller stopped\r\n")

	close(c.ready)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[sessionCtrl] Context channel has been closed\r\n")
			return ctx.Err()

		case ev := <-c.events:
			// log.Printf("[sessionCtrl] SessionEvent has been received\r\n")
			log.Printf("[sessionCtrl] received event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano))
			c.handleEvent(ev)

		case <-c.exit:
			log.Printf("[sessionCtrl] received exit event\r\n")
			return nil

		}
	}
}

/* ---------- Event handlers ---------- */

func (c *SessionController) handleEvent(ev api.SessionEvent) {
	// log.Printf("[sessionCtrl] session %s event received %d\r\n", ev.ID, ev.Type)
	switch ev.Type {
	case api.EvClosed:
		log.Printf("[sessionCtrl] session %s EvClosed error: %v\r\n", ev.ID, ev.Err)
		c.onClosed(ev.ID, ev.Err)

	case api.EvError:
		// log.Printf("[sessionCtrl] session %s EvError error: %v\r\n", ev.ID, ev.Err)

	case api.EvData:
		// optional metrics hook

	case api.EvSessionExited:
		log.Printf("[sessionCtrl] session %s EvSessionExited error: %v\r\n", ev.ID, ev.Err)
		close(c.exit)
	}
}

func (c *SessionController) onClosed(id api.SessionID, err error) {
	// Treat EIO/EOF as normal close
	if err != nil && !errors.Is(err, syscall.EIO) && !errors.Is(err, os.ErrClosed) {
		log.Printf("[sessionCtrl] session %s closed with error: %v\r\n", id, err)
	}

	if err = c.session.Close(); err != nil {
		log.Println("[sessionCtrl] error closing the session:", err)
		return
	}

	if c.ctrlLn != nil {
		_ = c.ctrlLn.Close()
	}

}

func (c *SessionController) StartSession(spec *api.SessionSpec) error {

	if len(spec.Command) == 0 {
		return errors.New("empty command in SessionSpec")
	}

	c.session = NewSession(spec)
	// c.mgr.Add(s)

	// Set up sockets
	base, err := common.RuntimeBaseSessions()
	if err != nil {
		return err
	}

	sessionDir = filepath.Join(base, string(spec.ID))
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	c.socketCTRL = filepath.Join(sessionDir, "ctrl.sock")
	log.Printf("[sessionCtrl] CTRL socket: %s", c.socketCTRL)

	// Remove sockets if they already exist
	// remove sockets and dir
	if err := os.Remove(c.socketCTRL); err != nil {
		log.Printf("[sessionCtrl] couldn't remove stale CTRL socket: %s\r\n", c.socketCTRL)
	}

	// Listen to CONTROL SOCKET
	ctrlLn, err := net.Listen("unix", c.socketCTRL)
	if err != nil {
		return fmt.Errorf("listen ctrl: %w", err)
	}
	if err := os.Chmod(c.socketCTRL, 0o600); err != nil {
		ctrlLn.Close()
		return err
	}

	// keep references for Close()
	c.ctrlLn = ctrlLn

	// Start the Session Socket CTRL Loop
	go func() {
		srv := rpc.NewServer()
		_ = srv.RegisterName("SessionController", &SessionControllerRPC{Core: *c})
		for {
			conn, err := ctrlLn.Accept()
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

	if err := c.session.Start(c.ctx, c.events); err != nil {
		log.Fatalf("failed to start session: %v", err)
		return err
	}

	return nil
}

func (c *SessionController) Resize(args api.ResizeArgs) {

	c.session.Resize(args)

}
