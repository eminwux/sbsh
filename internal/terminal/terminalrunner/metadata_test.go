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

package terminalrunner

import (
	"path/filepath"
	"testing"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/pkg/api"
)

// resolveMetadataDir is the runner-side switch for option A (issue #273).
// An empty override falls through to the legacy
// runPath/terminals/<id>/ layout that pkg/discovery.ScanTerminals scans;
// the second return is false so CreateMetadata MkdirAlls the path.
func TestResolveMetadataDir_NoOverrideUsesLegacyLayout(t *testing.T) {
	runPath := "/run/sbsh"
	id := api.ID("term-id")

	dir, embedderOwns := resolveMetadataDir(runPath, "", id)

	want := filepath.Join(runPath, defaults.TerminalsRunPath, string(id))
	if dir != want {
		t.Fatalf("dir: want %q, got %q", want, dir)
	}
	if embedderOwns {
		t.Fatal("embedderOwns: want false on legacy path so CreateMetadata MkdirAlls")
	}
}

// resolveMetadataDir: a non-empty override is used verbatim, and the
// second return is true so CreateMetadata skips MkdirAll — the embedder
// pre-allocated this directory (e.g. via a bind mount) and owns its
// contents.
func TestResolveMetadataDir_OverrideUsedVerbatim(t *testing.T) {
	override := "/run/kukeon/tty"

	dir, embedderOwns := resolveMetadataDir("/run/sbsh", override, api.ID("term-id"))

	if dir != override {
		t.Fatalf("dir: want %q, got %q", override, dir)
	}
	if !embedderOwns {
		t.Fatal("embedderOwns: want true so CreateMetadata skips MkdirAll")
	}
}
