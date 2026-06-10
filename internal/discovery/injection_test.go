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

package discovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/pkg/api"
	"go.yaml.in/yaml/v3"
)

// evilName carries an OSC title-set, a colour CSI, and a clear-screen CSI — the
// classic terminal-output-injection payloads an attacker can smuggle through a
// terminal/profile/client name (#390).
const evilName = "evil\x1b]0;pwned\x07\x1b[31m\x1b[2J"

// assertNoRawEscapes fails if out carries any byte a terminal would interpret as
// a control/escape command, and confirms the row was actually rendered (the
// visible "evil" prefix survives, escaped ESC bytes are present) rather than
// silently dropped.
func assertNoRawEscapes(t *testing.T, label, out string) {
	t.Helper()
	for i := 0; i < len(out); i++ {
		if b := out[i]; b < 0x20 && b != '\n' && b != '\t' || b == 0x7f {
			t.Fatalf("%s: raw control byte %#x emitted to terminal: %q", label, b, out)
		}
	}
	if !strings.Contains(out, "evil") {
		t.Fatalf("%s: expected the sanitized row to still render the visible name, got %q", label, out)
	}
	if !strings.Contains(out, `\x1b`) {
		t.Fatalf("%s: expected ESC bytes to be escaped as \\x1b, got %q", label, out)
	}
}

func TestScanAndPrintTerminals_SanitizesName(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()
	runPath := t.TempDir()
	writeTerminalDoc(t, runPath, "id-1", evilName, api.Ready, os.Getpid())

	for _, format := range []string{"", "wide"} {
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintTerminals(ctx, logger, runPath, &buf, false, format); err != nil {
			t.Fatalf("format %q: unexpected error: %v", format, err)
		}
		assertNoRawEscapes(t, "terminals "+format, buf.String())
	}
}

func TestScanAndPrintClients_SanitizesName(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()
	runPath := t.TempDir()
	writeNamedClientDoc(t, runPath, "id-1", evilName, api.ClientReady, os.Getpid())

	for _, format := range []string{"", "wide"} {
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintClients(ctx, logger, runPath, &buf, false, format); err != nil {
			t.Fatalf("format %q: unexpected error: %v", format, err)
		}
		assertNoRawEscapes(t, "clients "+format, buf.String())
	}
}

func TestScanAndPrintProfiles_SanitizesName(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()
	dir := t.TempDir()
	// The double-quoted YAML scalar decodes / to the raw control
	// bytes, mirroring a metadata.name an attacker authored on disk.
	evilProfile := "apiVersion: sbsh/v1beta1\n" +
		"kind: TerminalProfile\n" +
		"metadata:\n" +
		"  name: \"evil\\u001b]0;pwned\\u0007\\u001b[31m\\u001b[2J\"\n" +
		"spec:\n" +
		"  runTarget: local\n" +
		"  shell:\n" +
		"    cmd: /bin/bash\n"
	writeProfileFile(t, dir, "evil.yaml", evilProfile)

	for _, format := range []string{"", "wide"} {
		var buf, warn bytes.Buffer
		if err := discovery.ScanAndPrintProfiles(ctx, logger, dir, &buf, &warn, format); err != nil {
			t.Fatalf("format %q: unexpected error: %v", format, err)
		}
		assertNoRawEscapes(t, "profiles "+format, buf.String())
	}
}

// TestMachineReadableOutputKeepsRawName covers the AC counterpart: -o json and
// -o yaml are not terminal-rendered, so they must carry the exact original name
// byte-for-byte after a decode round-trip.
func TestMachineReadableOutputKeepsRawName(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()
	runPath := t.TempDir()
	writeTerminalDoc(t, runPath, "id-1", evilName, api.Ready, os.Getpid())

	t.Run("json", func(t *testing.T) {
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintTerminals(ctx, logger, runPath, &buf, true, "json"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var docs []api.TerminalDoc
		if err := json.Unmarshal(buf.Bytes(), &docs); err != nil {
			t.Fatalf("decode json: %v", err)
		}
		if len(docs) != 1 || docs[0].Spec.Name != evilName {
			t.Fatalf("json did not preserve the exact name; got %+v", docs)
		}
	})

	t.Run("yaml", func(t *testing.T) {
		var buf bytes.Buffer
		if err := discovery.ScanAndPrintTerminals(ctx, logger, runPath, &buf, true, "yaml"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var docs []api.TerminalDoc
		if err := yaml.Unmarshal(buf.Bytes(), &docs); err != nil {
			t.Fatalf("decode yaml: %v", err)
		}
		if len(docs) != 1 || docs[0].Spec.Name != evilName {
			t.Fatalf("yaml did not preserve the exact name; got %+v", docs)
		}
	})
}
