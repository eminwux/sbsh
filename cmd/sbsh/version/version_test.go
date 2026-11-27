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
	"strings"
	"testing"

	"github.com/eminwux/sbsh/cmd/config"
)

func TestNewVersionCmd_Metadata(t *testing.T) {
	cmd := NewVersionCmd()
	if cmd.Use != "version" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "version")
	}
	if cmd.Short == "" {
		t.Fatal("Short should not be empty")
	}
	if cmd.Run == nil {
		t.Fatal("Run should be set")
	}
}

func TestNewVersionCmd_PrintsVersion(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewVersionCmd()
	cmd.SetOut(&buf)
	cmd.Run(cmd, []string{})

	got, err := buf.ReadString('\n')
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if strings.TrimSpace(got) != config.Version {
		t.Fatalf("output = %q, want %q", strings.TrimSpace(got), config.Version)
	}
}
