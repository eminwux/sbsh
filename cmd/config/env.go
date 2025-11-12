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

package config

import (
	"os"

	"github.com/spf13/viper"
)

type Var struct {
	Key        string // e.g. "SBSH_RUN_PATH"
	ViperKey   string // optional, e.g. "global.runPath"
	CobraKey   string // optional, e.g. "run-path"
	Default    string // optional
	HasDefault bool
}

func DefineKV(envName, viperKey string, defaultVal ...string) Var {
	v := Var{Key: envName, ViperKey: viperKey}
	if len(defaultVal) > 0 {
		v.Default = defaultVal[0]
		v.HasDefault = true
	}
	return v
}

func Define(envName string, defaultVal ...string) Var {
	return DefineKV(envName, "", defaultVal...)
}

func (v *Var) EnvKey() string               { return v.Key }
func (v *Var) EnvVar() string               { return v.Key }
func (v *Var) DefaultValue() (string, bool) { return v.Default, v.HasDefault }

// ValueOrDefault defines precedence: viper (if ViperKey set and value present) → OS env → default → "".
func (v *Var) ValueOrDefault() string {
	if v.ViperKey != "" && viper.IsSet(v.ViperKey) {
		return viper.GetString(v.ViperKey)
	}
	if val, ok := os.LookupEnv(v.Key); ok {
		return val
	}
	if v.HasDefault {
		return v.Default
	}
	return ""
}

// BindEnv is safe if ViperKey is empty: does nothing.
func (v *Var) BindEnv() error {
	if v.ViperKey == "" {
		return nil
	}
	return viper.BindEnv(v.ViperKey, v.Key)
}

func (v *Var) Set(value string) error {
	return os.Setenv(v.Key, value)
}

func (v *Var) SetDefault(val string) {
	v.Default = val
	v.HasDefault = true
	if v.ViperKey != "" {
		viper.SetDefault(v.ViperKey, val)
	}
}

func KV(v Var, value string) string { return v.Key + "=" + value }

// ---- Declare statically (Viper key optional per var) ----.
var (
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_RUN_PATH = DefineKV("SB_RUN_PATH", "sb/runPath")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_PROFILES_FILE = DefineKV("SB_PROFILES_FILE", "sb/get/profilesFile")

	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_CONFIG_FILE = DefineKV("SBSH_CONFIG_FILE", "sbsh/configFile")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_PROFILES_FILE = DefineKV("SBSH_PROFILES_FILE", "sbsh/profilesFile")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_LOG_LEVEL = DefineKV("SBSH_LOG_LEVEL", "sbsh/logLevel", "info")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_SUPER_SOCKET = DefineKV("SBSH_SUP_SOCKET", "sbsh/supervisor.socket") // no viper key, no default
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_TERM_SOCKET = Define("SBSH_TERM_SOCKET", "sbsh/terminal.socket")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_TERM_ID = Define("SBSH_TERM_ID", "sbsh/terminal/id")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_TERM_NAME = Define("SBSH_TERM_NAME", "sbsh/terminal/name")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_TERM_PROFILE = Define("SBSH_TERM_PROFILE", "sbsh/terminal.profile")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_RUN_PATH = DefineKV("SBSH_RUN_PATH", "sbsh/runPath")
)
