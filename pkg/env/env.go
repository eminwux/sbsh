package env

import (
	"os"

	"github.com/spf13/viper"
)

const Prefix = "SBSH"

type Var struct {
	Key        string // e.g. "SBSH_RUN_PATH"
	ViperKey   string // optional, e.g. "global.runPath"
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

// ---- Declare statically (Viper key optional per var) ----
var (
	RUN_PATH        = DefineKV("RUN_PATH", "global.runPath")           // has viper key
	LOG_LEVEL       = DefineKV("LOG_LEVEL", "global.logLevel", "info") // has viper key
	SUP_SOCKET      = DefineKV("SUP_SOCKET", "status.sup.socket")      // no viper key, no default
	SES_SOCKET_CTRL = Define("SES_SOCKET_CTRL")
	SES_SOCKET_IO   = Define("SES_SOCKET_IO")
	SES_ID          = Define("SES_ID")
	SES_NAME        = Define("SES_NAME")
)
