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
	"fmt"
	"io"
	"log/slog"
	"strings"
	"text/tabwriter"

	"github.com/eminwux/sbsh/pkg/api"
	pkgdiscovery "github.com/eminwux/sbsh/pkg/discovery"
)

// ScanAndPrintProfiles loads all profiles from a YAML file (supports multiple '---' documents)
// and prints them to w.
// format: "" (compact table), "wide" (full table), "json", "yaml".
func ScanAndPrintProfiles(
	ctx context.Context,
	logger *slog.Logger,
	path string,
	w io.Writer,
	format string,
) error {
	logger.DebugContext(ctx, "ScanAndPrintProfiles: loading profiles", "path", path)
	profiles, err := LoadProfilesFromPath(ctx, logger, path)
	if err != nil {
		logger.ErrorContext(ctx, "ScanAndPrintProfiles: failed to load profiles", "path", path, "error", err)
		return err
	}
	logger.InfoContext(ctx, "ScanAndPrintProfiles: loaded profiles", "count", len(profiles))

	switch format {
	case "json", "yaml":
		return printTerminalMetadata(w, profiles, format)
	case "", "wide":
		return PrintProfilesTable(w, profiles, format == "wide")
	default:
		return fmt.Errorf("unknown output format: %q (use wide|json|yaml)", format)
	}
}

// LoadProfilesFromPath is a thin wrapper around [pkgdiscovery.LoadProfilesFromPath].
func LoadProfilesFromPath(
	ctx context.Context,
	logger *slog.Logger,
	profilesFile string,
) ([]api.TerminalProfileDoc, error) {
	return pkgdiscovery.LoadProfilesFromPath(ctx, logger, profilesFile)
}

// LoadProfilesFromReaderWithContext is a thin wrapper around
// [pkgdiscovery.LoadProfilesFromReaderWithContext].
func LoadProfilesFromReaderWithContext(
	ctx context.Context,
	logger *slog.Logger,
	r io.Reader,
) ([]api.TerminalProfileDoc, error) {
	return pkgdiscovery.LoadProfilesFromReaderWithContext(ctx, logger, r)
}

func PrintProfilesTable(w io.Writer, profiles []api.TerminalProfileDoc, wide bool) error {
	//nolint:mnd // tabwriter padding
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	if len(profiles) == 0 {
		fmt.Fprintln(tw, "no profiles found")
		return tw.Flush()
	}

	if wide {
		fmt.Fprintln(tw, "NAME\tTARGET\tENVVARS\tCMD")
	} else {
		fmt.Fprintln(tw, "NAME\tCMD")
	}
	for _, p := range profiles {
		args := strings.Join(p.Spec.Shell.CmdArgs, " ")
		var cmd string
		if args != "" {
			cmd = fmt.Sprintf("%s %s", p.Spec.Shell.Cmd, args)
		} else {
			cmd = p.Spec.Shell.Cmd
		}
		if wide {
			envCount := 0
			if p.Spec.Shell.Env != nil {
				envCount = len(p.Spec.Shell.Env)
			}
			fmt.Fprintf(tw, "%s\t%s\t%d vars\t%s\n",
				p.Metadata.Name,
				p.Spec.RunTarget,
				envCount,
				cmd,
			)
		} else {
			fmt.Fprintf(tw, "%s\t%s\n", p.Metadata.Name, cmd)
		}
	}

	return tw.Flush()
}

// FindProfileByName is a thin wrapper around [pkgdiscovery.FindProfileByName].
func FindProfileByName(ctx context.Context, logger *slog.Logger, path, name string) (*api.TerminalProfileDoc, error) {
	return pkgdiscovery.FindProfileByName(ctx, logger, path, name)
}

// FindAndPrintProfileMetadata finds all metadata.json under profiles file,
// unmarshals them into api.ProfileSpec, and prints a table to w.
func FindAndPrintProfileMetadata(
	ctx context.Context,
	logger *slog.Logger,
	profilesFile string,
	w io.Writer,
	terminalName string,
	format string,
) error {
	logger.DebugContext(ctx, "FindAndPrintProfileMetadata: scanning profiles", "profilesFile", profilesFile)
	profiles, err := FindProfileByName(ctx, logger, profilesFile, terminalName)
	if err != nil {
		logger.ErrorContext(ctx, "FindAndPrintProfileMetadata: failed to scan profiles", "error", err)
		return err
	}
	return printTerminalMetadata(w, profiles, format)
}
