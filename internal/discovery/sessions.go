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

package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/eminwux/sbsh/pkg/api"
)

// ScanAndPrintSessions finds all metadata.json under runPath/sessions/*,
// unmarshals them into api.SessionSpec, and prints a table to w.
func ScanAndPrintSessions(ctx context.Context, runPath string, w io.Writer, printAll bool) error {
	sessions, err := ScanSessions(ctx, runPath)
	if err != nil {
		return err
	}
	return printSessions(w, sessions, printAll)
}

// ScanAndPruneSessions finds all metadata.json under runPath/sessions/*,
// unmarshals them into api.SessionSpec, and removes the session folders
// for sessions that are in Exited state.
func ScanAndPruneSessions(ctx context.Context, runPath string, w io.Writer) error {
	sessions, err := ScanSessions(ctx, runPath)
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if s.Status.State == api.SessionStatusExited {
			if err := PruneSession(&s); err != nil {
				return fmt.Errorf("prune session %s: %w", sessionID(s), err)
			}
			if w != nil {
				fmt.Fprintf(w, "Pruned session %s (%s)\n", sessionID(s), sessionName(s))
			}
		}
	}
	return nil
}

func PruneSession(metadata *api.SessionMetadata) error {
	slog.Debug("pruning session folder", "path", metadata.Status.BaseRunPath)
	return os.RemoveAll(metadata.Status.SessionRunPath)
}

func ScanSessions(ctx context.Context, runPath string) ([]api.SessionMetadata, error) {
	pattern := filepath.Join(runPath, "sessions", "*", "metadata.json")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob %q: %w", pattern, err)
	}

	out := make([]api.SessionMetadata, 0, len(paths))
	for _, p := range paths {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		var s api.SessionMetadata
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, fmt.Errorf("decode %s: %w", p, err)
		}
		out = append(out, s)
	}

	// Optional: stable order by ID (fallback to Name if ID empty)
	sort.Slice(out, func(i, j int) bool {
		idi, idj := sessionID(out[i]), sessionID(out[j])
		if idi != idj {
			return idi < idj
		}
		return sessionName(out[i]) < sessionName(out[j])
	})

	return out, nil
}

func printSessions(w io.Writer, sessions []api.SessionMetadata, printAll bool) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	activeCount := 0
	for _, s := range sessions {
		if s.Status.State != api.SessionStatusExited {
			activeCount++
		}
	}

	if len(sessions) == 0 {
		fmt.Fprintln(tw, "no active or inactive sessions found")
		return tw.Flush()
	}

	if printAll {
		if len(sessions) == 0 {
			fmt.Fprintln(tw, "no active or inactive sessions found")
			return tw.Flush()
		}
	} else {
		if activeCount == 0 {
			fmt.Fprintln(tw, "no active sessions found")
			return tw.Flush()
		}
	}

	fmt.Fprintln(tw, "ID\tNAME\tCMD\tSTATUS\tLABELS")
	for _, s := range sessions {
		if s.Status.State != api.SessionStatusExited || (printAll && s.Status.State == api.SessionStatusExited) {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				sessionID(s),
				sessionName(s),
				sessionCmd(s),
				s.Status.State.String(),
				joinLabels(sessionLabels(s)),
			)
		}
	}
	return tw.Flush()
}

// --- helpers (adjust to your api.SessionSpec fields if needed) ---

func sessionID(s api.SessionMetadata) string {
	// If your type uses Id instead of ID, change to: return s.Id
	return string(s.Spec.ID)
}

func sessionName(s api.SessionMetadata) string {
	return s.Spec.Name
}

func sessionCmd(s api.SessionMetadata) string {
	parts := make([]string, 0, 1+len(s.Spec.CommandArgs))
	if s.Spec.Command != "" {
		parts = append(parts, s.Spec.Command)
	}
	parts = append(parts, s.Spec.CommandArgs...)
	return strings.Join(parts, " ")
}

func sessionLabels(s api.SessionMetadata) map[string]string {
	if len(s.Spec.Labels) != 0 {
		return s.Spec.Labels
	}
	return map[string]string{}
}

func joinLabels(m map[string]string) string {
	if len(m) == 0 {
		return "none"
	}
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	parts := make([]string, 0, len(ks))
	for _, k := range ks {
		parts = append(parts, fmt.Sprintf("%s=%s", k, m[k]))
	}
	return strings.Join(parts, ",")
}

// FindSessionByID scans runPath/sessions/*/metadata.json and returns
// the session whose Spec.ID matches the given id. If not found, returns nil.
func FindSessionByID(ctx context.Context, runPath string, id string) (*api.SessionMetadata, error) {
	sessions, err := ScanSessions(ctx, runPath)
	if err != nil {
		return nil, err
	}
	for _, s := range sessions {
		if string(s.Spec.ID) == id {
			// return a copy to avoid referencing the loop variable
			ss := s
			return &ss, nil
		}
	}
	return nil, fmt.Errorf("session %q not found", id)
}

// FindSessionByName scans runPath/sessions/*/metadata.json and returns
// the session whose Spec.Name matches the given name. If not found, returns nil.
func FindSessionByName(ctx context.Context, runPath string, name string) (*api.SessionMetadata, error) {
	sessions, err := ScanSessions(ctx, runPath)
	if err != nil {
		return nil, err
	}
	for _, s := range sessions {
		if sessionName(s) == name {
			ss := s // copy to avoid referencing loop variable
			return &ss, nil
		}
	}
	return nil, fmt.Errorf("session with name %q not found", name)
}

func PrintSessionSpec(s *api.SessionSpec, w io.Writer) error {
	if s == nil {
		fmt.Fprintln(w, "nil session spec")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fmt.Fprintf(tw, "ID:\t%s\n", s.ID)
	fmt.Fprintf(tw, "NAME:\t%s\n", s.Name)
	fmt.Fprintf(tw, "KIND:\t%s\n", s.Kind)
	fmt.Fprintf(tw, "COMMAND:\t%s %s\n", s.Command, strings.Join(s.CommandArgs, " "))
	fmt.Fprintf(tw, "PROMPT:\t%s\n", s.Prompt)

	if s.RunPath != "" {
		fmt.Fprintf(tw, "RUN PATH:\t%s\n", s.RunPath)
	}

	if s.LogFilename != "" {
		fmt.Fprintf(tw, "LOG FILE:\t%s\n", s.LogFilename)
	}

	if s.SocketFile != "" {
		fmt.Fprintf(tw, "SOCKET CTRL:\t%s\n", s.SocketFile)
	}

	if len(s.Env) > 0 {
		fmt.Fprintln(tw, "ENVIRONMENT:")
		for _, e := range s.Env {
			fmt.Fprintf(tw, "\t%s\n", e)
		}
	}

	if len(s.Labels) > 0 {
		fmt.Fprintln(tw, "LABELS:")
		keys := make([]string, 0, len(s.Labels))
		for k := range s.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(tw, "\t%s=%s\n", k, s.Labels[k])
		}
	}

	return tw.Flush()
}
