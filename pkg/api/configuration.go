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
// kind: Configuration

const KindConfiguration Kind = "Configuration"

// ConfigurationDoc models one YAML document containing user-level defaults
// for sbsh. It is the declarative equivalent of the --run-path,
// --profiles-dir, and --log-level flags and their corresponding environment
// variables.
type ConfigurationDoc struct {
	APIVersion Version               `json:"apiVersion" yaml:"apiVersion"`
	Kind       Kind                  `json:"kind"       yaml:"kind"`
	Metadata   ConfigurationMetadata `json:"metadata"   yaml:"metadata"`
	Spec       ConfigurationSpec     `json:"spec"       yaml:"spec"`
}

type ConfigurationMetadata struct {
	Name        string            `json:"name,omitempty"        yaml:"name,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"      yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// ConfigurationSpec holds the user-level defaults loaded from config.yaml.
// Fields are optional; empty values fall back to built-in defaults.
type ConfigurationSpec struct {
	RunPath     string `json:"runPath,omitempty"     yaml:"runPath,omitempty"`
	ProfilesDir string `json:"profilesDir,omitempty" yaml:"profilesDir,omitempty"`
	LogLevel    string `json:"logLevel,omitempty"    yaml:"logLevel,omitempty"`
}
