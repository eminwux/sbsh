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

type ClientController interface {
	Run(doc *ClientDoc) error
	WaitReady() error
	Close(reason error) error
	WaitClose() error
	Detach() error
	Ping(ping *PingMessage) (*PingMessage, error)
	Metadata() (*ClientDoc, error)
	State() (*ClientStatusMode, error)
	Stop(args *StopArgs) error
}

type ClientSpec struct {
	ID         ID     `json:"id"`
	LogFile    string `json:"logDir"`
	SockerCtrl string `json:"socketCtrl"`
	RunPath    string `json:"runPath"`

	// Only valid when ClientMode == RunNewTerminal
	TerminalSpec *TerminalSpec `json:"terminal,omitempty"`

	// DetachKeystroke controls whether the detach keystroke (^] twice) is enabled.
	// When true, users can detach by pressing ^] twice. When false, detach keystroke is disabled.
	DetachKeystroke bool `json:"detachKeystroke,omitempty"`

	// ClientMode determines whether the client runs a new terminal or attaches to an existing one.
	ClientMode ClientMode `json:"clientMode,omitempty"`
}

type ClientStatus struct {
	Pid int `json:"pid"`
	// PidStart mirrors TerminalStatus.PidStart: an opaque per-process
	// identifier so signal-based teardown can detect PID reuse.
	PidStart      uint64           `json:"pidStart,omitempty"`
	BaseRunPath   string           `json:"baseRunPath"`
	ClientRunPath string           `json:"clientRunPath"`
	State         ClientStatusMode `json:"state"`
}

type ClientStatusMode int

const (
	ClientInitializing ClientStatusMode = iota
	ClientReady
	ClientAttached
	ClientExiting
	ClientExited
)

func (s ClientStatusMode) String() string {
	switch s {
	case ClientInitializing:
		return "Initializing"
	case ClientReady:
		return "Ready"
	case ClientAttached:
		return "Attached"
	case ClientExiting:
		return "Exiting"
	case ClientExited:
		return "Exited"
	default:
		return "Unknown"
	}
}

type ClientDoc struct {
	APIVersion Version        `json:"apiVersion" yaml:"apiVersion"`
	Kind       Kind           `json:"kind"       yaml:"kind"`
	Metadata   ClientMetadata `json:"metadata"`
	Spec       ClientSpec     `json:"spec"`
	Status     ClientStatus   `json:"status"`
}

type ClientMetadata struct {
	Name        string            `json:"name"                  yaml:"name"`
	Labels      map[string]string `json:"labels,omitempty"      yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

type ClientMode int

const (
	RunNewTerminal ClientMode = iota
	AttachToTerminal
)

type AttachedTerminal struct {
	Spec        *TerminalSpec
	Command     string
	CommandArgs []string // for local: ["bash","-i"]; for ssh: ["ssh","-tt","user@host"]
}

const ClientService = "ClientController"

const (
	ClientMethodDetach   = ClientService + ".Detach"
	ClientMethodPing     = ClientService + ".Ping"
	ClientMethodMetadata = ClientService + ".Metadata"
	ClientMethodState    = ClientService + ".State"
	ClientMethodStop     = ClientService + ".Stop"
)
