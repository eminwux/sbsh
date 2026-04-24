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

import "time"

type TerminalController interface {
	Run(spec *TerminalSpec) error
	WaitReady() error
	WaitClose() error
	Ping(ping *PingMessage) (*PingMessage, error)
	Close(reason error) error
	Resize(ResizeArgs)
	Detach(id *ID) error
	Attach(id *ID, reply *ResponseWithFD) error
	Metadata() (*TerminalDoc, error)
	State() (*TerminalStatusMode, error)
	Stop(args *StopArgs) error
	Write(req *WriteRequest) error
	Subscribe(req *SubscribeRequest, reply *ResponseWithFD) error
}

type TerminalDoc struct {
	APIVersion Version          `json:"apiVersion" yaml:"apiVersion"`
	Kind       Kind             `json:"kind"       yaml:"kind"`
	Metadata   TerminalMetadata `json:"metadata"`
	Spec       TerminalSpec     `json:"spec"`
	Status     TerminalStatus   `json:"status"`
}

type TerminalMetadata struct {
	Name        string            `json:"name"                  yaml:"name"`
	Labels      map[string]string `json:"labels,omitempty"      yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"createdAt,omitzero"    yaml:"createdAt,omitempty"`
}

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
	SetPrompt   bool     `json:"setPrompt,omitempty"`

	RunPath     string `json:"runPath"`
	CaptureFile string `json:"captureFile"`
	LogFile     string `json:"logFile"`
	LogLevel    string `json:"logLevel"`
	SocketFile  string `json:"socketIO"`

	ProfileName string     `json:"profileName"`
	Stages      StagesSpec `json:"stages"`

	// ShutdownGrace bounds how long the terminal waits for the child to
	// exit after SIGTERM before escalating to SIGKILL. Zero means "use the
	// runner's default" (30s, matching Kubernetes terminationGracePeriodSeconds).
	ShutdownGrace time.Duration `json:"shutdownGrace,omitempty"`
}

type TerminalStatus struct {
	Pid int `json:"pid"`
	// PidStart is an opaque per-process identifier captured at metadata
	// write time so callers that later signal Pid can reject a recycled
	// PID. Zero means "no token recorded" and consumers should treat the
	// PID as unverifiable. See internal/pidutil.
	PidStart        uint64             `json:"pidStart,omitempty"`
	Tty             string             `json:"tty"`
	State           TerminalStatusMode `json:"state"`
	SocketFile      string             `json:"socketCtrl"`
	BaseRunPath     string             `json:"baseRunPath"`
	TerminalRunPath string             `json:"terminalRunPath"`
	LogFile         string             `json:"logFile"`
	LogLevel        string             `json:"logLevel"`
	CaptureFile     string             `json:"captureFile"`
	Attachers       []string           `json:"attachers"`
	LastAttachedAt  time.Time          `json:"lastAttachedAt,omitzero" yaml:"lastAttachedAt,omitempty"`
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

type TerminalStatusMode int

const (
	// Initializing internal state and server startup.
	Initializing TerminalStatusMode = iota
	// Starting PTY and command execution.
	Starting
	// SettingUp shell environment.
	SettingUp
	// OnInit waiting for shell to be initialized.
	OnInit
	// Ready terminal is fully initialized and ready for use.
	Ready
	// PostAttach waiting for shell to be attached.
	PostAttach
	// Exited terminal has exited.
	Exited
)

func (s TerminalStatusMode) String() string {
	switch s {
	case Initializing:
		return "Initializing"
	case Starting:
		return "Starting"
	case SettingUp:
		return "SettingUp"
	case OnInit:
		return "OnInit"
	case Ready:
		return "Ready"
	case PostAttach:
		return "PostAttach"
	case Exited:
		return "Exited"
	default:
		return "Unknown"
	}
}

// RPC definitions

const TerminalService = "TerminalController"

const (
	TerminalMethodResize    = TerminalService + ".Resize"
	TerminalMethodPing      = TerminalService + ".Ping"
	TerminalMethodAttach    = TerminalService + ".Attach"
	TerminalMethodDetach    = TerminalService + ".Detach"
	TerminalMethodMetadata  = TerminalService + ".Metadata"
	TerminalMethodState     = TerminalService + ".State"
	TerminalMethodStop      = TerminalService + ".Stop"
	TerminalMethodWrite     = TerminalService + ".Write"
	TerminalMethodSubscribe = TerminalService + ".Subscribe"
)

type PingMessage struct {
	Message string
}

type ResizeArgs struct {
	Cols int
	Rows int
}

type StopArgs struct {
	Reason string
}

// WriteRequest carries bytes to push into a terminal's PTY input.
// Bytes are sent verbatim — the CLI layer is responsible for any
// caret notation or hex-escape interpretation.
type WriteRequest struct {
	Data []byte
}

// SubscribeRequest identifies the caller of a Subscribe RPC. ClientID
// is used only for server-side logging and attacher accounting; it does
// not appear in TerminalStatus.Attachers.
type SubscribeRequest struct {
	ClientID ID
}

// ResponseWithFD carries a normal JSON result plus OOB file descriptors.
type ResponseWithFD struct {
	JSON any   // what to JSON-encode into "result"
	FDs  []int // file descriptors to pass via SCM_RIGHTS
}
