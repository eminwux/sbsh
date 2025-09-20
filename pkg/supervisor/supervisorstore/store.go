package supervisorstore

import (
	"errors"
	"net/rpc"
	"sbsh/pkg/api"
	"sync"
)

type SessionManager interface {
	Add(s *SupervisedSession) error
	Get(id api.SessionID) (*SupervisedSession, bool)
	ListLive() []api.SessionID
	Remove(id api.SessionID)
	Current() api.SessionID
	SetCurrent(id api.SessionID) error
}

type SessionManagerExec struct {
	mu       sync.RWMutex
	sessions map[api.SessionID]*SupervisedSession
	current  api.SessionID
}

type SupervisedSession struct {
	Id               api.SessionID
	Kind             api.SessionKind
	Label            string // user-friendly name
	Command          string
	CommandArgs      []string          // for local: ["bash","-i"]; for ssh: ["ssh","-tt","user@host"]
	Env              []string          // TERM, COLORTERM, etc.
	Context          map[string]string // kubectl ns, cwd hint, etc.
	LogDir           string
	SockerCtrl       string
	SocketIO         string
	Pid              int
	SessionClientRPC *rpc.Client
}

func NewSessionManagerExec() SessionManager {
	return &SessionManagerExec{
		sessions: make(map[api.SessionID]*SupervisedSession),
	}
}

func NewSupervisedSession(spec *api.SessionSpec) *SupervisedSession {
	return &SupervisedSession{
		Id:          spec.ID,
		Kind:        spec.Kind,
		Label:       spec.Label,
		Command:     spec.Command,
		CommandArgs: spec.CommandArgs,
		Env:         spec.Env,
		LogDir:      spec.LogDir,
		SockerCtrl:  spec.SockerCtrl,
		SocketIO:    spec.SocketIO,
	}
}

/* Basic ops */

func (m *SessionManagerExec) Add(s *SupervisedSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions[s.Id] != nil {
		return ErrSessionExists
	}
	m.sessions[s.Id] = s
	if m.current == "" {
		m.current = s.Id
	}
	return nil
}

func (m *SessionManagerExec) Get(id api.SessionID) (*SupervisedSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *SessionManagerExec) ListLive() []api.SessionID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]api.SessionID, 0, len(m.sessions))
	for id := range m.sessions {
		out = append(out, id)
	}
	return out
}

func (m *SessionManagerExec) Remove(id api.SessionID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	if m.current == id {
		m.current = "" // caller can SetCurrent to another live session
	}
}

func (m *SessionManagerExec) Current() api.SessionID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

func (m *SessionManagerExec) SetCurrent(id api.SessionID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; !ok {
		return errors.New("unknown session id")
	}
	m.current = id
	return nil
}
