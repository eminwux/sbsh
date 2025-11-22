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
	SB_ROOT_CONFIG = DefineKV("SB_CONFIG", "sb/config")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_ROOT_VERBOSE = DefineKV("SB_VERBOSE", "sb/verbose")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_ROOT_LOG_LEVEL = DefineKV("SB_LOG_LEVEL", "sb/logLevel")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_ROOT_RUN_PATH = DefineKV("SB_RUN_PATH", "sb/runPath")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_GET_PROFILES_FILE = DefineKV("SB_PROFILES_FILE", "sb/get/profilesFile")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_GET_PROFILES_OUTPUT = DefineKV("SB_PROFILES_OUTPUT", "sb/get/profiles/output")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_GET_SUPERVISORS_ALL = DefineKV("SB_SUPERVISORS_ALL", "sb/get/supervisors/all")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_GET_SUPERVISORS_OUTPUT = DefineKV("SB_SUPERVISORS_OUTPUT", "sb/get/supervisors/output")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_ATTACH_ID = DefineKV("SB_ATTACH_ID", "sb/attach/id")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_ATTACH_NAME = DefineKV("SB_ATTACH_NAME", "sb/attach/name")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_ATTACH_SOCKET = DefineKV("SB_ATTACH_SOCKET", "sb/attach/socket")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_ATTACH_DISABLE_DETACH_KEYSTROKE = DefineKV(
		"SB_ATTACH_DISABLE_DETACH_KEYSTROKE",
		"sb/attach/disableDetachKeystroke",
	)
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_DETACH_NAME = DefineKV("SB_DETACH_NAME", "sb/detach/name")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_DETACH_ID = DefineKV("SB_DETACH_ID", "sb/detach/id")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SB_DETACH_SOCKET = DefineKV("SB_DETACH_SOCKET", "sb/detach/socket")

	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_ROOT_CONFIG_FILE = DefineKV("SBSH_CONFIG_FILE", "sbsh/configFile")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_ROOT_PROFILES_FILE = DefineKV("SBSH_PROFILES_FILE", "sbsh/profilesFile")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_ROOT_LOG_LEVEL = DefineKV("SBSH_LOG_LEVEL", "sbsh/logLevel", "info")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_ROOT_SUPER_SOCKET = DefineKV("SBSH_SUP_SOCKET", "sbsh/supervisor.socket")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_ROOT_RUN_PATH = DefineKV("SBSH_RUN_PATH", "sbsh/runPath")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_ROOT_TERM_ID = DefineKV("SBSH_ROOT_TERM_ID", "sbsh.root.terminal.id")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_ROOT_TERM_COMMAND = DefineKV("SBSH_ROOT_TERM_COMMAND", "sbsh.root.terminal.command")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_ROOT_TERM_NAME = DefineKV("SBSH_ROOT_TERM_NAME", "sbsh.root.terminal.name")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_ROOT_TERM_CAPTURE_FILE = DefineKV("SBSH_ROOT_TERM_CAPTURE_FILE", "sbsh.root.terminal.captureFile")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_ROOT_TERM_LOG_FILE = DefineKV("SBSH_ROOT_TERM_LOG_FILE", "sbsh.root.terminal.logFile")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_ROOT_TERM_LOG_LEVEL = DefineKV("SBSH_ROOT_TERM_LOG_LEVEL", "sbsh.root.terminal.logLevel")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_ROOT_TERM_PROFILE = DefineKV("SBSH_ROOT_TERM_PROFILE", "sbsh.root.terminal.profile")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_ROOT_TERM_SOCKET = DefineKV("SBSH_ROOT_TERM_SOCKET", "sbsh.root.terminal.socket")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_SUPERVISOR_ID = DefineKV("SBSH_SUPERVISOR_ID", "sbsh.supervisor.id")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_SUPERVISOR_NAME = DefineKV("SBSH_SUPERVISOR_NAME", "sbsh.supervisor.name")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_SUPERVISOR_SOCKET = DefineKV("SBSH_SUPERVISOR_SOCKET", "sbsh.supervisor.socket")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_SUPERVISOR_LOG_FILE = DefineKV("SBSH_SUPERVISOR_LOG_FILE", "sbsh.supervisor.logFile")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_SUPERVISOR_LOG_LEVEL = DefineKV("SBSH_SUPERVISOR_LOG_LEVEL", "sbsh.supervisor.logLevel")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_SUPERVISOR_DETACH = DefineKV("SBSH_SUPERVISOR_DETACH", "sbsh.supervisor.detach")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_SUPERVISOR_DISABLE_DETACH_KEYSTROKE = DefineKV(
		"SBSH_SUPERVISOR_DISABLE_DETACH_KEYSTROKE",
		"sbsh.supervisor.disableDetachKeystroke",
	)

	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_TERM_SOCKET = DefineKV("SBSH_TERM_SOCKET", "sbsh.terminal.socket")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_TERM_ID = DefineKV("SBSH_TERM_ID", "sbsh.terminal.id")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_TERM_NAME = DefineKV("SBSH_TERM_NAME", "sbsh.terminal.name")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_TERM_PROFILE = DefineKV("SBSH_TERM_PROFILE", "sbsh.terminal.profile")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_TERM_COMMAND = DefineKV("SBSH_TERM_COMMAND", "sbsh.terminal.command")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_TERM_CAPTURE_FILE = DefineKV("SBSH_TERM_CAPTURE_FILE", "sbsh.terminal.captureFile")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_TERM_LOG_FILE = DefineKV("SBSH_TERM_LOG_FILE", "sbsh.terminal.logFile")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_TERM_LOG_LEVEL = DefineKV("SBSH_TERM_LOG_LEVEL", "sbsh.terminal.logLevel")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_TERM_TERMINAL_LOG = DefineKV("SBSH_TERM_TERMINAL_LOG", "sbsh.terminal.terminalLog")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SBSH_TERM_SPEC = DefineKV("SBSH_TERM_SPEC", "sbsh.terminal.spec")
)
