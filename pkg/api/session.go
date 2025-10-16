// Copyright 2025 Emiliano Spinella (eminwux)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package api

type SessionController interface {
	Run(spec *SessionSpec) error
	WaitReady() error
	WaitClose() error
	Ping(ping *PingMessage) (*PingMessage, error)
	Close(reason error) error
	Resize(ResizeArgs)
	Detach(id *ID) error
	Attach(id *ID, reply *ResponseWithFD) error
	Metadata() (*SessionMetadata, error)
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

// Inputs needed to spawn a session; serialize parts of this into sessions.json.
type SessionSpec struct {
	ID     ID                `json:"id"`
	Kind   SessionKind       `json:"kind"`
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`

	Cwd         string   `json:"cwd,omitempty"`
	Command     string   `json:"command"`
	CommandArgs []string `json:"commandArgs"`
	Env         []string `json:"env"`
	Prompt      string   `json:"prompt"`

	RunPath     string `json:"runPath"`
	CaptureFile string `json:"captureFile"`
	LogFile     string `json:"logFile"`
	LogLevel    string `json:"logLevel"`
	SocketFile  string `json:"socketIO"`

	ProfileName string     `json:"profileName"`
	Stages      StagesSpec `json:"stages"`
}

type SessionStatus struct {
	Pid            int               `json:"pid"`
	State          SessionStatusMode `json:"state"`
	SocketFile     string            `json:"socketCtrl"`
	BaseRunPath    string            `json:"baseRunPath"`
	SessionRunPath string            `json:"sessionRunPath"`
	LogFile        string            `json:"logFile"`
	LogLevel       string            `json:"logLevel"`
	CaptureFile    string            `json:"captureFile"`
	Attachers      []string          `json:"attachers"`
}

type SessionStatusMode int

const (
	Initializing SessionStatusMode = iota
	Ready
	Exited
)

func (s SessionStatusMode) String() string {
	switch s {
	case Initializing:
		return "Initializing"
	case Ready:
		return "Ready"
	case Exited:
		return "Exited"
	default:
		return "Unknown"
	}
}

type SessionMetadata struct {
	Spec   SessionSpec   `json:"spec"`
	Status SessionStatus `json:"status"`
}

// SESSION RPC.
const SessionService = "SessionController"

const (
	SessionMethodResize   = SessionService + ".Resize"
	SessionMethodPing     = SessionService + ".Ping"
	SessionMethodAttach   = SessionService + ".Attach"
	SessionMethodDetach   = SessionService + ".Detach"
	SessionMethodMetadata = SessionService + ".Metadata"
)

type PingMessage struct {
	Message string
}

type ResizeArgs struct {
	Cols int
	Rows int
}

// ResponseWithFD carries a normal JSON result plus OOB file descriptors.
type ResponseWithFD struct {
	JSON any   // what to JSON-encode into "result"
	FDs  []int // file descriptors to pass via SCM_RIGHTS
}
