package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sbsh/pkg/api"
	"sbsh/pkg/session"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

/* ---------- Controller ---------- */

type UIMode int

const (
	UIBash UIMode = iota
	UISupervisor
	UIExitShell // Saved lastState restore
)

type SupervisorController struct {
	ready  chan struct{}
	mgr    *session.SessionManager
	events chan api.SessionEvent // fan-in from all sessions
	// resizeSig chan os.Signal

	uiMode  UIMode
	boundID api.SessionID // session currently bound to supervisor UI
	// you can add more, e.g., quietOutput bool
	ctx           context.Context
	lastTermState *term.State
	exit          chan struct{}

	sessionClientRPC *rpc.Client
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

func (c *SupervisorController) WaitReady(ctx context.Context) error {
	select {
	case <-c.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run is the main orchestration loop. It owns all mode transitions.
func (c *SupervisorController) Run(ctx context.Context) error {
	c.ctx = ctx
	c.exit = make(chan struct{})
	log.Println("[supervisor] Starting controller loop")
	defer log.Printf("[supervisor] controller stopped\r\n")

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

	srv := &SupervisorControllerRPC{Core: *c} // your real impl
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
			log.Printf("[supervisor] Context channel has been closed\r\n")
			_ = term.Restore(int(os.Stdin.Fd()), c.lastTermState)
			return ctx.Err()

		case ev := <-c.events:
			// log.Printf("[supervisor] SessionEvent has been received\r\n")
			log.Printf("[supervisor] received event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano))
			c.handleEvent(ev)

		case <-c.exit:
			log.Printf("[supervisor] received exit event\r\n")
			return nil
			// case <-c.resizeSig:
			// 	log.Printf("[supervisor] Resize event has been received\r\n")
			// 	c.handleResize()

			// (optional) add tickers/timeouts here
		}
	}
}

/* ---------- Event handlers ---------- */

func (c *SupervisorController) handleEvent(ev api.SessionEvent) {
	// log.Printf("[supervisor] session %s event received %d\r\n", ev.ID, ev.Type)
	switch ev.Type {
	case api.EvClosed:
		log.Printf("[supervisor] session %s EvClosed error: %v\r\n", ev.ID, ev.Err)
		c.onClosed(ev.ID, ev.Err)

	case api.EvError:
		// log.Printf("[supervisor] session %s EvError error: %v\r\n", ev.ID, ev.Err)

	case api.EvData:
		// optional metrics hook

	}
}

func (c *SupervisorController) onClosed(id api.SessionID, err error) {
	// Treat EIO/EOF as normal close
	if err != nil && !errors.Is(err, syscall.EIO) && !errors.Is(err, os.ErrClosed) {
		log.Printf("[supervisor] session %s closed with error: %v\r\n", id, err)
	}

	if sess, ok := c.mgr.Get(id); ok {
		if err = c.mgr.StopSession(sess.ID()); err != nil {
			log.Println("[supervisor] error closing the session:", err)
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

/* ---------- UI mode transitions (terminal modes) ---------- */

func toRawMode() (*term.State, error) {
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("[supervisor] MakeRaw terminal: %v", err)

	}

	return state, nil
}

// toBashUIMode: set terminal to RAW, update flags
func (c *SupervisorController) toBashUIMode() error {
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
func (c *SupervisorController) toExitShell() error {
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

func (c *SupervisorController) AddSession(spec *api.SessionSpec) {
	// Create the new Session
	sess := session.NewSession(spec)
	c.mgr.Add(sess)
}

func (c *SupervisorController) SetCurrentSession(id api.SessionID) error {
	if err := c.mgr.SetCurrent(id); err != nil {
		log.Fatalf("failed to set current session: %v", err)
		return err
	}

	c.boundID = id

	// Initial terminal mode (bash passthrough)
	if err := c.toBashUIMode(); err != nil {
		log.Printf("[supervisor] initial raw mode failed: %v", err)
	}
	return nil
}

func (c *SupervisorController) Start() error {

	socketCTRL := "/home/inwx/.sbsh/run/sessions/s0/ctrl.sock"
	socketIO := "/home/inwx/.sbsh/run/sessions/s0/io.sock"

	exe := "/home/inwx/projects/sbsh/sbsh-session"
	args := []string{}

	// point stdio away from your TTY
	devNull, _ := os.OpenFile("/dev/null", os.O_RDWR, 0)
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from your pg/ctty
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, devNull, devNull
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn session: %w", err)
	}

	// IMPORTANT: reap it in the background so it never zombifies
	go func() { _ = cmd.Wait() }()

	// you can return cmd.Process.Pid to record in meta.json
	_ = cmd.Process.Pid

	c.sessionClientRPC = dialSessionCtrlSocket(socketCTRL)
	attachAndForwardResize(c.ctx, c.sessionClientRPC)
	attachIOSocket(c.ctx, socketIO)

	return nil
}

func dialSessionCtrlSocket(ctrlSock string) *rpc.Client {

	var conn net.Conn
	var err error

	// Dial the Unix domain socket
	for range 3 {
		conn, err = net.Dial("unix", ctrlSock)
		if err == nil {
			break // success
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err != nil {
		log.Fatalf("failed to connect to ctrl.sock after 3 retries: %v", err)
	}

	// defer conn.Close()

	// Wrap the connection in a JSON-RPC client
	client := rpc.NewClientWithCodec(jsonrpc.NewClientCodec(conn))

	// Example: call SessionController.Status (no args, returns SessionStatus)
	var info api.SessionStatus

	err = client.Call("SessionController.Status", struct{}{}, &info)
	if err != nil {
		log.Fatalf("RPC call failed: %v", err)
	}

	log.Printf("Session info: %+v\r\n", info)

	// Example: resize call
	// resizeArgs := api.ResizeArgs{Cols: 120, Rows: 40}
	// var reply api.Empty
	// err = client.Call("Session.Resize", resizeArgs, &reply)
	// if err != nil {
	// 	log.Fatalf("Resize failed: %v", err)
	// }
	// fmt.Println("Resize OK")
	return client

}

func attachIOSocket(ctx context.Context, ioSockPath string) error {

	var conn net.Conn
	var err error

	// Dial the Unix domain socket
	for range 3 {
		conn, err = net.Dial("unix", ioSockPath)
		if err == nil {
			break // success
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err != nil {
		log.Fatalf("failed to connect to io.sock after 3 retries: %v", err)
	}

	// Ensure we close on any exit path
	defer conn.Close()

	// We want half-closes; UnixConn exposes CloseRead/CloseWrite
	uc, _ := conn.(*net.UnixConn)

	// Put our terminal in raw mode so keystrokes pass through unchanged
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	errCh := make(chan error, 2)

	// stdin -> socket
	go func() {
		_, e := io.Copy(conn, os.Stdin)
		// tell peer we're done sending (but still willing to read)
		if uc != nil {
			_ = uc.CloseWrite()
		}
		errCh <- e
	}()

	// socket -> stdout
	go func() {
		_, e := io.Copy(os.Stdout, conn)
		// we won't read further; let the other goroutine finish
		if uc != nil {
			_ = uc.CloseRead()
		}
		errCh <- e
	}()

	// Force resize
	syscall.Kill(syscall.Getpid(), syscall.SIGWINCH)

	// Wait for either context cancel or one side finishing
	select {
	case <-ctx.Done():
		_ = conn.Close() // unblock goroutines
		<-errCh
		<-errCh
		return ctx.Err()
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
}

func attachAndForwardResize(ctx context.Context, client *rpc.Client) error {
	// Send initial size once (use the supervisor's TTY: os.Stdin)
	if rows, cols, err := pty.Getsize(os.Stdin); err == nil {
		_ = client.Call("Session.Resize",
			api.ResizeArgs{Cols: int(cols), Rows: int(rows)}, &api.Empty{})
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	go func() {
		defer signal.Stop(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				log.Printf("[supervisor] window change\r\n")
				// Query current terminal size again on every WINCH
				rows, cols, err := pty.Getsize(os.Stdin)
				if err != nil {
					// harmless: keep going; terminal may be detached briefly
					continue
				}
				var reply api.Empty
				if err := client.Call("SessionController.Resize",
					api.ResizeArgs{Cols: int(cols), Rows: int(rows)}, &reply); err != nil {
					// Don't kill the process on resize failure; just log
					log.Printf("resize RPC failed: %v\r\n", err)
				}
			}
		}
	}()
	return nil
}

// NewController wires the manager and the shared event channel from sessions.
func NewController() *SupervisorController {
	log.Printf("[supervisor] New controller is being created\r\n")

	events := make(chan api.SessionEvent, 32) // buffered so PTY readers never block
	// Create a session.SessionManager
	mgr := session.NewSessionManager()

	c := &SupervisorController{
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

func pickNext(m *session.SessionManager, closed api.SessionID) api.SessionID {
	live := m.ListLive()
	for _, id := range live {
		if id != closed {
			return id
		}
	}
	return ""
}
