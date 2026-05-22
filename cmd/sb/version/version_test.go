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

package version

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/eminwux/sbsh/cmd/config"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	var buf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		_, _ = io.Copy(&buf, r)
		wg.Done()
	}()

	fn()

	_ = w.Close()
	os.Stdout = oldStdout
	wg.Wait()
	_ = r.Close()
	return buf.String()
}

func Test_NewVersionCmd_Metadata(t *testing.T) {
	cmd := NewVersionCmd()
	if cmd == nil {
		t.Fatal("NewVersionCmd returned nil")
	}
	if cmd.Use != "version" {
		t.Errorf("Use = %q, want %q", cmd.Use, "version")
	}
	if cmd.Short == "" {
		t.Error("Short description should not be empty")
	}
}

func Test_VersionCmd_PrintsVersion(t *testing.T) {
	cmd := NewVersionCmd()
	out := captureStdout(t, func() {
		cmd.Run(cmd, []string{})
	})
	got := strings.TrimSpace(out)
	if got != config.Version {
		t.Errorf("printed version = %q, want %q", got, config.Version)
	}
}
