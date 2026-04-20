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
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
	pkgdiscovery "github.com/eminwux/sbsh/pkg/discovery"
	"go.yaml.in/yaml/v3"
)

// ScanAndPrintTerminals finds all metadata.json under runPath/terminals/*,
// unmarshals them into api.TerminalSpec, and prints them to w.
// format: "" (compact table), "wide" (full table), "json", "yaml".
func ScanAndPrintTerminals(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	w io.Writer,
	printAll bool,
	format string,
) error {
	logger.DebugContext(ctx, "ScanAndPrintTerminals: scanning terminals", "runPath", runPath)
	terminals, err := ScanTerminals(ctx, logger, runPath)
	if err != nil {
		logger.ErrorContext(ctx, "ScanAndPrintTerminals: failed to scan terminals", "error", err)
		return err
	}
	ReconcileTerminals(ctx, logger, runPath, terminals)
	logger.InfoContext(ctx, "ScanAndPrintTerminals: scanned terminals", "count", len(terminals))

	switch format {
	case "json", "yaml":
		filtered := filterTerminals(terminals, printAll)
		return printTerminalMetadata(w, filtered, format)
	case "", "wide":
		return printTerminals(w, terminals, printAll, format == "wide")
	default:
		return fmt.Errorf("unknown output format: %q (use wide|json|yaml)", format)
	}
}

func filterTerminals(terminals []api.TerminalDoc, printAll bool) []api.TerminalDoc {
	if printAll {
		return terminals
	}
	out := make([]api.TerminalDoc, 0, len(terminals))
	for _, t := range terminals {
		if t.Status.State != api.Exited {
			out = append(out, t)
		}
	}
	return out
}

// ScanAndPruneTerminals finds all metadata.json under runPath/terminals/*,
// unmarshals them into api.TerminalSpec, and removes the terminal folders
// for terminals that are in Exited state.
func ScanAndPruneTerminals(ctx context.Context, logger *slog.Logger, runPath string, w io.Writer) error {
	terminals, err := ScanTerminals(ctx, logger, runPath)
	if err != nil {
		logger.ErrorContext(ctx, "ScanAndPruneTerminals: failed to scan terminals", "error", err)
		return err
	}
	_, err = scanAndPruneMetadata(
		ctx,
		logger,
		w,
		terminals,
		"ScanAndPruneTerminals",
		"terminal",
		func(t api.TerminalDoc) bool { return t.Status.State == api.Exited },
		PruneTerminal,
		pkgdiscovery.TerminalID,
		pkgdiscovery.TerminalName,
	)
	return err
}

func PruneTerminal(logger *slog.Logger, metadata *api.TerminalDoc) error {
	logger.DebugContext(
		context.Background(),
		"PruneTerminal: pruning terminal folder",
		"path",
		metadata.Status.BaseRunPath,
	)
	err := os.RemoveAll(metadata.Status.TerminalRunPath)
	if err != nil {
		logger.ErrorContext(
			context.Background(),
			"PruneTerminal: failed to remove terminal folder",
			"path",
			metadata.Status.TerminalRunPath,
			"error",
			err,
		)
	} else {
		logger.InfoContext(context.Background(), "PruneTerminal: terminal folder removed", "path", metadata.Status.TerminalRunPath)
	}
	return err
}

// ScanTerminals is a thin wrapper around [pkgdiscovery.ScanTerminals].
// Kept here so in-tree internal callers do not need to import pkg/discovery
// directly; new external consumers should use pkg/discovery.
func ScanTerminals(ctx context.Context, logger *slog.Logger, runPath string) ([]api.TerminalDoc, error) {
	return pkgdiscovery.ScanTerminals(ctx, logger, runPath)
}

func printTerminals(w io.Writer, terminals []api.TerminalDoc, printAll, wide bool) error {
	//nolint:mnd // tabwriter padding
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	activeCount := 0
	for _, s := range terminals {
		if s.Status.State != api.Exited {
			activeCount++
		}
	}

	if len(terminals) == 0 {
		fmt.Fprint(tw, NoTerminalsString)
		return tw.Flush()
	}

	if !printAll && activeCount == 0 {
		fmt.Fprintln(tw, "no active terminals found")
		return tw.Flush()
	}

	if wide {
		fmt.Fprintln(tw, "ID\tNAME\tPROFILE\tCMD\tTTY\tSTATUS\tATTACHERS\tCREATED\tATTACHED\tLABELS")
	} else {
		fmt.Fprintln(tw, "NAME\tPROFILE\tTTY\tCREATED")
	}
	for _, s := range terminals {
		if s.Status.State == api.Exited && !printAll {
			continue
		}
		if wide {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				pkgdiscovery.TerminalID(s),
				pkgdiscovery.TerminalName(s),
				terminalProfile(s),
				terminalCmd(s),
				terminalPty(s),
				s.Status.State.String(),
				terminalAttachers(s),
				formatAge(s.Metadata.CreatedAt),
				formatAge(s.Status.LastAttachedAt),
				joinLabels(terminalLabels(s)),
			)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				pkgdiscovery.TerminalName(s),
				terminalProfile(s),
				terminalPty(s),
				formatAge(s.Metadata.CreatedAt),
			)
		}
	}
	return tw.Flush()
}

// formatAge renders a timestamp as a short, human-readable duration since now
// ("5s", "3m", "2h", "4d"). Zero timestamps render as "-".
func formatAge(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := max(time.Since(t), 0)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))

	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		//nolint:mnd // 24h = day
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func terminalAttachers(s api.TerminalDoc) string {
	attachers := "None"
	if len(s.Status.Attachers) > 0 {
		attachers = strings.Join(s.Status.Attachers, ",")
	}
	return attachers
}

func terminalCmd(s api.TerminalDoc) string {
	parts := make([]string, 0, 1+len(s.Spec.CommandArgs))
	if s.Spec.Command != "" {
		parts = append(parts, s.Spec.Command)
	}
	parts = append(parts, s.Spec.CommandArgs...)
	return strings.Join(parts, " ")
}

func terminalPty(s api.TerminalDoc) string {
	return s.Status.Tty
}

func terminalProfile(s api.TerminalDoc) string {
	return s.Spec.ProfileName
}

func terminalLabels(s api.TerminalDoc) map[string]string {
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

// FindTerminalByID is a thin wrapper around [pkgdiscovery.FindTerminalByID].
func FindTerminalByID(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	id string,
) (*api.TerminalDoc, error) {
	return pkgdiscovery.FindTerminalByID(ctx, logger, runPath, id)
}

// FindTerminalByName is a thin wrapper around [pkgdiscovery.FindTerminalByName].
func FindTerminalByName(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	name string,
) (*api.TerminalDoc, error) {
	return pkgdiscovery.FindTerminalByName(ctx, logger, runPath, name)
}

func PrintTerminalSpec(s *api.TerminalSpec, logger *slog.Logger) error {
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

// FindAndPrintTerminalMetadata finds all metadata.json under runPath/terminals/*,
// unmarshals them into api.TerminalSpec, and prints a table to w.
func FindAndPrintTerminalMetadata(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	w io.Writer,
	terminalName string,
	format string,
) error {
	logger.DebugContext(ctx, "FindAndPrintTerminalMetadata: scanning terminals", "runPath", runPath)
	terminals, err := FindTerminalByName(ctx, logger, runPath, terminalName)
	if err != nil {
		logger.ErrorContext(ctx, "FindAndPrintTerminalMetadata: failed to scan terminals", "error", err)
		return err
	}
	if terminals != nil {
		reconcileTerminal(ctx, logger, runPath, terminals)
	}
	return printTerminalMetadata(w, terminals, format)
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
		// Print all fields of TerminalMetadata in a human-readable form.
		// %+v includes struct field names and their values.
		fmt.Fprintf(w, "%+v\n", t)
		PrintHuman(w, t, "")

		return nil
	default:
		return fmt.Errorf("unknown output format: %q (use json|yaml)", format)
	}
}
