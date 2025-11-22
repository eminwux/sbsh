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

type SupervisorController interface {
	Run(doc *SupervisorDoc) error
	WaitReady() error
	Close(reason error) error
	WaitClose() error
	Detach() error
}

type SupervisorSpec struct {
	ID         ID     `json:"id"`
	LogFile    string `json:"logDir"`
	SockerCtrl string `json:"socketCtrl"`
	RunPath    string `json:"runPath"`

	// Only valid when SupervisorMode == RunNewTerminal
	TerminalSpec *TerminalSpec `json:"terminal,omitempty"`

	// DetachKeystroke controls whether the detach keystroke (^] twice) is enabled.
	// When true, users can detach by pressing ^] twice. When false, detach keystroke is disabled.
	DetachKeystroke bool `json:"detachKeystroke,omitempty"`

	// SupervisorMode determines whether the supervisor runs a new terminal or attaches to an existing one.
	SupervisorMode SupervisorMode `json:"supervisorMode,omitempty"`
}

type SupervisorStatus struct {
	Pid               int                  `json:"pid"`
	BaseRunPath       string               `json:"baseRunPath"`
	SupervisorRunPath string               `json:"supervisorRunPath"`
	State             SupervisorStatusMode `json:"state"`
}

type SupervisorStatusMode int

const (
	SupervisorInitializing SupervisorStatusMode = iota
	SupervisorReady
	SupervisorAttached
	SupervisorExiting
	SupervisorExited
)

func (s SupervisorStatusMode) String() string {
	switch s {
	case SupervisorInitializing:
		return "Initializing"
	case SupervisorReady:
		return "Ready"
	case SupervisorAttached:
		return "Attached"
	case SupervisorExiting:
		return "Exiting"
	case SupervisorExited:
		return "Exited"
	default:
		return "Unknown"
	}
}

type SupervisorDoc struct {
	APIVersion Version            `json:"apiVersion" yaml:"apiVersion"`
	Kind       Kind               `json:"kind"       yaml:"kind"`
	Metadata   SupervisorMetadata `json:"metadata"`
	Spec       SupervisorSpec     `json:"spec"`
	Status     SupervisorStatus   `json:"status"`
}

type SupervisorMetadata struct {
	Name        string            `json:"name"                  yaml:"name"`
	Labels      map[string]string `json:"labels,omitempty"      yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

type SupervisorMode int

const (
	RunNewTerminal SupervisorMode = iota
	AttachToTerminal
)

type SupervisedTerminal struct {
	Spec        *TerminalSpec
	Command     string
	CommandArgs []string // for local: ["bash","-i"]; for ssh: ["ssh","-tt","user@host"]
}

const SupervisorService = "SupervisorController"

const (
	SupervisorMethodDetach = SupervisorService + ".Detach"
)
