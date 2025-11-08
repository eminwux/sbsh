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

type TerminalController interface {
	Run(spec *TerminalSpec) error
	WaitReady() error
	WaitClose() error
	Ping(ping *PingMessage) (*PingMessage, error)
	Close(reason error) error
	Resize(ResizeArgs)
	Detach(id *ID) error
	Attach(id *ID, reply *ResponseWithFD) error
	Metadata() (*TerminalMetadata, error)
	State() (*TerminalStatusMode, error)
}

type TerminalState int

const (
	TerminalBash TerminalState = iota
)

type TerminalKind int

const (
	TerminalLocal TerminalKind = iota
	TerminalSSH
)

// TerminalSpec defines how to run a terminal.
type TerminalSpec struct {
	ID     ID                `json:"id"`
	Kind   TerminalKind      `json:"kind"`
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`

	Cwd         string   `json:"cwd,omitempty"`
	Command     string   `json:"command"`
	CommandArgs []string `json:"commandArgs"`
	EnvInherit  bool     `json:"inheritEnv"`
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

type TerminalStatus struct {
	Pid             int                `json:"pid"`
	Tty             string             `json:"tty"`
	State           TerminalStatusMode `json:"state"`
	SocketFile      string             `json:"socketCtrl"`
	BaseRunPath     string             `json:"baseRunPath"`
	TerminalRunPath string             `json:"terminalRunPath"`
	LogFile         string             `json:"logFile"`
	LogLevel        string             `json:"logLevel"`
	CaptureFile     string             `json:"captureFile"`
	Attachers       []string           `json:"attachers"`
}

type TerminalStatusMode int

const (
	Initializing TerminalStatusMode = iota
	Starting
	Ready
	Exited
)

func (s TerminalStatusMode) String() string {
	switch s {
	case Initializing:
		return "Initializing"
	case Starting:
		return "Starting"
	case Ready:
		return "Ready"
	case Exited:
		return "Exited"
	default:
		return "Unknown"
	}
}

type TerminalMetadata struct {
	Spec   TerminalSpec   `json:"spec"`
	Status TerminalStatus `json:"status"`
}

// RPC definitions

const TerminalService = "TerminalController"

const (
	TerminalMethodResize   = TerminalService + ".Resize"
	TerminalMethodPing     = TerminalService + ".Ping"
	TerminalMethodAttach   = TerminalService + ".Attach"
	TerminalMethodDetach   = TerminalService + ".Detach"
	TerminalMethodMetadata = TerminalService + ".Metadata"
	TerminalMethodState    = TerminalService + ".State"
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
