package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sbsh/pkg/api"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

type Session struct {
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
	evCh   chan<- api.SessionEvent // fan-out to controller (send-only from session)
	stopCh chan struct{}           // internal shutdown

	ctxCancel context.CancelFunc
	done      chan struct{} // closed when both goroutines exit
	errs      chan error    // internal: size 2

	ctrlLn net.Listener
	ioLn   net.Listener

	sessionDir string
	socketIO   string
	socketCTRL string
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[api.SessionID]Session
	ctx      context.Context
	current  api.SessionID
}

// NewSession creates the struct, not started yet (no PTY, no process).
func NewSession(spec *api.SessionSpec) *Session {
	return &Session{
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
		evCh:   nil, // assigned in Start(...)
		stopCh: nil, // created in Start(...)
	}
}

// Close requests graceful shutdown (closes PTY, stops goroutines, reaps child).
func (s *Session) Close() error {
	// stop accepting
	if s.ioLn != nil {
		_ = s.ioLn.Close()
	}
	if s.ctrlLn != nil {
		_ = s.ctrlLn.Close()
	}

	// // close clients
	// s.clientsMu.Lock()
	// for _, c := range s.clients {
	// 	_ = c.conn.Close()
	// 	close(c.wch)
	// }
	// s.clients = nil
	// s.clientsMu.Unlock()

	// kill PTY child and close PTY master as needed
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	if s.pty != nil {
		_ = s.pty.Close()
	}

	// remove sockets and dir
	if err := os.Remove(s.socketIO); err != nil {
		log.Printf("[session] couldn't remove IO socket: %s\r\n", s.socketIO)
	}
	if err := os.Remove(s.socketCTRL); err != nil {
		log.Printf("[session] couldn't remove CTRL socket: %s\r\n", s.socketCTRL)
	}
	_ = os.RemoveAll(s.sessionDir) // or leave meta/logs if you prefer

	return nil

}

// Resize forwards the current terminal size to the PTY (SIGWINCH handling).
func (s *Session) Resize(from *os.File) error { // typically os.Stdin
	return nil
}

// // Write writes bytes to the session PTY (used by controller or Smart executor).
// func (s *Session) Write(p []byte) (int, error)

// Open/Close the stdin forwarding gate (stdin->PTY). Reader goroutine
// will check this flag before writing to PTY.
func (s *Session) OpenStdinGate() {

}
func (s *Session) CloseStdinGate() {

}

// Control whether PTY output is rendered (reader still drains to avoid backpressure).
func (s *Session) SetOutput(policyOn bool) {

}

// // Accessors
func (s *Session) ID() api.SessionID {
	return s.id
}
func (s *Session) State() api.SessionState {
	return s.state
}
func (s *Session) Spec() api.SessionSpec {
	return s.spec
}

// Start spawns the child under PTY, starts the PTY->stdout reader goroutine,
// and begins emitting SessionEvent into evCh. Returns error if spawn fails.

func (s *Session) Start(ctx context.Context, evCh chan<- api.SessionEvent) error {
	if len(s.spec.Command) == 0 {
		return errors.New("empty command in SessionSpec")
	}

	// Set up session
	s.evCh = evCh
	s.stopCh = make(chan struct{})
	s.state = api.SessBash
	s.gates.StdinOpen = true
	s.gates.OutputOn = true

	// Set up sockets
	base, err := runtimeBase()
	if err != nil {
		return err
	}

	s.sessionDir = filepath.Join(base, "sessions", string(s.id))
	if err := os.MkdirAll(s.sessionDir, 0o700); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	s.socketCTRL = filepath.Join(s.sessionDir, "ctrl.sock")
	log.Printf("[session] CTRL socket: %s", s.socketCTRL)
	s.socketIO = filepath.Join(s.sessionDir, "io.sock")
	log.Printf("[session] IO socket: %s", s.socketIO)

	// Remove sockets if they already exist
	// remove sockets and dir
	if err := os.Remove(s.socketIO); err != nil {
		log.Printf("[session] couldn't remove stale IO socket: %s\r\n", s.socketIO)
	}
	if err := os.Remove(s.socketCTRL); err != nil {
		log.Printf("[session] couldn't remove stale CTRL socket: %s\r\n", s.socketCTRL)
	}

	// Listen to CONTROL SOCKET
	ctrlLn, err := net.Listen("unix", s.socketCTRL)
	if err != nil {
		return fmt.Errorf("listen ctrl: %w", err)
	}
	if err := os.Chmod(s.socketCTRL, 0o600); err != nil {
		ctrlLn.Close()
		return err
	}

	// Listen to IO SOCKET
	ioLn, err := net.Listen("unix", s.socketIO)
	if err != nil {
		ctrlLn.Close()
		return fmt.Errorf("listen io: %w", err)
	}
	if err := os.Chmod(s.socketIO, 0o600); err != nil {
		ctrlLn.Close()
		ioLn.Close()
		return err
	}

	// keep references for Close()
	s.ctrlLn = ctrlLn
	s.ioLn = ioLn

	// Start the Session Socket CTRL Loop
	// go func() {
	// 	srv := rpc.NewServer()
	// 	_ = srv.RegisterName("Session", &sessionRPC{Core: c})
	// 	for {
	// 		conn, err := ctrlLn.Accept()
	// 		if err != nil {
	// 			// listener closed -> exit loop
	// 			if _, ok := err.(net.Error); ok {
	// 				continue
	// 			}
	// 			return
	// 		}
	// 		go srv.ServeCodec(jsonrpc.NewServerCodec(conn))
	// 	}
	// }()

	// Build the child command with context (so ctx cancel can kill it)
	cmd := exec.CommandContext(ctx, s.spec.Command[0], s.spec.Command[1:]...)
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

	// Start under a PTY and inherit current terminal size
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	s.pty = ptmx

	/*
	* PAIRING
	 */

	var once sync.Once
	sessionCtx, cancel := context.WithCancel(ctx)
	s.ctxCancel = cancel
	s.done = make(chan struct{})
	s.errs = make(chan error, 2)

	/*
	* Resize Window
	 */
	// Initial resize (no fake signal needed)
	_ = pty.InheritSize(os.Stdin, s.pty)
	// Watch for window changes
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func(p *os.File) {
		defer signal.Stop(ch) // stop delivering to ch when we exit
		for {
			select {
			case <-sessionCtx.Done():
				return
			case <-ch:
				_ = pty.InheritSize(os.Stdin, p)
				log.Printf("[ctrl] Resize event has been received\r\n")
			}
		}
	}(s.pty)

	/*
	* PTY READER goroutine — single reader rule!
	 */
	go func(s *Session) {
		defer once.Do(cancel)

		buf := make([]byte, 8192)
		for {

			select {
			case <-sessionCtx.Done():
				s.errs <- sessionCtx.Err()
				return
			default:
			}

			n, err := s.pty.Read(buf)
			// drain/emit data
			if n > 0 {
				s.lastRead = time.Now()
				s.bytesOut += uint64(n)

				// Render if output is enabled; otherwise we just drain
				if s.gates.OutputOn {

					// log.Println("[session] output gate is on")
					_, err = os.Stdout.Write(buf[:n])
					if err != nil {
						// log.Println("[session] error writing raw data")
						return
					}
				}
			}

			// Handle read end/error
			if err != nil {
				// fmt.Printf("[session] stdout closed\n\r")
				// Linux PTYs often return EIO when slave side closes — treat as normal close
				if errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO) {
					trySendEvent(s.evCh, api.SessionEvent{ID: s.id, Type: api.EvClosed, Err: err, When: time.Now()})
				} else {
					trySendEvent(s.evCh, api.SessionEvent{ID: s.id, Type: api.EvError, Err: err, When: time.Now()})
				}

				s.errs <- sessionCtx.Err()

				return
			}
		}
	}(s)

	/*
	* PTY WRITER  routine
	 */
	go func(s *Session) {
		defer once.Do(cancel)
		buf := make([]byte, 4096)
		for {

			select {
			case <-sessionCtx.Done():
				// _ = s.pty.Close() // release master side if still open
				s.errs <- sessionCtx.Err()

				// Try graceful signal; fall back to Kill if still alive.
				// _ = s.cmd.Process.Signal(syscall.SIGHUP)
				// time.AfterFunc(500*time.Millisecond, func() { _ = s.cmd.Process.Kill() })
				// trySendEvent(s.evCh, SessionEvent{ID: s.id, Type: EvClosed, Err: err, When: time.Now()})

				return
			default:

				n, err := os.Stdin.Read(buf)
				if n > 0 {
					if s.gates.StdinOpen {

						if _, werr := s.pty.Write(buf[:n]); werr != nil {
							trySendEvent(s.evCh, api.SessionEvent{ID: s.id, Type: api.EvError, Err: werr, When: time.Now()})
							return
						}
					}
					// else: gate closed, drop input
				}

				if err != nil {
					// fmt.Printf("[session] stdin error\n\r")
					// stdin closed or fatal
					s.errs <- sessionCtx.Err()

					trySendEvent(s.evCh, api.SessionEvent{ID: s.id, Type: api.EvError, Err: err, When: time.Now()})
					return
				}

			}
		}
	}(s)

	s.pty.Write([]byte("echo 'Hello from Go!'\n"))
	s.pty.Write([]byte(`export PS1="(sbsh) $PS1"` + "\n"))
	// s.pty.Write([]byte(`__sbsh_emit() { printf '\033]1337;sbsh\007'; }` + "\n"))
	// s.pty.Write([]byte(`smart()  { __sbsh_emit;  }` + "\n"))

	// Supervisor: don’t block Start()
	go func() {
		// When one side finishes…
		_ = <-s.errs
		// …cancel the session context and close PTY to unblock the peer
		s.ctxCancel()
		s.gates.OutputOn = false
		s.gates.StdinOpen = false
		_ = s.pty.Close()
		// Wait for the second goroutine to report then signal “done”
		<-s.errs
		close(s.done)
		// Also reap child:
		_ = s.cmd.Wait()
	}()

	return nil
}

func runtimeBase() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sbsh", "run"), nil
}

/////////////////////////////////////////////////

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[api.SessionID]Session),
	}
}

/* Basic ops */

func (m *SessionManager) Add(s Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.id] = s
	if m.current == "" {
		m.current = s.id
	}
}

func (m *SessionManager) Get(id api.SessionID) (Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *SessionManager) ListLive() []api.SessionID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]api.SessionID, 0, len(m.sessions))
	for id := range m.sessions {
		out = append(out, id)
	}
	return out
}

func (m *SessionManager) Remove(id api.SessionID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	if m.current == id {
		m.current = "" // caller can SetCurrent to another live session
	}
}

func (m *SessionManager) Current() api.SessionID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

func (m *SessionManager) SetCurrent(id api.SessionID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; !ok {
		return errors.New("unknown session id")
	}
	m.current = id
	return nil
}

func (m *SessionManager) StartSession(id api.SessionID, ctx context.Context, evCh chan<- api.SessionEvent) error {
	m.mu.Lock()
	log.Printf("[session] SessionManager state locked")
	defer func() {
		m.mu.Unlock()
		log.Printf("[session] SessionManager state unlocked")
	}()
	if sess, ok := m.sessions[id]; !ok {
		return errors.New("unknown session id")
	} else {

		if err := sess.Start(ctx, evCh); err != nil {
			log.Fatalf("failed to start session: %v", err)
			return err
		}
	}
	m.current = id
	return nil
}

func (m *SessionManager) StopSession(id api.SessionID) error {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok {
		return errors.New("unknown session id")
	} else {

		if err := sess.Close(); err != nil {
			log.Fatalf("failed to stop session: %v", err)
			return err
		}

		m.Remove(id)

	}
	return nil
}

// helper: non-blocking event send so the PTY reader never stalls
func trySendEvent(ch chan<- api.SessionEvent, ev api.SessionEvent) {
	log.Printf("[session] send event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano))

	select {
	case ch <- ev:
	default:
		// drop on the floor if controller is momentarily busy; channel should be buffered
	}
}
