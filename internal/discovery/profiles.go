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

// ProfileWarning re-exports [pkgdiscovery.ProfileWarning] for callers that
// only import the internal discovery package.
type ProfileWarning = pkgdiscovery.ProfileWarning

// ScanAndPrintProfiles loads every TerminalProfile under profilesDir and
// prints them to w. Any warnings produced by the loader are written to warnOut
// (typically os.Stderr) so the user can see why a file or document was
// skipped. format selects the table / json / yaml rendering.
func ScanAndPrintProfiles(
	ctx context.Context,
	logger *slog.Logger,
	profilesDir string,
	w io.Writer,
	warnOut io.Writer,
	format string,
) error {
	logger.DebugContext(ctx, "ScanAndPrintProfiles: loading profiles", "dir", profilesDir)
	profiles, warnings, err := LoadProfilesFromDir(ctx, logger, profilesDir)
	if err != nil {
		logger.ErrorContext(ctx, "ScanAndPrintProfiles: failed to load profiles", "dir", profilesDir, "error", err)
		return err
	}
	PrintProfileWarnings(warnOut, warnings)
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

// LoadProfilesFromDir is a thin wrapper around
// [pkgdiscovery.LoadProfilesFromDir].
func LoadProfilesFromDir(
	ctx context.Context,
	logger *slog.Logger,
	profilesDir string,
) ([]api.TerminalProfileDoc, []ProfileWarning, error) {
	return pkgdiscovery.LoadProfilesFromDir(ctx, logger, profilesDir)
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

// PrintProfileWarnings writes one human-readable line per warning to w.
// Each line is prefixed with "sbsh: warning:" so the diagnostic is easy to
// grep or filter out. A nil writer turns the call into a no-op.
func PrintProfileWarnings(w io.Writer, warnings []ProfileWarning) {
	if w == nil {
		return
	}
	for _, warn := range warnings {
		fmt.Fprintf(w, "sbsh: warning: %s\n", warn)
	}
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

// FindProfileByNameInDir is a thin wrapper around
// [pkgdiscovery.FindProfileByNameInDir].
func FindProfileByNameInDir(
	ctx context.Context,
	logger *slog.Logger,
	profilesDir, name string,
) (*api.TerminalProfileDoc, []ProfileWarning, error) {
	return pkgdiscovery.FindProfileByNameInDir(ctx, logger, profilesDir, name)
}

// FindAndPrintProfileMetadata looks up a profile by name under profilesDir
// and prints its metadata to w in the requested format. Loader warnings are
// printed to warnOut.
func FindAndPrintProfileMetadata(
	ctx context.Context,
	logger *slog.Logger,
	profilesDir string,
	w io.Writer,
	warnOut io.Writer,
	terminalName string,
	format string,
) error {
	logger.DebugContext(ctx, "FindAndPrintProfileMetadata: scanning profiles", "profilesDir", profilesDir)
	doc, warnings, err := FindProfileByNameInDir(ctx, logger, profilesDir, terminalName)
	PrintProfileWarnings(warnOut, warnings)
	if err != nil {
		logger.ErrorContext(ctx, "FindAndPrintProfileMetadata: failed to find profile", "error", err)
		return err
	}
	return printTerminalMetadata(w, doc, format)
}
