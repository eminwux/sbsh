package sessionstore

import (
	"errors"
	"net/rpc"
	"sbsh/pkg/api"
	"sbsh/pkg/errdefs"
	"sync"
)

type SessionStore interface {
	Add(s *SupervisedSession) error
	Get(id api.ID) (*SupervisedSession, bool)
	ListLive() []api.ID
	Remove(id api.ID)
	Current() api.ID
	SetCurrent(id api.ID) error
}

type SessionStoreExec struct {
	mu       sync.RWMutex
	sessions map[api.ID]*SupervisedSession
	current  api.ID
}

type SupervisedSession struct {
	Id               api.ID
	Kind             api.SessionKind
	Name             string // user-friendly name
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

func NewSessionStoreExec() SessionStore {
	return &SessionStoreExec{
		sessions: make(map[api.ID]*SupervisedSession),
	}
}

func NewSupervisedSession(spec *api.SessionSpec) *SupervisedSession {
	return &SupervisedSession{
		Id:          spec.ID,
		Kind:        spec.Kind,
		Name:        spec.Name,
		Command:     spec.Command,
		CommandArgs: spec.CommandArgs,
		Env:         spec.Env,
		LogDir:      spec.LogDir,
		SockerCtrl:  spec.SockerCtrl,
		SocketIO:    spec.SocketIO,
	}
}

/* Basic ops */

func (m *SessionStoreExec) Add(s *SupervisedSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions[s.Id] != nil {
		return errdefs.ErrSessionExists
	}
	m.sessions[s.Id] = s
	if m.current == "" {
		m.current = s.Id
	}
	return nil
}

func (m *SessionStoreExec) Get(id api.ID) (*SupervisedSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *SessionStoreExec) ListLive() []api.ID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]api.ID, 0, len(m.sessions))
	for id := range m.sessions {
		out = append(out, id)
	}
	return out
}

func (m *SessionStoreExec) Remove(id api.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	if m.current == id {
		m.current = "" // caller can SetCurrent to another live session
	}
}

func (m *SessionStoreExec) Current() api.ID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

func (m *SessionStoreExec) SetCurrent(id api.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; !ok {
		return errors.New("unknown session id")
	}
	m.current = id
	return nil
}
