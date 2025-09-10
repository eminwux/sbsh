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
	evCh chan<- api.SessionEvent // fan-out to controller (send-only from session)

	ctxCancel context.CancelFunc
	mutualErr chan error // internal: size 2

	ioLn net.Listener

	socketIO string

	clientsMu sync.RWMutex
	clients   map[int]*ioClient
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[api.SessionID]*Session
	ctx      context.Context
	current  api.SessionID
}

type ioClient struct {
	id   int
	conn net.Conn
	wch  chan []byte // buffered write queue to avoid blocking PTY
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
		evCh: nil, // assigned in Start(...)
	}
}

// Close requests graceful shutdown (closes PTY, stops goroutines, reaps child).
func (s *Session) Close() error {
	// stop accepting
	if s.ioLn != nil {
		_ = s.ioLn.Close()
	}

	// close clients
	s.clientsMu.Lock()
	for _, c := range s.clients {
		_ = c.conn.Close()
		close(c.wch)
	}
	s.clients = nil
	s.clientsMu.Unlock()

	// kill PTY child and close PTY master as needed
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	if s.pty != nil {
		_ = s.pty.Close()
	}

	// remove sockets and dir
	// if err := os.Remove(s.socketIO); err != nil {
	// 	log.Printf("[session] couldn't remove IO socket: %s\r\n", s.socketIO)
	// }
	// if err := os.RemoveAll(sessionDir); err != nil {
	// 	log.Printf("[session] couldn't remove IO socket: %s\r\n", s.socketIO)
	// }
	return nil

}

// Resize forwards the current terminal size to the PTY (SIGWINCH handling).

func (s *Session) Resize(args api.ResizeArgs) {
	pty.Setsize(s.pty, &pty.Winsize{
		Cols: uint16(args.Cols),
		Rows: uint16(args.Rows),
	})
}

// Write writes bytes to the session PTY (used by controller or Smart executor).
func (s *Session) Write(p []byte) (int, error) {
	return s.pty.Write(p)
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

// Accessors
func (s *Session) ID() api.SessionID {
	return s.id
}
func (s *Session) State() api.SessionState {
	return s.state
}
func (s *Session) Spec() api.SessionSpec {
	return s.spec
}

// Function to be called by sbsh-session
func (s *Session) Start(ctx context.Context, evCh chan<- api.SessionEvent) error {

	// Set up session
	s.evCh = evCh
	s.state = api.SessBash
	s.gates.StdinOpen = true
	s.gates.OutputOn = true

	s.socketIO = filepath.Join(sessionDir, "io.sock")
	log.Printf("[session] IO socket: %s", s.socketIO)

	// Remove sockets if they already exist
	// remove sockets and dir
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

	// keep references for Close()
	s.ioLn = ioLn

	// Build the child command with context (so ctx cancel can kill it)
	cmd := exec.CommandContext(ctx, s.spec.Command, s.spec.CommandArgs...)
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
	// Intra-routine error channel
	s.mutualErr = make(chan error, 2)

	// Go func to handle new connections to the socket
	go func() {
		cid := 0
		for {
			// New client connects
			conn, err := s.ioLn.Accept()
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Temporary() {
					log.Printf("[session] socket error, continuing\r\n")
					continue
				}
				log.Printf("[session] closing client routine\r\n")

				return // listener closed
			}
			cid++
			cl := &ioClient{id: cid, conn: conn, wch: make(chan []byte, 64)}
			s.addClient(cl)

			/*
			 * PTY READER goroutine
			 */
			go func(s *Session, c *ioClient) {
				// If routine returns, close connection
				defer c.conn.Close()
				// If routine returns, cancel the context
				defer once.Do(s.ctxCancel)

				buf := make([]byte, 8192)
				for {
					// Loop between (a) check if context is done; and (b) new reads from buffer
					select {
					// If sessionCtx is done, tell partner routine
					case <-sessionCtx.Done():
						log.Printf("[session] closing reader\r\n")
						s.mutualErr <- sessionCtx.Err()
						log.Printf("[session] reader closed, finished routine\r\n")

					default:
					}

					n, err := s.pty.Read(buf)
					// drain/emit data
					if n > 0 {
						s.lastRead = time.Now()
						s.bytesOut += uint64(n)

						// Render if output is enabled; otherwise we just drain
						if s.gates.OutputOn {
							_, err := c.conn.Write(buf[:n])
							if err != nil {
								log.Println("[session] error writing raw data to client")
								once.Do(s.ctxCancel)
								// return
							}
						}
					}

					// Handle read end/error
					if err != nil {
						log.Printf("[session] stdout closed %v:\r\n", err)
						// Linux PTYs often return EIO when slave side closes — treat as normal close
						if errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO) {
							trySendEvent(s.evCh, api.SessionEvent{ID: s.id, Type: api.EvClosed, Err: err, When: time.Now()})
						} else {
							trySendEvent(s.evCh, api.SessionEvent{ID: s.id, Type: api.EvError, Err: err, When: time.Now()})
						}
						// Now that context is done, tell partner routine
						once.Do(s.ctxCancel)
						// s.mutualErr <- sessionCtx.Err()

						// return
					}
				}
			}(s, cl)

			/*
			* PTY WRITER  routine
			 */
			go func(s *Session, c *ioClient) {
				// If routine returns, close connection
				defer c.conn.Close()
				// If routine returns, cancel the context
				// defer once.Do(s.ctxCancel)
				// If routine returns, remove the client
				defer func() { s.removeClient(c) }()

				buf := make([]byte, 4096)
				for {
					// Loop between (a) check if context is done; and (b) new reads from buffer
					select {
					case <-sessionCtx.Done():
						log.Printf("[session] closing writer\r\n")
						s.mutualErr <- sessionCtx.Err()
						log.Printf("[session] writer closed, finished routine\r\n")

						return
					default:

						n, err := cl.conn.Read(buf)
						if n > 0 {
							if s.gates.StdinOpen {

								if _, werr := s.pty.Write(buf[:n]); werr != nil {
									trySendEvent(s.evCh, api.SessionEvent{ID: s.id, Type: api.EvError, Err: werr, When: time.Now()})
									once.Do(s.ctxCancel)
									// return
								}
							}
							// else: gate closed, drop input
						}

						if err != nil {
							log.Printf("[session] stdin error: %v\r\n", err)
							// stdin closed or fatal

							trySendEvent(s.evCh, api.SessionEvent{ID: s.id, Type: api.EvError, Err: err, When: time.Now()})
							once.Do(s.ctxCancel)
							// Now that context is done, tell partner routine
							// s.mutualErr <- sessionCtx.Err()

							// return
						}

					}
				}
			}(s, cl)

			s.Write([]byte(`export PS1="(sbsh-` + s.id + `) $PS1"` + "\n"))
			// s.pty.Write([]byte("echo 'Hello from Go!'\n"))
			// s.pty.Write([]byte(`export PS1="(sbsh) $PS1"` + "\n"))
			// s.pty.Write([]byte(`__sbsh_emit() { printf '\033]1337;sbsh\007'; }` + "\n"))
			// s.pty.Write([]byte(`smart()  { __sbsh_emit;  }` + "\n"))

			// Supervisor: don’t block Start()
			// This function is used when connection with a client is established
			go func() {
				// When one side finishes…
				<-s.mutualErr
				log.Printf("[session] mutual control, first exit\r\n")
				log.Printf("[session] closing socket\r\n")
				_ = s.ioLn.Close()
				// …cancel the session context and close PTY to unblock the peer
				log.Printf("[session] cancelling context\r\n")
				once.Do(s.ctxCancel)
				s.gates.OutputOn = false
				s.gates.StdinOpen = false
				log.Printf("[session] closing pty\r\n")
				_ = s.pty.Close()
				// Wait for the second goroutine to report then signal “done”
				<-s.mutualErr
				log.Printf("[session] mutual control, second exit\r\n")
				// Also reap child:
				// _ = s.cmd.Wait()
			}()

		}
	}()

	go func() {
		log.Printf("[session] pid=%d, waiting on bash pid=%d\r\n", os.Getpid(), s.cmd.Process.Pid)
		_ = s.cmd.Wait()
		log.Printf("[session] pid=%d, bash with pid=%d has exited\r\n", os.Getpid(), s.cmd.Process.Pid)
		_ = s.ioLn.Close()
		log.Printf("[session] cancelling context\r\n")
		s.ctxCancel()
		log.Printf("[session] sending EvSessionExited event\r\n")
		trySendEvent(s.evCh, api.SessionEvent{ID: s.id, Type: api.EvSessionExited, Err: err, When: time.Now()})
	}()

	return nil
}

func (s *Session) addClient(c *ioClient) {
	s.clientsMu.Lock()
	s.clients[c.id] = c
	s.clientsMu.Unlock()
}
func (s *Session) removeClient(c *ioClient) {
	s.clientsMu.Lock()
	delete(s.clients, c.id)
	close(c.wch)
	s.clientsMu.Unlock()
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
