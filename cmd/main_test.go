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

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestDispatch(t *testing.T) {
	tests := []struct {
		name   string
		exe    string
		debug  string
		wantOK bool
	}{
		{name: "exe sb", exe: "sb", debug: "", wantOK: true},
		{name: "exe sbsh", exe: "sbsh", debug: "", wantOK: true},
		{name: "exe -sbsh", exe: "-sbsh", debug: "", wantOK: true},
		{name: "debug fallback sb", exe: "weird", debug: "sb", wantOK: true},
		{name: "debug fallback sbsh", exe: "weird", debug: "sbsh", wantOK: true},
		{name: "unknown exe empty debug", exe: "weird", debug: "", wantOK: false},
		{name: "unknown exe unknown debug", exe: "weird", debug: "nope", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, ok := dispatch(tt.exe, tt.debug)
			if ok != tt.wantOK {
				t.Fatalf("dispatch(%q, %q) ok = %v, want %v", tt.exe, tt.debug, ok, tt.wantOK)
			}
			if tt.wantOK && factory == nil {
				t.Fatal("expected non-nil factory when ok is true")
			}
			if !tt.wantOK && factory != nil {
				t.Fatal("expected nil factory when ok is false")
			}
		})
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	var stderr bytes.Buffer
	got := run([]string{"/usr/bin/bogus"}, func(string) string { return "" }, &stderr)
	if got != 1 {
		t.Fatalf("run() = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "unknown entry command: bogus") {
		t.Fatalf("stderr = %q, want it to mention the unknown command", stderr.String())
	}
}

func TestExecRoot(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cmd := &cobra.Command{
			Use:           "x",
			SilenceUsage:  true,
			SilenceErrors: true,
			RunE:          func(*cobra.Command, []string) error { return nil },
		}
		cmd.SetArgs([]string{})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		if got := execRoot(cmd); got != 0 {
			t.Fatalf("execRoot() = %d, want 0", got)
		}
	})

	t.Run("error", func(t *testing.T) {
		cmd := &cobra.Command{
			Use:           "x",
			SilenceUsage:  true,
			SilenceErrors: true,
			RunE:          func(*cobra.Command, []string) error { return errors.New("boom") },
		}
		cmd.SetArgs([]string{})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		if got := execRoot(cmd); got != 1 {
			t.Fatalf("execRoot() = %d, want 1", got)
		}
	})
}

func TestRunWithFactory(t *testing.T) {
	t.Run("factory error", func(t *testing.T) {
		factory := func() (*cobra.Command, error) { return nil, errors.New("build failed") }
		if got := runWithFactory(context.Background(), factory); got != 1 {
			t.Fatalf("runWithFactory() = %d, want 1", got)
		}
	})

	t.Run("factory success", func(t *testing.T) {
		factory := func() (*cobra.Command, error) {
			cmd := &cobra.Command{
				Use:           "x",
				SilenceUsage:  true,
				SilenceErrors: true,
				RunE:          func(*cobra.Command, []string) error { return nil },
			}
			cmd.SetArgs([]string{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			return cmd, nil
		}
		if got := runWithFactory(context.Background(), factory); got != 0 {
			t.Fatalf("runWithFactory() = %d, want 0", got)
		}
	})
}
