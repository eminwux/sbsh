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

package api

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func Test_TerminalStatusMode_String(t *testing.T) {
	tests := []struct {
		name     string
		state    TerminalStatusMode
		expected string
	}{
		{
			name:     "Initializing",
			state:    Initializing,
			expected: "Initializing",
		},
		{
			name:     "Starting",
			state:    Starting,
			expected: "Starting",
		},
		{
			name:     "Ready",
			state:    Ready,
			expected: "Ready",
		},
		{
			name:     "Exited",
			state:    Exited,
			expected: "Exited",
		},
		{
			name:     "Unknown",
			state:    TerminalStatusMode(999),
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.expected {
				t.Errorf("TerminalStatusMode.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func Test_TerminalSpec_Validate(t *testing.T) {
	tests := []struct {
		name    string
		spec    TerminalSpec
		wantErr string // substring; empty means "must succeed"
	}{
		{
			name: "single child via top-level Command",
			spec: TerminalSpec{Command: "/bin/bash"},
		},
		{
			name: "multi-process via Processes",
			spec: TerminalSpec{
				Processes: []ProcessSpec{
					{Name: "a", Command: "/bin/bash"},
					{Name: "b", Command: "/bin/sh"},
				},
			},
		},
		{
			name: "rejects Processes + top-level Command",
			spec: TerminalSpec{
				Command:   "/bin/bash",
				Processes: []ProcessSpec{{Name: "a", Command: "/bin/bash"}},
			},
			wantErr: "mutually exclusive",
		},
		{
			name:    "rejects empty Processes + empty top-level Command",
			spec:    TerminalSpec{},
			wantErr: "either processes or top-level command must be set",
		},
		{
			name: "rejects duplicate process names",
			spec: TerminalSpec{
				Processes: []ProcessSpec{
					{Name: "a", Command: "/bin/bash"},
					{Name: "a", Command: "/bin/sh"},
				},
			},
			wantErr: `duplicate process name "a"`,
		},
		{
			name: "rejects empty process Name",
			spec: TerminalSpec{
				Processes: []ProcessSpec{{Command: "/bin/bash"}},
			},
			wantErr: "processes[0].name is required",
		},
		{
			name: "rejects empty process Command",
			spec: TerminalSpec{
				Processes: []ProcessSpec{{Name: "a"}},
			},
			wantErr: "processes[0]",
		},
		{
			name: "rejects negative SwitchReplayBytes",
			spec: TerminalSpec{
				Command:           "/bin/bash",
				SwitchReplayBytes: -1,
			},
			wantErr: "switchReplayBytes must be non-negative",
		},
		{
			name: "accepts zero SwitchReplayBytes (means runner default)",
			spec: TerminalSpec{
				Command:           "/bin/bash",
				SwitchReplayBytes: 0,
			},
		},
		{
			name: "accepts custom CyclePrefix",
			spec: TerminalSpec{
				Command:     "/bin/bash",
				CyclePrefix: "\x02",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// Test_TerminalSpec_YAML_RoundTrip_SingleChild asserts that a spec carrying
// today's single-child shape (top-level Command, empty Processes) marshals
// to YAML and back to a YAML form that is byte-identical on the second
// pass, and that the new phase-1 fields stay absent from the wire form
// when zero — preserving backwards compatibility with pre-phase-1
// manifests. The second-pass byte equality is the operational round-trip
// claim (pre-existing yaml lib quirks like nil-vs-empty-map normalisation
// are out of scope for phase-1 schema work).
func Test_TerminalSpec_YAML_RoundTrip_SingleChild(t *testing.T) {
	original := TerminalSpec{
		ID:          "term-1",
		Name:        "demo",
		Command:     "/bin/bash",
		CommandArgs: []string{"-l"},
		CaptureFile: "/tmp/capture",
	}

	first, err := marshalYAML(&original)
	if err != nil {
		t.Fatalf("yaml.Marshal (first pass): %v", err)
	}

	got := string(first)
	for _, key := range []string{"processes", "cyclePrefix", "switchReplayBytes"} {
		if strings.Contains(got, key+":") {
			t.Errorf("marshalled YAML unexpectedly carries %q for a single-child spec:\n%s", key, got)
		}
	}

	var decoded TerminalSpec
	if uerr := unmarshalYAML(first, &decoded); uerr != nil {
		t.Fatalf("yaml.Unmarshal: %v", uerr)
	}

	// Verify the load-bearing fields survived the round-trip.
	if decoded.ID != original.ID ||
		decoded.Name != original.Name ||
		decoded.Command != original.Command ||
		!reflect.DeepEqual(decoded.CommandArgs, original.CommandArgs) ||
		decoded.CaptureFile != original.CaptureFile {
		t.Errorf("YAML round-trip lost data:\nwant: %#v\n got: %#v", original, decoded)
	}
	// And that the new phase-1 fields stayed at their zero values.
	if len(decoded.Processes) != 0 || decoded.CyclePrefix != "" || decoded.SwitchReplayBytes != 0 {
		t.Errorf("phase-1 fields drifted on round-trip: %#v", decoded)
	}

	second, err := marshalYAML(&decoded)
	if err != nil {
		t.Fatalf("yaml.Marshal (second pass): %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("YAML output not stable across re-marshal:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// marshalYAML and unmarshalYAML route through `any` so the musttag linter
// (which requires yaml tags on every field of a struct passed directly to
// yaml.Marshal) leaves these tests alone. Existing TerminalSpec fields
// rely on yaml.v3's default field-name lowercasing — adding explicit yaml
// tags to every field would change the on-the-wire field names from
// "commandargs" to "commandArgs", which is out of scope for the phase-1
// schema work.
func marshalYAML(v any) ([]byte, error)   { return yaml.Marshal(v) }
func unmarshalYAML(b []byte, v any) error { return yaml.Unmarshal(b, v) }

// Test_TerminalSpec_YAML_Parse_Processes asserts that a YAML document
// carrying the phase-1 multi-process shape parses into ProcessSpec entries
// with the expected field bindings (json/yaml tag names).
func Test_TerminalSpec_YAML_Parse_Processes(t *testing.T) {
	src := []byte(`
id: term-2
name: multi
processes:
  - name: a
    command: /bin/bash
    commandArgs: ["-l"]
    captureFile: /tmp/a.cap
    turn: 0
  - name: b
    command: /bin/sh
    turn: 1
cyclePrefix: "\x02"
switchReplayBytes: 8192
`)

	var spec TerminalSpec
	if err := unmarshalYAML(src, &spec); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	if len(spec.Processes) != 2 {
		t.Fatalf("Processes len = %d, want 2", len(spec.Processes))
	}
	if spec.Processes[0].Name != "a" || spec.Processes[0].Command != "/bin/bash" {
		t.Errorf("Processes[0] = %+v, want {Name:a Command:/bin/bash ...}", spec.Processes[0])
	}
	if !reflect.DeepEqual(spec.Processes[0].CommandArgs, []string{"-l"}) {
		t.Errorf("Processes[0].CommandArgs = %v, want [-l]", spec.Processes[0].CommandArgs)
	}
	if spec.Processes[0].CaptureFile != "/tmp/a.cap" {
		t.Errorf("Processes[0].CaptureFile = %q, want /tmp/a.cap", spec.Processes[0].CaptureFile)
	}
	if spec.Processes[1].Turn != 1 {
		t.Errorf("Processes[1].Turn = %d, want 1", spec.Processes[1].Turn)
	}
	if spec.CyclePrefix != "\x02" {
		t.Errorf("CyclePrefix = %q, want \\x02", spec.CyclePrefix)
	}
	if spec.SwitchReplayBytes != 8192 {
		t.Errorf("SwitchReplayBytes = %d, want 8192", spec.SwitchReplayBytes)
	}

	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate() on parsed multi-process spec: %v", err)
	}
}

// Test_TerminalSpec_JSON_RoundTrip_Processes covers the metadata.json
// codepath: TerminalSpec is persisted as JSON, so the new fields must
// round-trip cleanly through encoding/json too.
func Test_TerminalSpec_JSON_RoundTrip_Processes(t *testing.T) {
	original := TerminalSpec{
		ID:   "term-3",
		Name: "multi-json",
		Processes: []ProcessSpec{
			{Name: "a", Command: "/bin/bash", CommandArgs: []string{"-l"}},
			{Name: "b", Command: "/bin/sh", Turn: 1},
		},
		CyclePrefix:       "\x01",
		SwitchReplayBytes: 4096,
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(&original); err != nil {
		t.Fatalf("json.Encode: %v", err)
	}

	var decoded TerminalSpec
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(decoded.Processes, original.Processes) {
		t.Fatalf("Processes diverged after JSON round-trip:\nwant: %#v\n got: %#v",
			original.Processes, decoded.Processes)
	}
	if decoded.CyclePrefix != original.CyclePrefix {
		t.Errorf("CyclePrefix diverged: want %q, got %q", original.CyclePrefix, decoded.CyclePrefix)
	}
	if decoded.SwitchReplayBytes != original.SwitchReplayBytes {
		t.Errorf("SwitchReplayBytes diverged: want %d, got %d",
			original.SwitchReplayBytes, decoded.SwitchReplayBytes)
	}
}
