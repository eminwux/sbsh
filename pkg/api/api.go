package api

import (
	"context"
	"time"
)

type Controller interface {
	Run(ctx context.Context) error
	GetSessionEventChannel() chan<- SessionEvent
	WaitReady(ctx context.Context) error
	Stop()
	AddSession(s *SessionSpec)
	SetCurrentSession(id SessionID) error
	StartSession(id SessionID) error
}

/////////////// SESSION

// Identity & lifecycle
type SessionID string

type SessionState int

const (
	SessBash SessionState = iota
	SessSupervisor
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
