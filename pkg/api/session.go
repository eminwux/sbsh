package api

type SessionController interface {
	Run(spec *SessionSpec) error
	WaitReady() error
	WaitClose() error
	Status() string
	Close(reason error) error
	Resize(ResizeArgs)
	Detach() error
}

type SessionState int

const (
	SessionBash SessionState = iota
)

type SessionKind int

const (
	SessionLocal SessionKind = iota // /bin/bash -i
	SessSSH                         // ssh -tt user@host ...
)

// Inputs needed to spawn a session; serialize parts of this into sessions.json
type SessionSpec struct {
	ID          ID                `json:"id"`
	Kind        SessionKind       `json:"kind"`
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	CommandArgs []string          `json:"commandArgs"`
	Env         []string          `json:"env"`
	Labels      map[string]string `json:"labels"`
	LogFilename string            `json:"logFile"`
	SockerCtrl  string            `json:"socketCtrl"`
	SocketIO    string            `json:"socketIO"`
	RunPath     string            `json:"runPath"`
}

type SessionStatus struct {
	Pid   int               `json:"pid"`
	State SessionStatusMode `json:"state"`
}

type SessionStatusMode int

const (
	SessionStatusNew SessionStatusMode = iota
	SessionStatusInitializing
	SessionStatusAttached
	SessionStatusDetached
)

type SessionMetadata struct {
	Spec   *SessionSpec   `json:"spec"`
	Status *SessionStatus `json:"status"`
}

// SESSION RPC
const SessionService = "SessionController"

const (
	SessionMethodResize = SessionService + ".Resize"
	SessionMethodStatus = SessionService + ".Status"
	SessionMethodDetach = SessionService + ".Detach"
)

type SessionStatusMessage struct {
	Message string
}

type ResizeArgs struct {
	Cols int
	Rows int
}
