package session

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// Identity & lifecycle
type SessionID string

type SessionState int

const (
	SessBash SessionState = iota
	SessSupervisor
	SessTerminated
)

// What kind of session we spawn
type SessionKind int

const (
	SessLocal SessionKind = iota // /bin/bash -i
	SessSSH                      // ssh -tt user@host ...
)

// Inputs needed to spawn a session; serialize parts of this into sessions.json
type SessionSpec struct {
	ID      SessionID
	Kind    SessionKind
	Label   string            // user-friendly name
	Command []string          // for local: ["bash","-i"]; for ssh: ["ssh","-tt","user@host"]
	Env     []string          // TERM, COLORTERM, etc.
	Context map[string]string // kubectl ns, cwd hint, etc.
	LogDir  string
}

type SessionEventType int

const (
	EvData     SessionEventType = iota // optional metrics
	EvSentinel                         // OSC sentinel detected
	EvClosed                           // PTY closed / child exited
	EvError                            // abnormal error
)

type SessionEvent struct {
	ID    SessionID
	Type  SessionEventType
	Bytes int   // for EvData
	Err   error // for EvClosed/EvError
	When  time.Time
}

type Session struct {
	// immutable
	id   SessionID
	spec SessionSpec

	// runtime (owned by Session)
	cmd   *exec.Cmd
	pty   *os.File // master
	state SessionState
	fsm   *SentinelFSM
	gates struct {
		StdinOpen bool
		OutputOn  bool
	}
	oldState *term.State

	// observability
	bytesIn, bytesOut uint64
	lastRead          time.Time

	// signaling
	evCh   chan<- SessionEvent // fan-out to controller (send-only from session)
	stopCh chan struct{}       // internal shutdown

	cancel context.CancelFunc
	done   chan struct{} // closed when both goroutines exit
	errs   chan error    // internal: size 2
	ctx    context.Context
}

// NewSession creates the struct, not started yet (no PTY, no process).
func NewSession(spec SessionSpec) *Session {
	log.Printf("[session] New session is being created")
	return &Session{
		id:   spec.ID,
		spec: spec,

		// runtime (initialized but inactive)
		cmd:   nil,
		pty:   nil,
		state: SessBash, // default logical state before start
		fsm:   &SentinelFSM{},

		gates: struct {
			StdinOpen bool
			OutputOn  bool
		}{
			StdinOpen: true, // allow stdin by default once started
			OutputOn:  true, // render PTY output by default
		},

		oldState: nil,

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
	// Here we need to implement the case
	// when the user commands to close the session
	// using the CLI

	//c.ctx CANCEL!
	return nil

}

// Resize forwards the current terminal size to the PTY (SIGWINCH handling).
func (s *Session) Resize(from *os.File) error { // typically os.Stdin
	return nil
}

// // Write writes bytes to the session PTY (used by controller or Smart executor).
// func (s *Session) Write(p []byte) (int, error)

// EnterSupervisor flips internal state; does NOT touch global terminal modes.
// Controller calls this upon EvSentinel.
func (s *Session) EnterSupervisor() {

}

// ExitSupervisor flips back to bash mode; controller calls when REPL exits.
func (s *Session) ExitSupervisor() {

}

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
func (s *Session) ID() SessionID {
	return s.id
}
func (s *Session) State() SessionState {
	return s.state
}
func (s *Session) Spec() SessionSpec {
	return s.spec
}

// Start spawns the child under PTY, starts the PTY->stdout reader goroutine,
// and begins emitting SessionEvent into evCh. Returns error if spawn fails.

func (s *Session) Start(ctx context.Context, evCh chan<- SessionEvent) error {
	if len(s.spec.Command) == 0 {
		return errors.New("empty command in SessionSpec")
	}
	s.evCh = evCh
	s.stopCh = make(chan struct{})
	s.fsm = &SentinelFSM{}
	s.state = SessBash
	s.gates.StdinOpen = true
	s.gates.OutputOn = true

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
	s.ctx = sessionCtx
	s.cancel = cancel
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

				found, cleaned := s.fsm.Feed(buf[:n])

				if found {
					// Non-blocking event send
					trySendEvent(s.evCh, SessionEvent{ID: s.id, Type: EvSentinel, When: time.Now()})
				}

				// Render if output is enabled; otherwise we just drain
				if s.gates.OutputOn {

					// log.Println("[session] output gate is on")

					if found && len(cleaned) > 0 {
						log.Println("[session] sentinel found")
						_, _ = os.Stdout.Write(cleaned)
						if err != nil {
							// log.Println("[session] error writing cleaned data")
							return
						}
					} else {
						// log.Printf("[session] data arrived %d\n", n)
						_, err = os.Stdout.Write(buf[:n])
						if err != nil {
							// log.Println("[session] error writing raw data")
							return
						}
					}
				}
			}

			// Handle read end/error
			if err != nil {
				// fmt.Printf("[session] stdout closed\n\r")
				// Linux PTYs often return EIO when slave side closes — treat as normal close
				if errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO) {
					trySendEvent(s.evCh, SessionEvent{ID: s.id, Type: EvClosed, Err: err, When: time.Now()})
				} else {
					trySendEvent(s.evCh, SessionEvent{ID: s.id, Type: EvError, Err: err, When: time.Now()})
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
							trySendEvent(s.evCh, SessionEvent{ID: s.id, Type: EvError, Err: werr, When: time.Now()})
							return
						}
					}
					// else: gate closed, drop input
				}

				if err != nil {
					// fmt.Printf("[session] stdin error\n\r")
					// stdin closed or fatal
					s.errs <- sessionCtx.Err()

					trySendEvent(s.evCh, SessionEvent{ID: s.id, Type: EvError, Err: err, When: time.Now()})
					return
				}

			}
		}
	}(s)

	s.pty.Write([]byte("echo 'Hello from Go!'\n"))
	s.pty.Write([]byte(`export PS1="(sbsh) $PS1"` + "\n"))
	s.pty.Write([]byte(`__sbsh_emit() { printf '\033]1337;sbsh\007'; }` + "\n"))
	s.pty.Write([]byte(`smart()  { __sbsh_emit;  }` + "\n"))

	// go func() {
	// 	_ = s.cmd.Wait() // MUST be called exactly once
	// 	// Child has exited → notify & cleanup
	// 	// WRITER
	// 	// SEND MESSAGE when process finishes
	// 	// trySendEvent(s.evCh, SessionEvent{ID: s.id, Type: EvClosed, Err: err, When: time.Now()})
	// 	s.errs <- sessionCtx.Err()
	// }()

	// Supervisor: don’t block Start()
	go func() {
		// When one side finishes…
		_ = <-s.errs
		// …cancel the session context and close PTY to unblock the peer
		s.cancel()
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

/////////////////////////////////////////////////

/* Dependencies the manager relies on (minimal) */

/* Manager */

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[SessionID]Session
	ctx      context.Context
	current  SessionID
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[SessionID]Session),
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

func (m *SessionManager) Get(id SessionID) (Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *SessionManager) ListLive() []SessionID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]SessionID, 0, len(m.sessions))
	for id, s := range m.sessions {
		if s.state != SessTerminated {
			out = append(out, id)
		}
	}
	return out
}

func (m *SessionManager) Remove(id SessionID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	if m.current == id {
		m.current = "" // caller can SetCurrent to another live session
	}
}

func (m *SessionManager) Current() SessionID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

func (m *SessionManager) SetCurrent(id SessionID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; !ok {
		return errors.New("unknown session id")
	}
	m.current = id
	return nil
}

func (m *SessionManager) StartSession(id SessionID, ctx context.Context, evCh chan<- SessionEvent) error {
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

func (m *SessionManager) StopSession(id SessionID) error {
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
func trySendEvent(ch chan<- SessionEvent, ev SessionEvent) {
	log.Printf("[session] send event: id=%s type=%v err=%v when=%s\r\n", ev.ID, ev.Type, ev.Err, ev.When.Format(time.RFC3339Nano))

	select {
	case ch <- ev:
	default:
		// drop on the floor if controller is momentarily busy; channel should be buffered
	}
}

// ESC]1337;sbshBEL  (no payload)
var sentinel = []byte{0x1b, ']', '1', '3', '3', '7', ';', 's', 'b', 's', 'h', 0x07}

type SentinelFSM struct {
	matched int // how many bytes of sentinel matched so far (across chunks)
}

// Feed consumes a chunk and returns (found, cleanedOut).
// - found: true if a complete sentinel was detected in this chunk
// - cleanedOut: the input with the sentinel bytes removed
func (s *SentinelFSM) Feed(chunk []byte) (bool, []byte) {
	found := false
	out := make([]byte, 0, len(chunk))

	for _, b := range chunk {
		if s.matched > 0 {
			// continue matching
			if b == sentinel[s.matched] {
				s.matched++
				if s.matched == len(sentinel) {
					// full sentinel matched: strip it and signal
					found = true
					s.matched = 0
				}
				continue // absorb matched byte; emit nothing
			}
			// mismatch: previously-matched bytes were normal output, emit them
			out = append(out, sentinel[:s.matched]...)
			s.matched = 0
			// reprocess current byte in base state
		}

		// base state
		if b == sentinel[0] {
			s.matched = 1 // possible start
			continue
		}
		out = append(out, b)
	}

	// If the chunk ended exactly on a full match, 'found' is already true.
	// If it ended mid-match, we keep 'matched' to continue across the next chunk.
	return found, out
}
