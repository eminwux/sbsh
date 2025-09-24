package supervisorrunner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sbsh/pkg/api"
	"sbsh/pkg/errdefs"
	"sbsh/pkg/supervisor/sessionstore"
	"sbsh/pkg/supervisor/supervisorrpc"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

type SupervisorRunner interface {
	OpenSocketCtrl() error
	StartServer(ctx context.Context, sc *supervisorrpc.SupervisorControllerRPC, readyCh chan error, doneCh chan error)
	StartSupervisor(ctx context.Context, evCh chan<- SupervisorRunnerEvent, session *sessionstore.SupervisedSession) error
	ID() api.ID
	Close(reason error) error
	Resize(args api.ResizeArgs)
	SetCurrentSession(id api.ID) error
}

type SupervisorRunnerExec struct {
	id                   api.ID
	spec                 api.SupervisorSpec
	runPath              string
	session              *sessionstore.SupervisedSession
	sessionCtx           context.Context
	uiMode               UIMode
	events               chan<- SupervisorRunnerEvent
	lastTermState        *term.State
	Mgr                  *sessionstore.SessionStoreExec
	supervisorSockerCtrl string
	lnCtrl               net.Listener
}

type SupervisorRunnerEvent struct {
	ID    api.ID
	Type  SupervisorRunnerEventType
	Bytes int   // for EvData
	Err   error // for EvClosed/EvError
	When  time.Time
}

type SupervisorRunnerEventType int

const (
	EvError SupervisorRunnerEventType = iota // abnormal error
	EvCmdExited
)

type UIMode int

const (
	UIBash UIMode = iota
	UISupervisor
	UIExitShell // Saved lastState restore
)

func NewSupervisorRunnerExec(spec *api.SupervisorSpec) SupervisorRunner {
	return &SupervisorRunnerExec{
		sessionCtx: spec.Ctx,
		id:         spec.ID,
		spec:       *spec,
		runPath:    spec.RunPath + "/supervisors",
	}
}

func (s *SupervisorRunnerExec) OpenSocketCtrl() error {

	supervisorsDir := filepath.Join(s.runPath, string(s.id))
	if err := os.MkdirAll(supervisorsDir, 0o700); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	s.supervisorSockerCtrl = filepath.Join(supervisorsDir, "ctrl.sock")
	slog.Debug(fmt.Sprintf("[supervisor] CTRL socket: %s", s.supervisorSockerCtrl))

	// remove stale socket if it exists
	if _, err := os.Stat(s.supervisorSockerCtrl); err == nil {
		_ = os.Remove(s.supervisorSockerCtrl)
	}
	ln, err := net.Listen("unix", s.supervisorSockerCtrl)
	if err != nil {
		log.Fatal(err)
	}

	s.lnCtrl = ln

	return nil
}

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
	if err := srv.RegisterName("SessionController", sc); err != nil {

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

func (s *SupervisorRunnerExec) StartSupervisor(ctx context.Context, evCh chan<- SupervisorRunnerEvent, session *sessionstore.SupervisedSession) error {
	s.events = evCh
	s.session = session

	devNull, _ := os.OpenFile("/dev/null", os.O_RDWR, 0)
	cmd := exec.Command(session.Command, session.CommandArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from your pg/ctty
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, devNull, devNull
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w:%w", errdefs.ErrSessionCmdStart, err)
	}

	// you can return cmd.Process.Pid to record in meta.json
	session.Pid = cmd.Process.Pid

	slog.Debug(fmt.Sprintf("[supervisor] session %s process %d has started\r\n", session.Id, session.Pid))

	// IMPORTANT: reap it in the background so it never zombifies
	go func() {
		_ = cmd.Wait()
		slog.Debug(fmt.Sprintf("[supervisor] session %s process has exited\r\n", session.Id))
		err := fmt.Errorf("session %s process has exited", session.Id)
		trySendEvent(s.events, SupervisorRunnerEvent{ID: api.ID(session.Id), Type: EvCmdExited, Err: err, When: time.Now()})
	}()

	if err := s.dialSessionCtrlSocket(); err != nil {
		return err
	}

	if err := s.attachAndForwardResize(); err != nil {
		return err
	}

	if err := s.attachIOSocket(); err != nil {
		return err
	}

	return nil
}

func (s *SupervisorRunnerExec) ID() api.ID {
	return s.session.Id
}

func (s *SupervisorRunnerExec) Close(reason error) error {
	// remove sockets and dir
	if err := os.Remove(s.supervisorSockerCtrl); err != nil {
		slog.Debug(fmt.Sprintf("[supervisor] couldn't remove Ctrl socket '%s': %v\r\n", s.supervisorSockerCtrl, err))
	}

	if err := os.RemoveAll(filepath.Dir(s.supervisorSockerCtrl)); err != nil {
		slog.Debug(fmt.Sprintf("[supervisor] couldn't remove socket Directory '%s': %v\r\n", s.supervisorSockerCtrl, err))
	}
	s.toExitShell()
	return nil
}

func (s *SupervisorRunnerExec) WaitClose(reason error) error {
	return nil
}

func (s *SupervisorRunnerExec) Resize(args api.ResizeArgs) {
	// No-op
}

func (s *SupervisorRunnerExec) SetCurrentSession(id api.ID) error {
	// Initial terminal mode (bash passthrough)
	if err := s.toBashUIMode(); err != nil {
		slog.Debug(fmt.Sprintf("[supervisor] initial raw mode failed: %v", err))
	}
	return nil
}

func (s *SupervisorRunnerExec) dialSessionCtrlSocket() error {

	var conn net.Conn
	var err error

	slog.Debug(fmt.Sprintf("[supervisor] %s session on  %d trying to connect to %s\r\n", s.session.Id, s.session.Pid, s.session.SockerCtrl))

	// Dial the Unix domain socket
	for range 3 {
		conn, err = net.Dial("unix", s.session.SockerCtrl)
		if err == nil {
			break // success
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err != nil {
		slog.Debug(fmt.Sprintf("[supervisor] session %s process has exited\r\n", s.session.Id))
		log.Fatalf("failed to connect to ctrl.sock in '%s' after 3 retries: %v", s.session.SockerCtrl, err)
	}

	// Wrap the connection in a JSON-RPC client
	s.session.SessionClientRPC = rpc.NewClientWithCodec(jsonrpc.NewClientCodec(conn))

	// Example: call SessionController.Status (no args, returns SessionStatus)
	var info api.SessionStatus

	err = s.session.SessionClientRPC.Call("SessionController.Status", struct{}{}, &info)
	if err != nil {
		log.Fatalf("RPC call failed: %v", err)
		return err
	}

	slog.Debug(fmt.Sprintf("[supervisor] rpc->session (Status): %+v\r\n", info))

	return nil

}

func (s *SupervisorRunnerExec) attachIOSocket() error {

	var conn net.Conn
	var err error

	// Dial the Unix domain socket
	for range 3 {
		conn, err = net.Dial("unix", s.session.SocketIO)
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
	if err := s.toBashUIMode(); err != nil {
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
			trySendEvent(s.events, SupervisorRunnerEvent{ID: s.session.Id, Type: EvCmdExited, Err: err, When: time.Now()})
		} else if e != nil {
			slog.Debug(fmt.Sprintf("[supervisor] stdin->socket error: %v\r\n", e))
			trySendEvent(s.events, SupervisorRunnerEvent{ID: s.session.Id, Type: EvError, Err: err, When: time.Now()})
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
			trySendEvent(s.events, SupervisorRunnerEvent{ID: s.session.Id, Type: EvCmdExited, Err: err, When: time.Now()})
		} else if e != nil {
			slog.Debug(fmt.Sprintf("[supervisor] socket->stdout error: %v\r\n", e))
			trySendEvent(s.events, SupervisorRunnerEvent{ID: s.session.Id, Type: EvError, Err: err, When: time.Now()})
		}

		errCh <- e
	}()

	// Force resize
	syscall.Kill(syscall.Getpid(), syscall.SIGWINCH)

	go func() error {
		// Wait for either context cancel or one side finishing
		select {
		case <-s.sessionCtx.Done():
			slog.Debug("[supervisor-runner] context done\r\n")
			_ = conn.Close() // unblock goroutines
			<-errCh
			<-errCh
			return s.sessionCtx.Err()
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

func (s *SupervisorRunnerExec) attachAndForwardResize() error {

	// Send initial size once (use the supervisor's TTY: os.Stdin)
	if rows, cols, err := pty.Getsize(os.Stdin); err == nil {
		_ = s.session.SessionClientRPC.Call("Session.Resize",
			api.ResizeArgs{Cols: int(cols), Rows: int(rows)}, &api.Empty{})
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	go func() {
		defer signal.Stop(ch)
		for {
			select {
			case <-s.sessionCtx.Done():
				return
			case <-ch:
				// slog.Debug("[supervisor] window change\r\n")
				// Query current terminal size again on every WINCH
				rows, cols, err := pty.Getsize(os.Stdin)
				if err != nil {
					// harmless: keep going; terminal may be detached briefly
					continue
				}
				var reply api.Empty
				if err := s.session.SessionClientRPC.Call("SessionController.Resize",
					api.ResizeArgs{Cols: int(cols), Rows: int(rows)}, &reply); err != nil {
					// Don't kill the process on resize failure; just log
					slog.Debug(fmt.Sprintf("resize RPC failed: %v\r\n", err))
				}
			}
		}
	}()
	return nil
}

// toBashUIMode: set terminal to RAW, update flags
func (s *SupervisorRunnerExec) toBashUIMode() error {
	lastTermState, err := toRawMode()
	if err != nil {
		log.Fatalf("MakeRaw: %v", err)
		return err
	}
	// defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	s.uiMode = UIBash
	s.lastTermState = lastTermState
	return nil
}

// toSupervisorUIMode: set terminal to COOKED for your REPL
func (s *SupervisorRunnerExec) toExitShell() error {
	err := term.Restore(int(os.Stdin.Fd()), s.lastTermState)
	if err != nil {
		log.Fatalf("MakeRaw: %v", err)
		return err
	}

	s.uiMode = UIExitShell
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
func trySendEvent(ch chan<- SupervisorRunnerEvent, ev SupervisorRunnerEvent) {
	slog.Debug(fmt.Sprintf("[supervisor] send event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano)))

	select {
	case ch <- ev:
	default:
		// drop on the floor if controller is momentarily busy; channel should be buffered
	}
}
