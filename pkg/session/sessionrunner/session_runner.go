package sessionrunner

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
	"path/filepath"
	"sbsh/pkg/api"
	"sbsh/pkg/common"
	"sbsh/pkg/session/sessionrpc"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

type SessionRunner interface {
	OpenSocketCtrl() (net.Listener, error)
	StartServer(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error, errCh chan error)
	StartSession(ctx context.Context, evCh chan<- SessionRunnerEvent) error
	ID() api.SessionID
	Close(reason error) error
	Resize(args api.ResizeArgs)
}

type SessionRunnerExec struct {
	sessionCtxCancel context.CancelFunc
	sessionCtx       context.Context

	// immutable
	id   api.SessionID
	spec api.SessionSpec

	// runtime (owned by Session)
	cmd   *exec.Cmd
	pty   *os.File // master
	state api.SessionState

	gates struct {
		StdinOpen bool
		OutputOn  bool
	}

	// observability
	bytesIn, bytesOut uint64
	lastRead          time.Time

	// signaling
	evCh chan<- SessionRunnerEvent // fan-out to controller (send-only from session)

	listenerIO   net.Listener
	listenerCtrl net.Listener

	socketIO   string
	socketCtrl string

	clientsMu sync.RWMutex
	clients   map[int]*ioClient

	closeReqCh chan error
	closedCh   chan struct{}
	closingCh  chan struct{}
}
type ioClient struct {
	id       int
	conn     net.Conn
	pipeInR  *os.File
	pipeInW  *os.File
	pipeOutR *os.File
	pipeOutW *os.File
}

var finishTermMgr chan struct{} = make(chan struct{}, 2)

func NewSessionRunnerExec(spec *api.SessionSpec) SessionRunner {
	return &SessionRunnerExec{
		id:   spec.ID,
		spec: *spec,

		// runtime (initialized but inactive)
		cmd:   nil,
		pty:   nil,
		state: api.SessBash, // default logical state before start

		gates: struct {
			StdinOpen bool
			OutputOn  bool
		}{
			StdinOpen: true, // allow stdin by default once started
			OutputOn:  true, // render PTY output by default
		},

		// observability (zeroed; will be updated when running)
		bytesIn:  0,
		bytesOut: 0,

		// signaling (set in Start)
		evCh: nil, // assigned in Start(...)

		closeReqCh: make(chan error),
		closingCh:  make(chan struct{}, 1),
		closedCh:   make(chan struct{}),
	}
}

type SessionRunnerEventType int

const (
	EvError SessionRunnerEventType = iota // abnormal error
	EvCmdExited
)

type SessionRunnerEvent struct {
	ID    api.SessionID
	Type  SessionRunnerEventType
	Bytes int   // for EvData
	Err   error // for EvClosed/EvError
	When  time.Time
}

func (sr *SessionRunnerExec) ID() api.SessionID {
	return sr.id
}

func (sr *SessionRunnerExec) OpenSocketCtrl() (net.Listener, error) {

	// Set up sockets
	base, err := common.RuntimeBaseSessions()
	if err != nil {
		return nil, err
	}

	runPath := filepath.Join(base, string(sr.id))
	if err := os.MkdirAll(runPath, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir session dir: %w", err)
	}

	sr.socketCtrl = filepath.Join(runPath, "ctrl.sock")
	log.Printf("[sessionCtrl] CTRL socket: %s", sr.socketCtrl)

	// Remove sockets if they already exist
	// remove sockets and dir
	if err := os.Remove(sr.socketCtrl); err != nil {
		log.Printf("[sessionCtrl] couldn't remove stale CTRL socket: %s\r\n", sr.socketCtrl)
	}

	// Listen to CONTROL SOCKET
	ctrlLn, err := net.Listen("unix", sr.socketCtrl)
	if err != nil {
		return nil, fmt.Errorf("listen ctrl: %w", err)
	}

	sr.listenerCtrl = ctrlLn

	if err := os.Chmod(sr.socketCtrl, 0o600); err != nil {
		ctrlLn.Close()
		return nil, err
	}

	// keep references for Close()

	return ctrlLn, nil
}
func (sr *SessionRunnerExec) StartServer(ctx context.Context, sc *sessionrpc.SessionControllerRPC, readyCh chan error, doneCh chan error) {
	defer func() {
		// Ensure ln is closed and no leaks on exit
		_ = sr.listenerCtrl.Close()
	}()
	go func() {
		<-ctx.Done()
		_ = sr.listenerCtrl.Close()
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

	// stop accepting when ctx is canceled.

	for {
		conn, err := sr.listenerCtrl.Accept()
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

func (sr *SessionRunnerExec) StartSession(ctx context.Context, evCh chan<- SessionRunnerEvent) error {

	sr.evCh = evCh

	sessionCtx, cancel := context.WithCancel(ctx)
	sr.sessionCtx = sessionCtx
	sr.sessionCtxCancel = cancel

	if err := sr.openSocketIO(); err != nil {
		log.Fatalf("failed to open IO socket for session %s: %v", sr.id, err)
		return err
	}

	if err := sr.prepareSessionCommand(); err != nil {
		log.Fatalf("failed to run session command for session %s: %v", sr.id, err)
		return err
	}

	if err := sr.startPTY(); err != nil {
		log.Fatalf("failed to start PTY for session %s: %v", sr.id, err)
		return err
	}

	go sr.waitOnSession()

	return nil
}

func (sr *SessionRunnerExec) Resize(args api.ResizeArgs) {
	pty.Setsize(sr.pty, &pty.Winsize{
		Cols: uint16(args.Cols),
		Rows: uint16(args.Rows),
	})
}

func (s *SessionRunnerExec) openSocketIO() error {

	// Set up sockets
	base, err := common.RuntimeBaseSessions()
	if err != nil {
		return err
	}

	runPath := filepath.Join(base, string(s.id))
	if err := os.MkdirAll(runPath, 0o700); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	s.socketIO = filepath.Join(runPath, "io.sock")
	log.Printf("[session] IO socket: %s", s.socketIO)

	// Remove socket if already exists
	if err := os.Remove(s.socketIO); err != nil {
		log.Printf("[session] couldn't remove stale IO socket: %s\r\n", s.socketIO)
	}

	// Listen to IO SOCKET
	ioLn, err := net.Listen("unix", s.socketIO)
	if err != nil {
		return fmt.Errorf("listen io: %w", err)
	}
	if err := os.Chmod(s.socketIO, 0o600); err != nil {
		_ = ioLn.Close()
		return err
	}

	s.clientsMu.Lock()
	s.clients = make(map[int]*ioClient)
	s.clientsMu.Unlock()

	s.listenerIO = ioLn

	return nil
}

func (s *SessionRunnerExec) prepareSessionCommand() error {

	// Build the child command with context (so ctx cancel can kill it)
	cmd := exec.CommandContext(s.sessionCtx, s.spec.Command, s.spec.CommandArgs...)
	// Environment: use provided or inherit
	if len(s.spec.Env) > 0 {
		cmd.Env = s.spec.Env
	} else {
		cmd.Env = os.Environ()
	}

	// Start the process in a new session so it has its own process group
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setctty: true, // make the child the controlling TTY
		Setsid:  true, // new session
	}

	// Make sure TERM is reasonable if not set (helps colors)
	hasTERM := false
	for _, e := range cmd.Env {
		if len(e) >= 5 && e[:5] == "TERM=" {
			hasTERM = true
			break
		}
	}
	if !hasTERM {
		cmd.Env = append(cmd.Env, "TERM=xterm-256color", "COLORTERM=truecolor")
	}

	s.cmd = cmd

	return nil
}

func (s *SessionRunnerExec) startPTY() error {

	// Start under a PTY and inherit current terminal size
	ptmx, err := pty.Start(s.cmd)
	if err != nil {
		return err
	}
	s.pty = ptmx

	go func() {
		log.Printf("[session] pid=%d, waiting on bash pid=%d\r\n", os.Getpid(), s.cmd.Process.Pid)
		_ = s.cmd.Wait() // blocks until process exits
		log.Printf("[session] pid=%d, bash with pid=%d has exited\r\n", os.Getpid(), s.cmd.Process.Pid)
		s.closeReqCh <- fmt.Errorf("the shell process has exited")

	}()

	// StdIn
	// PTY reads from pipeInR
	// conn writes to pipeInW
	pipeInR, pipeInW, err := os.Pipe()
	if err != nil {
		log.Printf("[session] error opening IN pipe: %v\r\n", err)
		return fmt.Errorf("error opening IN pipe: %w", err)
	}

	// StdOut
	// conn reads from pipeOutR
	// PTY writes to pipeOutW
	pipeOutR, pipeOutW, err := os.Pipe()
	if err != nil {
		log.Printf("[session] error opening OUT pipe: %v\r\n", err)
		return fmt.Errorf("error opening OUT pipe: %w", err)
	}

	go s.terminalManager(pipeInR, pipeOutW)

	go s.handleConnections(pipeInR, pipeInW, pipeOutR, pipeOutW)

	return nil
}

func (s *SessionRunnerExec) waitOnSession() {

	select {
	case err := <-s.closeReqCh:
		log.Printf("[session] sending EvSessionExited event\r\n")
		trySendEvent(s.evCh, SessionRunnerEvent{ID: s.id, Type: EvCmdExited, Err: err, When: time.Now()})
		return
	case <-s.sessionCtx.Done():
		log.Printf("[session] session context has been closed\r\n")
		s.Close(s.sessionCtx.Err())
		return
	}

}

func (s *SessionRunnerExec) terminalManager(pipeInR *os.File, pipeOutW *os.File) error {
	/*
	 * PTY READER goroutine
	 */

	go func() {
		s.terminalManagerReader(pipeOutW, finishTermMgr)
	}()

	/*
	* PTY WRITER  routine
	 */
	go func() {
		s.terminalManagerWriter(pipeInR, finishTermMgr)
	}()

	s.Write([]byte(`export PS1="(sbsh-` + s.id + `) $PS1"` + "\n"))
	// s.pty.Write([]byte("echo 'Hello from Go!'\n"))
	// s.pty.Write([]byte(`export PS1="(sbsh) $PS1"` + "\n"))
	// s.pty.Write([]byte(`__sbsh_emit() { printf '\033]1337;sbsh\007'; }` + "\n"))
	// s.pty.Write([]byte(`smart()  { __sbsh_emit;  }` + "\n")

	return nil
}

func (s *SessionRunnerExec) handleClient(client *ioClient) {
	defer client.conn.Close()
	errCh := make(chan error, 2)

	// READ FROM CONN, WRITE TO PTY STDIN
	go func(chan error) {
		// conn writes to pipeInW
		w, err := io.Copy(client.pipeInW, client.conn)
		if err != nil {
			errCh <- fmt.Errorf("error in conn->pty copy pipe: %w", err)
		}
		if w == 0 {
			errCh <- fmt.Errorf("EOF in conn->pty copy pipe: %w", err)
		}
	}(errCh)

	// READ FROM PTY STDOUT, WRITE TO CONN
	go func(chan error) {
		// conn reads from pipeOutR
		w, err := io.Copy(client.conn, client.pipeOutR)
		if err != nil {
			errCh <- fmt.Errorf("error in pty->conn copy pipe: %w", err)
		}
		if w == 0 {
			errCh <- fmt.Errorf("EOF in pty->conn copy pipe: %w", err)
		}
	}(errCh)

	err := <-errCh
	if err != nil {
		log.Printf("[session-runner] error in copy pipes: %v\r\n", err)
	}
	client.conn.Close()
	close(errCh)
	s.removeClient(client)

}

func (s *SessionRunnerExec) handleConnections(pipeInR, pipeInW, pipeOutR, pipeOutW *os.File) error {

	cid := 0
	for {
		// New client connects
		log.Printf("[session] waiting for new connection...\r\n")
		conn, err := s.listenerIO.Accept()
		if err != nil {
			log.Printf("[session] closing IO listener routine\r\n")
			return err
		}
		log.Printf("[session] client connected!\r\n")
		cid++
		cl := &ioClient{id: cid, conn: conn, pipeInR: pipeInR, pipeInW: pipeInW, pipeOutR: pipeOutR, pipeOutW: pipeOutW}

		s.addClient(cl)
		go s.handleClient(cl)
	}
}
func (s *SessionRunnerExec) Close(reason error) error {

	log.Printf("[session-runner] closing session-runner on request, reason: %v\r\n", reason)
	s.closingCh <- struct{}{}
	log.Printf("[session-runner] sent 'closingCh' signal\r\n")

	// stop terminalManager, 2 messages needed, one for writer/reader
	finishTermMgr <- struct{}{}
	log.Printf("[session-runner] sent 'finishTermMgr' 1s signalt\r\n")
	finishTermMgr <- struct{}{}
	log.Printf("[session-runner] sent 'finishTermMgr' 2nd signal \r\n")

	// stop accepting
	if s.listenerCtrl != nil {
		if err := s.listenerCtrl.Close(); err != nil {
			log.Printf("[session-runner] could not close IO listener: %v", err)
			// return err
		}
	}
	// stop accepting
	if s.listenerIO != nil {
		if err := s.listenerIO.Close(); err != nil {
			log.Printf("[session-runner] could not close IO listener: %v", err)
			// return err
		}
	}

	// close clients
	s.clientsMu.Lock()
	for _, c := range s.clients {
		if err := c.conn.Close(); err != nil {
			log.Printf("[session-runner] could not close connection: %v\r\n", err)
			// return err
		}
	}

	s.clients = nil
	s.clientsMu.Unlock()

	// kill PTY child and close PTY master as needed
	if s.cmd != nil && s.cmd.Process != nil {
		if err := s.cmd.Process.Kill(); err != nil {
			log.Printf("[sesion] could not kill cmd: %v\r\n", err)
			// return err
		}
	}
	if s.pty != nil {
		if err := s.pty.Close(); err != nil {
			log.Printf("[sesion] could not close pty: %v\r\n", err)
			// return err
		}
	}

	// remove sockets and dir
	if err := os.Remove(s.socketIO); err != nil {
		log.Printf("[session] couldn't remove IO socket: %s: %v\r\n", s.socketIO, err)
		// return err
	}

	// remove Ctrl socket
	if err := os.Remove(s.socketCtrl); err != nil {
		log.Printf("[sessionCtrl] couldn't remove Ctrl socket %s: %v\r\n", s.socketCtrl, err)
	}

	if err := os.RemoveAll(filepath.Dir(s.socketIO)); err != nil {
		log.Printf("[session] couldn't remove socket Directory '%s': %v\r\n", s.socketIO, err)
	}

	close(s.closedCh)
	return nil

}

func (s *SessionRunnerExec) terminalManagerReader(pipeOutW *os.File, finReqCh <-chan struct{}) error {

	go func() {
		<-finReqCh
		log.Printf("[session-runner] finishing terminalManagerReader ")
		_ = pipeOutW.Close()
		_ = s.pty.Close() // This unblocks s.pty.Read(...)
		log.Printf("[session-runner] FINISHED terminalManagerReader ")
	}()

	buf := make([]byte, 8192)
	for {
		// READ FROM PTY - WRITE TO PIPE
		n, err := s.pty.Read(buf)

		// drain/emit data
		if n > 0 {
			s.lastRead = time.Now()
			s.bytesOut += uint64(n)

			// Render if output is enabled; otherwise we just drain
			if s.gates.OutputOn {
				//  WRITE TO PIPE
				// PTY writes to pipeOutW
				log.Printf("read from pty %q", buf[:n])
				log.Println("[session] writing to pipeOutW")
				_, err := pipeOutW.Write(buf[:n])
				if err != nil {
					log.Println("[session] error writing raw data to client")
					return ErrPipeWrite
				}
			}
		}

		// Handle read end/error
		if err != nil {
			log.Printf("[session] stdout err  %v:\r\n", err)
			trySendEvent(s.evCh, SessionRunnerEvent{ID: s.id, Type: EvError, Err: err, When: time.Now()})
			return ErrTerminalRead
		}
	}
}

func (s *SessionRunnerExec) terminalManagerWriter(pipeInR *os.File, finReqCh <-chan struct{}) error {
	go func() {
		<-finReqCh
		log.Printf("[session-runner] finishing terminalManagerWriter ")
		_ = pipeInR.Close()
		_ = s.pty.Close() // This unblocks s.pty.Read(...)
		log.Printf("[session-runner] FINISHED terminalManagerWriter ")
	}()

	buf := make([]byte, 4096)
	i := 0
	for {
		// READ FROM PIPE - WRITE TO PTY
		// PTY reads from pipeInR
		log.Printf("reading from pipeInR %d\r\n", i) // quoted, escapes control chars
		i++
		n, err := pipeInR.Read(buf)
		log.Printf("read from pipeInR %q", buf[:n]) // quoted, escapes control chars
		if n > 0 {
			if s.gates.StdinOpen {
				log.Println("[session] reading from pipeInR")
				// if _, werr := s.pty.Write(buf[:n]); werr != nil {
				// WRITE TO PIPE
				if _, werr := s.pty.Write(buf[:n]); werr != nil {
					trySendEvent(s.evCh, SessionRunnerEvent{ID: s.id, Type: EvError, Err: werr, When: time.Now()})
					return ErrPipeRead
				}
			}
			// else: gate closed, drop input
		}

		if err != nil {
			log.Printf("[session] stdin error: %v\r\n", err)
			trySendEvent(s.evCh, SessionRunnerEvent{ID: s.id, Type: EvError, Err: err, When: time.Now()})
			return ErrTerminalWrite
		}

		// }
	}
}

func (s *SessionRunnerExec) addClient(c *ioClient) {
	s.clientsMu.Lock()
	s.clients[c.id] = c
	s.clientsMu.Unlock()
}
func (s *SessionRunnerExec) removeClient(c *ioClient) {
	s.clientsMu.Lock()
	delete(s.clients, c.id)
	s.clientsMu.Unlock()
}

// Write writes bytes to the session PTY (used by controller or Smart executor).
func (s *SessionRunnerExec) Write(p []byte) (int, error) {
	return s.pty.Write(p)
}

// helper: non-blocking event send so the PTY reader never stalls
func trySendEvent(ch chan<- SessionRunnerEvent, ev SessionRunnerEvent) {
	log.Printf("[session] send event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano))

	select {
	case ch <- ev:
	default:
		// drop on the floor if controller is momentarily busy; channel should be buffered
	}
}
