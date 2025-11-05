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

// apiVersion: sbsh/v1beta1
// kind: TerminalProfile

type (
	Version string
	Kind    string
)

const (
	APIVersionV1Beta1   Version = "sbsh/v1beta1"
	KindTerminalProfile Kind    = "TerminalProfile"
)

type (
	RunTarget     string
	RestartPolicy string
)

const (
	RunTargetLocal   RunTarget     = "local" // current scope; future: "docker", "kubernetes"
	RestartExit      RestartPolicy = "exit"
	RestartUnlimited RestartPolicy = "restart-unlimited"
	RestartOnError   RestartPolicy = "restart-on-error"
)

// TerminalProfileDoc models one YAML document containing a TerminalProfile.
type TerminalProfileDoc struct {
	APIVersion Version                 `json:"apiVersion" yaml:"apiVersion"`
	Kind       Kind                    `json:"kind"       yaml:"kind"`
	Metadata   TerminalProfileMetadata `json:"metadata"   yaml:"metadata"`
	Spec       TerminalProfileSpec     `json:"spec"       yaml:"spec"`
}

type TerminalProfileMetadata struct {
	Name        string            `json:"name"                  yaml:"name"`
	Labels      map[string]string `json:"labels,omitempty"      yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

type TerminalProfileSpec struct {
	RunTarget     RunTarget     `json:"runTarget"     yaml:"runTarget"`
	RestartPolicy RestartPolicy `json:"restartPolicy" yaml:"restartPolicy"`
	Shell         ShellSpec     `json:"shell"         yaml:"shell"`
	Stages        StagesSpec    `json:"stages"        yaml:"stages"`
}

// ShellSpec describes the base interactive process that owns the terminal lifetime.
type ShellSpec struct {
	Cwd        string            `json:"cwd,omitempty"        yaml:"cwd,omitempty"`
	Cmd        string            `json:"cmd"                  yaml:"cmd"`
	CmdArgs    []string          `json:"cmdArgs,omitempty"    yaml:"cmdArgs,omitempty"`
	Env        map[string]string `json:"env,omitempty"        yaml:"env,omitempty"`
	EnvInherit bool              `json:"inheritEnv,omitempty" yaml:"inheritEnv,omitempty"`
	Prompt     string            `json:"prompt,omitempty"     yaml:"prompt,omitempty"`
}

// StagesSpec groups lifecycle hooks. For this schema we only need onInit.
type StagesSpec struct {
	OnInit     []ExecStep `json:"onInit,omitempty"     yaml:"onInit,omitempty"`
	PostAttach []ExecStep `json:"postAttach,omitempty" yaml:"postAttach,omitempty"`
}

// ExecStep runs a command (argv form via cmd + cmdArgs) before the first attach.
type ExecStep struct {
	Script string            `json:"script"        yaml:"script"`
	Env    map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
}
