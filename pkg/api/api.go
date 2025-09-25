package api

import (
	"context"
	"time"
)

type SupervisorController interface {
	Run(spec *SupervisorSpec) error
	WaitReady(ctx context.Context) error
	SetCurrentSession(id ID) error
	Close(reason error) error
	WaitClose() error
}

/////////////// SESSION

type SessionController interface {
	Run(spec *SessionSpec) error
	WaitReady() error
	WaitClose() error
	Status() string
	Close(reason error) error
	Resize(ResizeArgs)
}

// Identity & lifecycle
type ID string

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
	ID          ID                `json:"id"`
	Kind        SessionKind       `json:"kind"`
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	CommandArgs []string          `json:"commandArgs"`
	Env         []string          `json:"env"`
	Labels      map[string]string `json:"context"`
	LogDir      string            `json:"logDir"`
	SockerCtrl  string            `json:"socketCtrl"`
	SocketIO    string            `json:"socketIO"`
	Pid         int               `json:"pid"`
	RunPath     string            `json:"runPath"`
}

type SupervisorSpec struct {
	ID         ID                `json:"id"`
	Kind       SessionKind       `json:"kind"`
	Name       string            `json:"name"`
	Env        []string          `json:"env"`
	Labels     map[string]string `json:"context"`
	LogDir     string            `json:"logDir"`
	SockerCtrl string            `json:"socketCtrl"`
	Pid        int               `json:"pid"`
	RunPath    string            `json:"runPath"`
}
type SessionEventType int

const (
	EvData   SessionEventType = iota // optional metrics
	EvClosed                         // PTY closed / child exited
	EvError                          // abnormal error
	EvSessionExited
)

type SessionEvent struct {
	ID    ID
	Type  SessionEventType
	Bytes int   // for EvData
	Err   error // for EvClosed/EvError
	When  time.Time
}

// ////////////////// FOR RPC
type Empty struct{}

type SessionIDArg struct {
	ID ID
}

type SessionStatus struct {
	Message string
}

type ResizeArgs struct {
	Cols int
	Rows int
}
