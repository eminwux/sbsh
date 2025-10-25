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
	"go.yaml.in/yaml/v3"
)

// ScanAndPrintTerminals finds all metadata.json under runPath/sessions/*,
// unmarshals them into api.SessionSpec, and prints a table to w.
func ScanAndPrintTerminals(ctx context.Context, logger *slog.Logger, runPath string, w io.Writer, printAll bool) error {
	logger.DebugContext(ctx, "ScanAndPrintTerminals: scanning terminals", "runPath", runPath)
	sessions, err := ScanTerminals(ctx, logger, runPath)
	if err != nil {
		logger.ErrorContext(ctx, "ScanAndPrintTerminals: failed to scan terminals", "error", err)
		return err
	}
	logger.InfoContext(ctx, "ScanAndPrintTerminals: scanned terminals", "count", len(sessions))
	return printTerminals(w, sessions, printAll)
}

// ScanAndPruneTerminals finds all metadata.json under runPath/sessions/*,
// unmarshals them into api.SessionSpec, and removes the session folders
// for terminals that are in Exited state.
func ScanAndPruneTerminals(ctx context.Context, logger *slog.Logger, runPath string, w io.Writer) error {
	logger.DebugContext(ctx, "ScanAndPruneTerminals: scanning terminals", "runPath", runPath)
	terminals, err := ScanTerminals(ctx, logger, runPath)
	if err != nil {
		logger.ErrorContext(ctx, "ScanAndPruneTerminals: failed to scan terminals", "error", err)
		return err
	}
	pruned := 0
	for _, s := range terminals {
		if s.Status.State == api.Exited {
			logger.InfoContext(ctx, "ScanAndPruneTerminals: pruning terminal", "id", terminalID(s))
			if errC := PruneTerminal(logger, &s); errC != nil {
				logger.ErrorContext(
					ctx,
					"ScanAndPruneTerminals: failed to prune terminal",
					"id",
					terminalID(s),
					"error",
					errC,
				)
				return fmt.Errorf("prune terminal %s: %w", terminalID(s), errC)
			}
			pruned++
			if w != nil {
				fmt.Fprintf(w, "Pruned terminal %s (%s)\n", terminalID(s), terminalName(s))
			}
		}
	}
	logger.InfoContext(ctx, "ScanAndPruneTerminals: prune complete", "pruned", pruned)
	return nil
}

func PruneTerminal(logger *slog.Logger, metadata *api.SessionMetadata) error {
	logger.DebugContext(
		context.Background(),
		"PruneSession: pruning session folder",
		"path",
		metadata.Status.BaseRunPath,
	)
	err := os.RemoveAll(metadata.Status.SessionRunPath)
	if err != nil {
		logger.ErrorContext(
			context.Background(),
			"PruneSession: failed to remove session folder",
			"path",
			metadata.Status.SessionRunPath,
			"error",
			err,
		)
	} else {
		logger.InfoContext(context.Background(), "PruneSession: session folder removed", "path", metadata.Status.SessionRunPath)
	}
	return err
}

func ScanTerminals(ctx context.Context, logger *slog.Logger, runPath string) ([]api.SessionMetadata, error) {
	pattern := filepath.Join(runPath, TerminalsRunPath, "*", "metadata.json")
	logger.DebugContext(ctx, "ScanTerminals: globbing for terminal metadata", "pattern", pattern)
	paths, err := filepath.Glob(pattern)
	if err != nil {
		logger.ErrorContext(ctx, "ScanTerminals: glob failed", "error", err)
		return nil, fmt.Errorf("glob %q: %w", pattern, err)
	}

	out := make([]api.SessionMetadata, 0, len(paths))
	for _, p := range paths {
		select {
		case <-ctx.Done():
			logger.WarnContext(ctx, "ScanTerminals: context done while reading terminals")
			return nil, ctx.Err()
		default:
		}
		b, errRead := os.ReadFile(p)
		if errRead != nil {
			logger.ErrorContext(ctx, "ScanTerminals: failed to read file", "file", p, "error", errRead)
			return nil, fmt.Errorf("read %s: %w", p, errRead)
		}
		var s api.SessionMetadata
		if errUnmarshal := json.Unmarshal(b, &s); errUnmarshal != nil {
			logger.ErrorContext(ctx, "ScanTerminals: failed to decode file", "file", p, "error", errUnmarshal)
			return nil, fmt.Errorf("decode %s: %w", p, errUnmarshal)
		}
		logger.DebugContext(
			ctx,
			"ScanTerminals: loaded terminal metadata",
			"id",
			terminalID(s),
			"name",
			terminalName(s),
		)
		out = append(out, s)
	}

	// Optional: stable order by ID (fallback to Name if ID empty)
	sort.Slice(out, func(i, j int) bool {
		idi, idj := terminalID(out[i]), terminalID(out[j])
		if idi != idj {
			return idi < idj
		}
		return terminalName(out[i]) < terminalName(out[j])
	})

	logger.InfoContext(ctx, "ScanTerminals: finished scanning", "count", len(out))
	return out, nil
}

func printTerminals(w io.Writer, terminals []api.SessionMetadata, printAll bool) error {
	//nolint:mnd // tabwriter padding
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	activeCount := 0
	for _, s := range terminals {
		if s.Status.State != api.Exited {
			activeCount++
		}
	}

	if len(terminals) == 0 {
		fmt.Fprintln(tw, "no active or inactive terminals found")
		return tw.Flush()
	}

	if printAll {
		if len(terminals) == 0 {
			fmt.Fprintln(tw, "no active or inactive terminals found")
			return tw.Flush()
		}
	} else {
		if activeCount == 0 {
			fmt.Fprintln(tw, "no active terminals found")
			return tw.Flush()
		}
	}

	fmt.Fprintln(tw, "ID\tNAME\tPROFILE\tCMD\tTTY\tSTATUS\tATTACHERS\tLABELS")
	for _, s := range terminals {
		if s.Status.State != api.Exited || (printAll && s.Status.State == api.Exited) {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				terminalID(s),
				terminalName(s),
				terminalProfile(s),
				terminalCmd(s),
				terminalPty(s),
				s.Status.State.String(),
				terminalAttachers(s),
				joinLabels(sessionLabels(s)),
			)
		}
	}
	return tw.Flush()
}

// --- helpers (adjust to your api.SessionSpec fields if needed) ---

func terminalAttachers(s api.SessionMetadata) string {
	attachers := "None"
	if len(s.Status.Attachers) > 0 {
		attachers = strings.Join(s.Status.Attachers, ",")
	}
	return attachers
}

func terminalID(s api.SessionMetadata) string {
	// If your type uses Id instead of ID, change to: return s.Id
	return string(s.Spec.ID)
}

func terminalName(s api.SessionMetadata) string {
	return s.Spec.Name
}

func terminalCmd(s api.SessionMetadata) string {
	parts := make([]string, 0, 1+len(s.Spec.CommandArgs))
	if s.Spec.Command != "" {
		parts = append(parts, s.Spec.Command)
	}
	parts = append(parts, s.Spec.CommandArgs...)
	return strings.Join(parts, " ")
}

func terminalPty(s api.SessionMetadata) string {
	return s.Status.Tty
}

func terminalProfile(s api.SessionMetadata) string {
	return s.Spec.ProfileName
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

// FindTerminalByID scans runPath/sessions/*/metadata.json and returns
// the session whose Spec.ID matches the given id. If not found, returns nil.
func FindTerminalByID(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	id string,
) (*api.SessionMetadata, error) {
	sessions, err := ScanTerminals(ctx, logger, runPath)
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

// FindTerminalByName scans runPath/sessions/*/metadata.json and returns
// the session whose Spec.Name matches the given name. If not found, returns nil.
func FindTerminalByName(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	name string,
) (*api.SessionMetadata, error) {
	sessions, err := ScanTerminals(ctx, logger, runPath)
	if err != nil {
		return nil, err
	}
	for _, s := range sessions {
		if terminalName(s) == name {
			ss := s // copy to avoid referencing loop variable
			return &ss, nil
		}
	}
	return nil, fmt.Errorf("session with name %q not found", name)
}

func PrintTerminalSpec(s *api.SessionSpec, logger *slog.Logger) error {
	if s == nil {
		logger.Info("nil terminal spec")
		return nil
	}

	logger.Info("TerminalSpec",
		"ID", s.ID,
		"NAME", s.Name,
		"KIND", s.Kind,
		"COMMAND", fmt.Sprintf("%s %s", s.Command, strings.Join(s.CommandArgs, " ")),
		"PROMPT", s.Prompt,
	)

	if s.RunPath != "" {
		logger.Info("RunPath", "value", s.RunPath)
	}

	if s.CaptureFile != "" {
		logger.Info("CaptureFile", "value", s.CaptureFile)
	}

	if s.LogFile != "" {
		logger.Info("LogFile", "value", s.LogFile)
	}

	if s.LogLevel != "" {
		logger.Info("LogLevel", "value", s.LogLevel)
	}

	if s.SocketFile != "" {
		logger.Info("SocketCtrl", "value", s.SocketFile)
	}

	if len(s.Env) > 0 {
		logger.Info("Environment", "vars", s.Env)
	}

	if len(s.Labels) > 0 {
		keys := make([]string, 0, len(s.Labels))
		for k := range s.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		labels := make([]string, 0, len(keys))
		for _, k := range keys {
			labels = append(labels, fmt.Sprintf("%s=%s", k, s.Labels[k]))
		}
		logger.Info("Labels", "labels", labels)
	}

	return nil
}

// FindAndPrintTerminalMetadata finds all metadata.json under runPath/sessions/*,
// unmarshals them into api.SessionSpec, and prints a table to w.
func FindAndPrintTerminalMetadata(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	w io.Writer,
	terminalName string,
	format string,
) error {
	logger.DebugContext(ctx, "FindAndPrintTerminalMetadata: scanning terminals", "runPath", runPath)
	sessions, err := FindTerminalByName(ctx, logger, runPath, terminalName)
	if err != nil {
		logger.ErrorContext(ctx, "FindAndPrintTerminalMetadata: failed to scan terminals", "error", err)
		return err
	}
	return printTerminalMetadata(w, sessions, format)
}

func printTerminalMetadata(w io.Writer, t any, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(t)
	case "yaml":
		b, err := yaml.Marshal(t)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	case "":
		// Print all fields of SessionMetadata in a human-readable form.
		// %+v includes struct field names and their values.
		fmt.Fprintf(w, "%+v\n", t)
		PrintHuman(w, t, "")

		return nil
	default:
		return fmt.Errorf("unknown output format: %q (use json|yaml)", format)
	}
}
