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
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/pkg/api"
)

func Test_LoadConfigurationDoc_Missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	doc, err := LoadConfigurationDoc(path)
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if doc != nil {
		t.Fatalf("expected nil doc for missing file, got %+v", doc)
	}
}

func Test_LoadConfigurationDoc_EmptyPath(t *testing.T) {
	doc, err := LoadConfigurationDoc("")
	if err != nil {
		t.Fatalf("expected no error for empty path, got %v", err)
	}
	if doc != nil {
		t.Fatalf("expected nil doc for empty path, got %+v", doc)
	}
}

func Test_LoadConfigurationDoc_HappyPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `apiVersion: sbsh/v1beta1
kind: Configuration
metadata:
  name: default
spec:
  runPath: /var/run/sbsh
  profilesFile: /etc/sbsh/profiles.yaml
  logLevel: debug
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	doc, err := LoadConfigurationDoc(path)
	if err != nil {
		t.Fatalf("LoadConfigurationDoc() error = %v", err)
	}
	if doc == nil {
		t.Fatal("expected doc, got nil")
	}

	if got, want := doc.APIVersion, api.APIVersionV1Beta1; got != want {
		t.Errorf("APIVersion = %q, want %q", got, want)
	}
	if got, want := doc.Kind, api.KindConfiguration; got != want {
		t.Errorf("Kind = %q, want %q", got, want)
	}
	if got, want := doc.Spec.RunPath, "/var/run/sbsh"; got != want {
		t.Errorf("Spec.RunPath = %q, want %q", got, want)
	}
	if got, want := doc.Spec.ProfilesFile, "/etc/sbsh/profiles.yaml"; got != want {
		t.Errorf("Spec.ProfilesFile = %q, want %q", got, want)
	}
	if got, want := doc.Spec.LogLevel, "debug"; got != want {
		t.Errorf("Spec.LogLevel = %q, want %q", got, want)
	}
}

func Test_LoadConfigurationDoc_UnsupportedAPIVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `apiVersion: sbsh/v9999
kind: Configuration
spec:
  runPath: /var/run/sbsh
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadConfigurationDoc(path); err == nil {
		t.Fatal("expected error for unsupported apiVersion, got nil")
	}
}

func Test_LoadConfigurationDoc_UnsupportedKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `apiVersion: sbsh/v1beta1
kind: TerminalProfile
metadata:
  name: default
spec:
  runTarget: local
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadConfigurationDoc(path); err == nil {
		t.Fatal("expected error for unsupported kind, got nil")
	}
}

func Test_LoadConfigurationDoc_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	doc, err := LoadConfigurationDoc(path)
	if err != nil {
		t.Fatalf("LoadConfigurationDoc() error = %v", err)
	}
	if doc != nil {
		t.Fatalf("expected nil doc for empty file, got %+v", doc)
	}
}

func Test_LoadConfigurationDoc_Malformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `apiVersion: sbsh/v1beta1
kind: Configuration
spec:
  runPath: [not-a-string
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadConfigurationDoc(path); err == nil {
		t.Fatal("expected decode error, got nil")
	}
}
