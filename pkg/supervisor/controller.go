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
	"sbsh/pkg/common"
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
	mgr    *SessionManager
	events chan api.SessionEvent

	uiMode UIMode

	ctx           context.Context
	lastTermState *term.State
	exit          chan struct{}

	supervisorSockerCtrl string
}

// NewSupervisorController wires the manager and the shared event channel from sessions.
func NewSupervisorController() *SupervisorController {
	log.Printf("[supervisor] New controller is being created\r\n")

	events := make(chan api.SessionEvent, 32) // buffered so PTY readers never block
	// Create a session.SessionManager
	mgr := NewSessionManager()

	c := &SupervisorController{
		ready:  make(chan struct{}),
		mgr:    mgr,
		events: events,
		// resizeSig: make(chan os.Signal, 1),
		uiMode: UIBash,
	}
	// signal.Notify(c.resizeSig, syscall.SIGWINCH)
	return c
}

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

	supervisorID := common.RandomID()

	// Set up sockets
	base, err := common.RuntimeBaseSupervisor()
	if err != nil {
		return err
	}

	supervisorsDir := filepath.Join(base, string(supervisorID))
	if err := os.MkdirAll(supervisorsDir, 0o700); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	c.supervisorSockerCtrl = filepath.Join(supervisorsDir, "ctrl.sock")
	log.Printf("[supervisor] CTRL socket: %s", c.supervisorSockerCtrl)

	// remove stale socket if it exists
	if _, err := os.Stat(c.supervisorSockerCtrl); err == nil {
		_ = os.Remove(c.supervisorSockerCtrl)
	}
	ln, err := net.Listen("unix", c.supervisorSockerCtrl)
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
			log.Printf("[supervisor] received event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano))
			c.handleEvent(ev)

		case <-c.exit:
			log.Printf("[supervisor] received exit event\r\n")
			return nil

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
		log.Printf("[supervisor] session %s EvError error: %v\r\n", ev.ID, ev.Err)
		c.onClosed(ev.ID, ev.Err)

	case api.EvData:
		// optional metrics hook

	case api.EvSessionExited:
		log.Printf("[sessionCtrl] session %s EvSessionExited error: %v\r\n", ev.ID, ev.Err)
		c.toExitShell()
		close(c.exit)

	}
}

func (c *SupervisorController) onClosed(id api.SessionID, err error) {
	// Treat EIO/EOF as normal close
	if err != nil && !errors.Is(err, syscall.EIO) && !errors.Is(err, os.ErrClosed) {
		log.Printf("[supervisor] session %s closed with error: %v\r\n", id, err)
	}

	// if _, ok := c.mgr.Get(id); ok {
	// }

	// If the closed session was bound or current, pick another
	if c.mgr.Current() == id {
		c.mgr.SetCurrent(api.SessionID(""))
		c.mgr.Remove(id)
		c.toExitShell()
	}

	// If no sessions remain and we’re in supervisor, drop back to bash UI
	if len(c.mgr.ListLive()) == 0 {
		// term.Restore(int(os.Stdin.Fd()), c.lastTermState)
		close(c.exit)
	}

	// remove sockets and dir
	if err := os.Remove(c.supervisorSockerCtrl); err != nil {
		log.Printf("[session] couldn't remove IO socket %s: %v\r\n", c.supervisorSockerCtrl, err)
	}
}

// func (c *SupervisorController) AddSession(spec *api.SessionSpec) error {
// 	// Create the new Session
// 	sess := NewSupervisedSession(spec)
// 	c.mgr.Add(sess)
// 	return nil
// }

func (c *SupervisorController) SetCurrentSession(id api.SessionID) error {
	if err := c.mgr.SetCurrent(id); err != nil {
		log.Fatalf("failed to set current session: %v", err)
		return err
	}

	// Initial terminal mode (bash passthrough)
	if err := c.toBashUIMode(); err != nil {
		log.Printf("[supervisor] initial raw mode failed: %v", err)
	}
	return nil
}

func (c *SupervisorController) Start() error {

	sessionID := common.RandomID()

	exe := "/home/inwx/projects/sbsh/sbsh-session"
	args := []string{"--id", sessionID}

	sessionSpec := &api.SessionSpec{
		ID:          api.SessionID(sessionID),
		Kind:        api.SessLocal,
		Label:       "default",
		Command:     exe,
		CommandArgs: args,
		Env:         os.Environ(),
		LogDir:      "/tmp/sbsh-logs/s0",
		SockerCtrl:  "/home/inwx/.sbsh/run/sessions/" + sessionID + "/ctrl.sock",
		SocketIO:    "/home/inwx/.sbsh/run/sessions/" + sessionID + "/io.sock",
	}

	session := NewSupervisedSession(sessionSpec)
	c.mgr.Add(session)
	c.SetCurrentSession(api.SessionID(sessionID))

	// Begins start procedure
	devNull, _ := os.OpenFile("/dev/null", os.O_RDWR, 0)
	cmd := exec.Command(session.command, session.commandArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from your pg/ctty
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, devNull, devNull
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn session: %w", err)
	}

	// you can return cmd.Process.Pid to record in meta.json
	session.pid = cmd.Process.Pid

	// IMPORTANT: reap it in the background so it never zombifies
	go func() {
		_ = cmd.Wait()
		log.Printf("[supervisor] session %s process has exited\r\n", sessionID)
		err := fmt.Errorf("session %s process has exited", sessionID)
		trySendEvent(c.events, api.SessionEvent{ID: api.SessionID(sessionID), Type: api.EvSessionExited, Err: err, When: time.Now()})
	}()

	if err := c.dialSessionCtrlSocket(); err != nil {
		return err
	}

	if err := c.attachAndForwardResize(); err != nil {
		return err
	}

	if err := c.attachIOSocket(); err != nil {
		return err
	}

	return nil
}

func (c *SupervisorController) dialSessionCtrlSocket() error {

	var conn net.Conn
	var err error

	session, ok := c.mgr.Get(api.SessionID(c.mgr.Current()))
	if !ok {
		return errors.New("could not get bound session")
	}

	// Dial the Unix domain socket
	for range 3 {
		conn, err = net.Dial("unix", session.sockerCtrl)
		if err == nil {
			break // success
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err != nil {
		log.Fatalf("failed to connect to ctrl.sock after 3 retries: %v", err)
	}

	// Wrap the connection in a JSON-RPC client
	session.sessionClientRPC = rpc.NewClientWithCodec(jsonrpc.NewClientCodec(conn))

	// Example: call SessionController.Status (no args, returns SessionStatus)
	var info api.SessionStatus

	err = session.sessionClientRPC.Call("SessionController.Status", struct{}{}, &info)
	if err != nil {
		log.Fatalf("RPC call failed: %v", err)
		return err
	}

	log.Printf("[supervisor] rpc->session (Status): %+v\r\n", info)

	return nil

}

func (c *SupervisorController) attachIOSocket() error {

	var conn net.Conn
	var err error

	session, ok := c.mgr.Get(api.SessionID(c.mgr.Current()))
	if !ok {
		return errors.New("could not get bound session")
	}

	// Dial the Unix domain socket
	for range 3 {
		conn, err = net.Dial("unix", session.socketIO)
		if err == nil {
			break // success
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err != nil {
		log.Fatalf("failed to connect to io.sock after 3 retries: %v", err)
		return err
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
			log.Printf("[supervisor] stdin reached EOF\r\n")
			trySendEvent(c.events, api.SessionEvent{ID: session.id, Type: api.EvClosed, Err: err, When: time.Now()})
		} else if e != nil {
			log.Printf("[supervisor] stdin->socket error: %v\r\n", e)
			trySendEvent(c.events, api.SessionEvent{ID: session.id, Type: api.EvError, Err: err, When: time.Now()})
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
			log.Printf("[supervisor] socket closed (EOF)\r\n")
			trySendEvent(c.events, api.SessionEvent{ID: session.id, Type: api.EvClosed, Err: err, When: time.Now()})
		} else if e != nil {
			log.Printf("[supervisor] socket->stdout error: %v\r\n", e)
			trySendEvent(c.events, api.SessionEvent{ID: session.id, Type: api.EvError, Err: err, When: time.Now()})
		}

		errCh <- e
	}()

	// Force resize
	syscall.Kill(syscall.Getpid(), syscall.SIGWINCH)

	go func() error {
		// Wait for either context cancel or one side finishing
		select {
		case <-c.ctx.Done():
			_ = conn.Close() // unblock goroutines
			<-errCh
			<-errCh
			return c.ctx.Err()
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

func (c *SupervisorController) attachAndForwardResize() error {

	session, ok := c.mgr.Get(api.SessionID(c.mgr.Current()))
	if !ok {
		return errors.New("could not get bound session")
	}

	// Send initial size once (use the supervisor's TTY: os.Stdin)
	if rows, cols, err := pty.Getsize(os.Stdin); err == nil {
		_ = session.sessionClientRPC.Call("Session.Resize",
			api.ResizeArgs{Cols: int(cols), Rows: int(rows)}, &api.Empty{})
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	go func() {
		defer signal.Stop(ch)
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-ch:
				// log.Printf("[supervisor] window change\r\n")
				// Query current terminal size again on every WINCH
				rows, cols, err := pty.Getsize(os.Stdin)
				if err != nil {
					// harmless: keep going; terminal may be detached briefly
					continue
				}
				var reply api.Empty
				if err := session.sessionClientRPC.Call("SessionController.Resize",
					api.ResizeArgs{Cols: int(cols), Rows: int(rows)}, &reply); err != nil {
					// Don't kill the process on resize failure; just log
					log.Printf("resize RPC failed: %v\r\n", err)
				}
			}
		}
	}()
	return nil
}

// toBashUIMode: set terminal to RAW, update flags
func (c *SupervisorController) toBashUIMode() error {
	// TODO: restore raw mode on os.Stdin (your terminal manager)
	// e.g., term.MakeRaw / term.Restore handled by a helper
	// Put sbsh terminal into raw mode so ^C (0x03) is passed through

	lastTermState, err := toRawMode()
	if err != nil {
		log.Fatalf("MakeRaw: %v", err)
		return err
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
		return err
	}

	c.uiMode = UIExitShell
	return nil
}

func toRawMode() (*term.State, error) {
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("[supervisor] MakeRaw terminal: %v", err)

	}

	return state, nil
}

// helper: non-blocking event send so the PTY reader never stalls
func trySendEvent(ch chan<- api.SessionEvent, ev api.SessionEvent) {
	log.Printf("[supervisor] send event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano))

	select {
	case ch <- ev:
	default:
		// drop on the floor if controller is momentarily busy; channel should be buffered
	}
}
