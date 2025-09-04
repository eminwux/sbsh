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
	"syscall"
	"time"

	"golang.org/x/term"
)

/* ---------- Controller ---------- */

type UIMode int

const (
	UIBash UIMode = iota
	UISupervisor
	UIExitShell // Saved lastState restore
)

type SessionController struct {
	ready  chan struct{}
	mgr    *SessionManager
	events chan api.SessionEvent // fan-in from all sessions
	// resizeSig chan os.Signal

	uiMode  UIMode
	boundID api.SessionID // session currently bound to supervisor UI
	// you can add more, e.g., quietOutput bool
	ctx           context.Context
	lastTermState *term.State
	exit          chan struct{}
}

// type Controller interface {
//     Run(ctx context.Context) error
//     // high-level ops (daemon API):
//     SessionsList(ctx) ([]SessionInfo, error)
//     SessionsNew(ctx, spec SessionSpec) (SessionID, error)
//     SessionsUse(ctx, id SessionID) error
//     SessionsKill(ctx, id SessionID) error
//     // stream (optional):
//     Events(ctx context.Context) (<-chan SessionEvent, error)
// }
////////////////////////////////////////////////

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
	log.Println("[ctrl] Starting controller loop")
	defer log.Printf("[ctrl] controller stopped\r\n")

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	sockPath := filepath.Join(home, ".sbsh", "socket")

	// ensure directory exists
	if err := os.MkdirAll(filepath.Dir(sockPath), 0700); err != nil {
		log.Fatal(err)
	}

	// remove stale socket if it exists
	if _, err := os.Stat(sockPath); err == nil {
		_ = os.Remove(sockPath)
	}
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		log.Fatal(err)
	}

	srv := &sessionRPC{Core: *c} // your real impl
	rpc.RegisterName("Controller", srv)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				continue
			}
			go rpc.ServeCodec(jsonrpc.NewServerCodec(conn))
		}
	}()

	close(c.ready)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[ctrl] Context channel has been closed\r\n")
			_ = term.Restore(int(os.Stdin.Fd()), c.lastTermState)
			return ctx.Err()

		case ev := <-c.events:
			// log.Printf("[ctrl] SessionEvent has been received\r\n")
			log.Printf("[ctrl] received event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano))
			c.handleEvent(ev)

		case <-c.exit:
			log.Printf("[ctrl] received exit event\r\n")
			return nil
			// case <-c.resizeSig:
			// 	log.Printf("[ctrl] Resize event has been received\r\n")
			// 	c.handleResize()

			// (optional) add tickers/timeouts here
		}
	}
}

/* ---------- Event handlers ---------- */

func (c *SessionController) handleEvent(ev api.SessionEvent) {
	// log.Printf("[ctrl] session %s event received %d\r\n", ev.ID, ev.Type)
	switch ev.Type {
	case api.EvClosed:
		log.Printf("[ctrl] session %s EvClosed error: %v\r\n", ev.ID, ev.Err)
		c.onClosed(ev.ID, ev.Err)

	case api.EvError:
		// log.Printf("[ctrl] session %s EvError error: %v\r\n", ev.ID, ev.Err)

	case api.EvData:
		// optional metrics hook

	}
}

func (c *SessionController) onClosed(id api.SessionID, err error) {
	// Treat EIO/EOF as normal close
	if err != nil && !errors.Is(err, syscall.EIO) && !errors.Is(err, os.ErrClosed) {
		log.Printf("[ctrl] session %s closed with error: %v\r\n", id, err)
	}

	if sess, ok := c.mgr.Get(id); ok {
		if err = c.mgr.StopSession(sess.ID()); err != nil {
			log.Println("[ctrl] error closing the session:", err)
			return
		}

	}

}

func (c *SessionController) StartSession(spec *api.SessionSpec) error {
	sess := NewSession(spec)
	c.mgr.Add(*sess)

	if err := c.mgr.StartSession(spec.ID, c.ctx, c.events); err != nil {
		log.Fatalf("failed to start session: %v", err)
		return err
	}

	return nil
}

// NewController wires the manager and the shared event channel from sessions.
func NewController() *SessionController {
	log.Printf("[ctrl] New controller is being created\r\n")

	events := make(chan api.SessionEvent, 32) // buffered so PTY readers never block
	// Create a session.SessionManager
	mgr := NewSessionManager()

	c := &SessionController{
		ready:  make(chan struct{}),
		mgr:    mgr,
		events: events,
		// resizeSig: make(chan os.Signal, 1),
		uiMode:  UIBash,
		boundID: mgr.Current(),
	}
	// signal.Notify(c.resizeSig, syscall.SIGWINCH)
	return c
}
