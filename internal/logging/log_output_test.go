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

package logging

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/cmd/types"
	"github.com/spf13/cobra"
)

func TestSetupLogger_StdoutPopulatesContextWithoutFile(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	if err := SetupLogger(cmd, "stdout", "", "info"); err != nil {
		t.Fatalf("SetupLogger(stdout) error: %v", err)
	}
	for _, key := range []types.CtxLoggerType{types.CtxLogger, types.CtxLevelVar, types.CtxHandler} {
		if cmd.Context().Value(key) == nil {
			t.Errorf("%v not set in context", key)
		}
	}
}

func TestSetupLogger_RejectsInvalidCombinations(t *testing.T) {
	logfile := filepath.Join(t.TempDir(), "log")
	cases := []struct {
		name, output, logfile, level string
	}{
		{"stdout with logfile", "stdout", logfile, "info"},
		{"stdout without level", "stdout", "", ""},
		{"file without logfile", "file", "", "info"},
		{"unknown output", "stderr", "", "info"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			if err := SetupLogger(cmd, tc.output, tc.logfile, tc.level); err == nil {
				t.Fatalf("SetupLogger(%q, %q, %q) = nil; want error", tc.output, tc.logfile, tc.level)
			}
		})
	}
}

func TestBufferedLogger_FlushReplaysInOrderAtLevel(t *testing.T) {
	b := NewBufferedLogger("info")
	b.Debug("dropped at info")
	b.Info("first", "k", 1)
	b.Warn("second")

	var out bytes.Buffer
	dst := slog.New(&ReformatHandler{
		Inner:  slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}),
		Writer: &out,
	})
	b.Flush(context.Background(), dst)

	got := out.String()
	if strings.Contains(got, "dropped at info") {
		t.Errorf("debug record buffered at info level: %q", got)
	}
	first, second := strings.Index(got, `INFO "first" k=1`), strings.Index(got, `WARN "second"`)
	if first < 0 || second < 0 || first > second {
		t.Fatalf("replay = %q; want INFO first then WARN second", got)
	}

	out.Reset()
	b.Flush(context.Background(), dst)
	if out.Len() != 0 {
		t.Errorf("second Flush replayed %q; want empty buffer", out.String())
	}
}
