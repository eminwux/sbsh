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

package env

import (
	"os"

	"github.com/spf13/viper"
)

const Prefix = "SBSH"

type Var struct {
	Key        string // e.g. "SBSH_RUN_PATH"
	ViperKey   string // optional, e.g. "global.runPath"
	CobraKey   string // optional, e.g. "run-path"
	Default    string // optional
	HasDefault bool
}

func DefineKV(envName, viperKey string, defaultVal ...string) Var {
	v := Var{Key: Prefix + "_" + envName, ViperKey: viperKey}
	if len(defaultVal) > 0 {
		v.Default = defaultVal[0]
		v.HasDefault = true
	}
	return v
}

func Define(envName string, defaultVal ...string) Var {
	return DefineKV(envName, "", defaultVal...)
}

func (v Var) EnvKey() string               { return v.Key }
func (v Var) DefaultValue() (string, bool) { return v.Default, v.HasDefault }

// Precedence: viper (if ViperKey set and value present) → OS env → default → "".
func (v Var) ValueOrDefault() string {
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

// Safe if ViperKey is empty: does nothing.
func (v Var) BindEnv() error {
	if v.ViperKey == "" {
		return nil
	}
	return viper.BindEnv(v.ViperKey, v.Key)
}

func (v Var) Set(value string) error { return os.Setenv(v.Key, value) }

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
	RUN_PATH = DefineKV("RUN_PATH", "sbsh.global.runPath") // has viper key
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	CONFIG_FILE = DefineKV("CONFIG_FILE", "sbsh.global.configFile") // has viper key
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	PROFILES_FILE = DefineKV("PROFILES_FILE", "sbsh.global.profilesFile") // has viper key
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	LOG_LEVEL = DefineKV("LOG_LEVEL", "sbsh.global.logLevel", "info") // has viper key
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SUP_SOCKET = DefineKV("SUP_SOCKET", "sbsh.supervisor.socket") // no viper key, no default
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SES_SOCKET_CTRL = Define("SES_SOCKET_CTRL", "sbsh.session.socket")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SES_ID = Define("SES_ID", "sbsh.session.id")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SES_NAME = Define("SES_NAME", "sbsh.session.name")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SES_PROFILE = Define("SES_PROFILE", "sbsh.session.profile")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SES_CMD = Define("SES_CMD", "sbsh.session.command")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SES_LOGFILENAME = Define("SES_LOGFILENAME", "sbsh.session.logFilename")
	//nolint:revive,gochecknoglobals,staticcheck // ignore linter warning about this variable
	SUP_DETACH = Define("SUP_DETACH", "sbsh.supervisor.detach")
)
