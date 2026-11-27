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

package sbsh

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/pkg/api"
	"github.com/spf13/viper"
)

// The interactive client hands the terminal child its own stdout (the
// user's TTY), so a profile with logOutput: stdout must be downgraded to a
// log file with exactly one warning — even if SBSH_TERM_LOG_OUTPUT=stdout
// is exported, since that variable is scoped to standalone `sbsh terminal`.
func Test_buildClientTerminalSpec_StdoutProfileDowngradesToFile(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		if strings.HasPrefix(k, "SBSH_TERM_") || strings.HasPrefix(k, "SBSH_ROOT_") {
			t.Setenv(k, "")
		}
	}
	t.Setenv(config.SBSH_TERM_LOG_OUTPUT.EnvVar(), "stdout")

	profilesDir := t.TempDir()
	const yaml = `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: stdoutprof
spec:
  runTarget: local
  shell:
    cmd: /bin/sh
  logOutput: stdout
`
	if err := os.WriteFile(filepath.Join(profilesDir, "p.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	viper.Set(config.SBSH_ROOT_RUN_PATH.ViperKey, t.TempDir())
	viper.Set(config.SBSH_ROOT_PROFILES_DIR.ViperKey, profilesDir)
	viper.Set(config.SBSH_ROOT_TERM_PROFILE.ViperKey, "stdoutprof")

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	cmd, err := NewSbshRootCmd()
	if err != nil {
		t.Fatalf("NewSbshRootCmd: %v", err)
	}
	cmd.SetContext(context.Background())

	spec, err := buildClientTerminalSpec(cmd, logger)
	if err != nil {
		t.Fatalf("buildClientTerminalSpec() error = %v", err)
	}
	if spec.LogOutput != api.LogOutputFile {
		t.Fatalf("LogOutput = %q; want file on the client path", spec.LogOutput)
	}
	if spec.LogFile == "" {
		t.Fatal("LogFile is empty; want the default per-terminal log path")
	}
	if n := strings.Count(logBuf.String(), "level=WARN"); n != 1 {
		t.Fatalf("got %d warnings; want exactly 1:\n%s", n, logBuf.String())
	}

	// The child `sbsh terminal -` receives this spec on stdin and logs
	// where it says, so the serialized form must carry "file".
	b, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"logOutput":"file"`) {
		t.Fatalf("serialized spec %s does not pin logOutput to file", b)
	}
}
