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

package shared

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteMetadata_DefaultNameUnchanged locks the default file name and
// temp pattern: WriteMetadata still writes metadata.json and the atomic
// write still stages through ".meta-*.tmp".
func TestWriteMetadata_DefaultNameUnchanged(t *testing.T) {
	dir := t.TempDir()
	if err := WriteMetadata(context.Background(), map[string]int{"n": 1}, dir); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "metadata.json")); err != nil {
		t.Fatalf("metadata.json not written: %v", err)
	}
	if got := metadataTempPattern(DefaultMetadataFile); got != ".meta-*.tmp" {
		t.Fatalf("default temp pattern = %q, want .meta-*.tmp", got)
	}
	assertOnlyFiles(t, dir, "metadata.json")
}

// TestWriteMetadataFile_CustomName asserts a custom name is used for the
// document and as the temp-file prefix, and that metadata.json is not
// written alongside it.
func TestWriteMetadataFile_CustomName(t *testing.T) {
	dir := t.TempDir()
	in := map[string]string{"kind": "Terminal"}
	if err := WriteMetadataFile(context.Background(), in, dir, "terminal.json"); err != nil {
		t.Fatalf("WriteMetadataFile: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "terminal.json"))
	if err != nil {
		t.Fatalf("terminal.json not written: %v", err)
	}
	var out map[string]string
	if uerr := json.Unmarshal(b, &out); uerr != nil || out["kind"] != "Terminal" {
		t.Fatalf("terminal.json content = %q (err %v)", b, uerr)
	}
	assertOnlyFiles(t, dir, "terminal.json")

	pattern := metadataTempPattern("terminal.json")
	if pattern != ".terminal.json-*.tmp" {
		t.Fatalf("custom temp pattern = %q, want .terminal.json-*.tmp", pattern)
	}
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		t.Fatalf("CreateTemp(%q): %v", pattern, err)
	}
	_ = f.Close()
	if base := filepath.Base(f.Name()); !strings.HasPrefix(base, ".terminal.json-") ||
		!strings.HasSuffix(base, ".tmp") {
		t.Fatalf("temp file %q not named after terminal.json", base)
	}
}

func assertOnlyFiles(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("dir entries = %v, want %v", got, want)
	}
}
