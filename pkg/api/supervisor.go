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
	Run(spec *SupervisorSpec) error
	WaitReady() error
	Close(reason error) error
	WaitClose() error
	Detach() error
}

type SupervisorSpec struct {
	ID         ID                `json:"id"`
	Kind       SupervisorKind    `json:"kind"`
	Name       string            `json:"name"`
	Env        []string          `json:"env"`
	Labels     map[string]string `json:"context"`
	LogDir     string            `json:"logDir"`
	SockerCtrl string            `json:"socketCtrl"`
	Pid        int               `json:"pid"`
	RunPath    string            `json:"runPath"`

	// Only valid when Kind == AttachToSession
	AttachID   ID     `json:"attachId,omitempty"`
	AttachName string `json:"attachName,omitempty"`
}

type SupervisorKind int

const (
	RunNewSession SupervisorKind = iota
	AttachToSession
)

type SupervisedSession struct {
	Id          ID
	Kind        SessionKind
	Name        string // user-friendly name
	Command     string
	CommandArgs []string          // for local: ["bash","-i"]; for ssh: ["ssh","-tt","user@host"]
	Env         []string          // TERM, COLORTERM, etc.
	Context     map[string]string // kubectl ns, cwd hint, etc.
	LogFilename string
	SocketCtrl  string
	SocketIO    string
	Pid         int
	Prompt      string
}

// SUPERVISOR RPC
const SupervisorService = "SupervisorController"

const (
	SupervisorMethodDetach = SupervisorService + ".Detach"
)
