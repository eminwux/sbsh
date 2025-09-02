package controller

import (
	"context"
	"errors"
	"log"
	"os"
	"sbsh/pkg/session"
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

type Controller struct {
	ready  chan struct{}
	mgr    *session.SessionManager
	events chan session.SessionEvent // fan-in from all sessions
	// resizeSig chan os.Signal

	uiMode  UIMode
	boundID session.SessionID // session currently bound to supervisor UI
	// you can add more, e.g., quietOutput bool
	ctx           context.Context
	lastTermState *term.State
	exit          chan struct{}
}

func (c *Controller) GetSessionEventChannel() chan<- session.SessionEvent {
	return c.events
}

func (c *Controller) GetSessionManager() *session.SessionManager {
	return c.mgr
}

func (c *Controller) WaitReady(ctx context.Context) error {
	select {
	case <-c.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run is the main orchestration loop. It owns all mode transitions.
func (c *Controller) Run(ctx context.Context) error {
	c.ctx = ctx
	c.exit = make(chan struct{})
	log.Println("[ctrl] Starting controller loop")
	defer log.Printf("[ctrl] controller stopped\r\n")

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

func (c *Controller) handleEvent(ev session.SessionEvent) {
	// log.Printf("[ctrl] session %s event received %d\r\n", ev.ID, ev.Type)
	switch ev.Type {
	case session.EvSentinel:
		// log.Printf("[ctrl] session %s EvSentinel\r\n", ev.ID)
		c.onSentinel(ev.ID)

	case session.EvClosed:
		log.Printf("[ctrl] session %s EvClosed error: %v\r\n", ev.ID, ev.Err)
		c.onClosed(ev.ID, ev.Err)

	case session.EvError:
		// log.Printf("[ctrl] session %s EvError error: %v\r\n", ev.ID, ev.Err)

	case session.EvData:
		// optional metrics hook

	}
}

func (c *Controller) onSentinel(id session.SessionID) {
	// Ignore if already in supervisor for this session
	if c.uiMode == UISupervisor && c.boundID == id {
		return
	}

	sess, ok := c.mgr.Get(id)
	if !ok {
		log.Printf("[ctrl] EvSentinel from unknown session %s", id)
		return
	}

	// Flip per-session gates/state
	sess.EnterSupervisor()
	sess.CloseStdinGate() // keystrokes stop going to PTY
	// Decide output policy during supervisor (stream or pause)
	sess.SetOutput(true)

	// Bind UI to this session and switch terminal to cooked
	c.boundID = id
	if err := c.toSupervisorUIMode(); err != nil {
		log.Printf("[ctrl] toSupervisorUIMode failed: %v", err)
	}

	// Start REPL bound to this session (blocking until user exits)
	c.runSupervisorREPL(sess)

	// When REPL exits, go back to bash UI for the same (or current) session
	c.onExitSupervisor(sess.ID())
}

func (c *Controller) onExitSupervisor(id session.SessionID) {
	sess, ok := c.mgr.Get(id)
	if ok {
		sess.ExitSupervisor()
		sess.OpenStdinGate()
		sess.SetOutput(true) // keep streaming in bash
	}

	// Switch back to raw terminal and bash UI
	if err := c.toBashUIMode(); err != nil {
		log.Printf("[ctrl] toBashUIMode failed: %v", err)
	}

	c.uiMode = UIBash
	c.boundID = c.mgr.Current()
}

func (c *Controller) onClosed(id session.SessionID, err error) {
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

	// Session is Closed now

	// If the closed session was bound or current, pick another
	if c.boundID == id {
		c.boundID = ""
		c.toExitShell()
	}

	if c.mgr.Current() == id {
		next := pickNext(c.mgr, id)

		c.mgr.SetCurrent(next)
	}

	// If no sessions remain and we’re in supervisor, drop back to bash UI
	if len(c.mgr.ListLive()) == 0 {
		close(c.exit)
	}
}

func (c *Controller) Stop() {

}

// func (c *Controller) handleResize() {
// 	// Forward current terminal size to the *current* session
// 	curID := c.mgr.Current()
// 	if curID == "" {
// 		return
// 	}
// 	if sess, ok := c.mgr.Get(curID); ok {
// 		_ = sess.Resize(os.Stdin)
// 	}
// }

/* ---------- UI mode transitions (terminal modes) ---------- */

func toRawMode() (*term.State, error) {
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("[ctrl] MakeRaw terminal: %v", err)

	}

	return state, nil
}

// toBashUIMode: set terminal to RAW, update flags
func (c *Controller) toBashUIMode() error {
	// TODO: restore raw mode on os.Stdin (your terminal manager)
	// e.g., term.MakeRaw / term.Restore handled by a helper
	// Put sbsh terminal into raw mode so ^C (0x03) is passed through

	lastTermState, err := toRawMode()
	if err != nil {
		log.Fatalf("MakeRaw: %v", err)
	}
	// defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	c.uiMode = UIBash
	c.lastTermState = lastTermState
	return nil
}

// toSupervisorUIMode: set terminal to COOKED for your REPL
func (c *Controller) toSupervisorUIMode() error {
	// TODO: restore cooked mode on os.Stdin
	// Put sbsh terminal into raw mode so ^C (0x03) is passed through
	err := term.Restore(int(os.Stdin.Fd()), nil)
	if err != nil {
		log.Fatalf("MakeRaw: %v", err)
	}

	c.uiMode = UISupervisor
	return nil
}

// toSupervisorUIMode: set terminal to COOKED for your REPL
func (c *Controller) toExitShell() error {
	// TODO: restore cooked mode on os.Stdin
	// Put sbsh terminal into raw mode so ^C (0x03) is passed through
	err := term.Restore(int(os.Stdin.Fd()), c.lastTermState)
	if err != nil {
		log.Fatalf("MakeRaw: %v", err)
	}

	c.uiMode = UIExitShell
	return nil
}

/* ---------- Supervisor REPL (placeholder) ---------- */

func (c *Controller) runSupervisorREPL(sess session.Session) {
	// TODO: implement your minimal REPL loop (readline in cooked mode)
	// - show "(sbsh) %"
	// - accept 'exit' to return
	// - later: integrate planner/executor
}

func (c *Controller) AddSession(s *session.Session) {
	c.mgr.Add(*s)
}

func (c *Controller) SetCurrentSession(id session.SessionID) error {
	if err := c.mgr.SetCurrent(id); err != nil {
		log.Fatalf("failed to set current session: %v", err)
		return err
	}

	c.boundID = id

	// Initial terminal mode (bash passthrough)
	if err := c.toBashUIMode(); err != nil {
		log.Printf("[ctrl] initial raw mode failed: %v", err)
	}
	return nil
}

func (c *Controller) StartSession(id session.SessionID) error {

	if err := c.mgr.StartSession(id, c.ctx, c.events); err != nil {
		log.Fatalf("failed to start session: %v", err)
		return err
	}

	return nil
}

// NewController wires the manager and the shared event channel from sessions.
func NewController() *Controller {
	log.Printf("[ctrl] New controller is being created\r\n")

	events := make(chan session.SessionEvent, 32) // buffered so PTY readers never block
	// Create a SessionManager
	mgr := session.NewSessionManager()

	c := &Controller{
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

/* ---------- Helpers ---------- */

func pickNext(m *session.SessionManager, closed session.SessionID) session.SessionID {
	live := m.ListLive()
	for _, id := range live {
		if id != closed {
			return id
		}
	}
	return ""
}
