package supervisor

// import (
// 	"errors"
// 	"net/rpc"
// 	"sbsh/pkg/api"
// 	"sync"
// )

// type SessionManager struct {
// 	mu       sync.RWMutex
// 	sessions map[api.SessionID]*SupervisedSession
// 	current  api.SessionID
// }

// type SupervisedSession struct {
// 	id               api.SessionID
// 	kind             api.SessionKind
// 	label            string // user-friendly name
// 	command          string
// 	commandArgs      []string          // for local: ["bash","-i"]; for ssh: ["ssh","-tt","user@host"]
// 	env              []string          // TERM, COLORTERM, etc.
// 	context          map[string]string // kubectl ns, cwd hint, etc.
// 	logDir           string
// 	sockerCtrl       string
// 	socketIO         string
// 	pid              int
// 	sessionClientRPC *rpc.Client
// }

// func NewSessionManager() *SessionManager {
// 	return &SessionManager{
// 		sessions: make(map[api.SessionID]*SupervisedSession),
// 	}
// }

// func NewSupervisedSession(spec *api.SessionSpec) *SupervisedSession {
// 	return &SupervisedSession{
// 		id:          spec.ID,
// 		kind:        spec.Kind,
// 		label:       spec.Label,
// 		command:     spec.Command,
// 		commandArgs: spec.CommandArgs,
// 		env:         spec.Env,
// 		logDir:      spec.LogDir,
// 		sockerCtrl:  spec.SockerCtrl,
// 		socketIO:    spec.SocketIO,
// 	}
// }

// /* Basic ops */

// func (m *SessionManager) Add(s *SupervisedSession) {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	m.sessions[s.id] = s
// 	if m.current == "" {
// 		m.current = s.id
// 	}
// }

// func (m *SessionManager) Get(id api.SessionID) (*SupervisedSession, bool) {
// 	m.mu.RLock()
// 	defer m.mu.RUnlock()
// 	s, ok := m.sessions[id]
// 	return s, ok
// }

// func (m *SessionManager) ListLive() []api.SessionID {
// 	m.mu.RLock()
// 	defer m.mu.RUnlock()
// 	out := make([]api.SessionID, 0, len(m.sessions))
// 	for id := range m.sessions {
// 		out = append(out, id)
// 	}
// 	return out
// }

// func (m *SessionManager) Remove(id api.SessionID) {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	delete(m.sessions, id)
// 	if m.current == id {
// 		m.current = "" // caller can SetCurrent to another live session
// 	}
// }

// func (m *SessionManager) Current() api.SessionID {
// 	m.mu.RLock()
// 	defer m.mu.RUnlock()
// 	return m.current
// }

// func (m *SessionManager) SetCurrent(id api.SessionID) error {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	if _, ok := m.sessions[id]; !ok {
// 		return errors.New("unknown session id")
// 	}
// 	m.current = id
// 	return nil
// }

// func (m *SessionManager) StopSession(id api.SessionID) error {
// 	m.mu.Lock()
// 	_, ok := m.sessions[id]
// 	m.mu.Unlock()
// 	if !ok {
// 		return errors.New("unknown session id")
// 	} else {

// 		// if err := sess.Close(); err != nil {
// 		// 	log.Fatalf("failed to stop session: %v", err)
// 		// 	return err
// 		// }

// 		m.Remove(id)

// 	}
// 	return nil
// }
