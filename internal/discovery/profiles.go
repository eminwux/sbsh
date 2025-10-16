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

// Package profiles provides helpers to load and list SessionProfile YAMLs.
package discovery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/eminwux/sbsh/pkg/api"
	"gopkg.in/yaml.v3"
)

// ScanAndPrintProfiles loads all profiles from a YAML file (supports multiple '---' documents)
// and prints them in a table to w.
func ScanAndPrintProfiles(ctx context.Context, logger *slog.Logger, path string, w io.Writer) error {
	logger.DebugContext(ctx, "ScanAndPrintProfiles: loading profiles", "path", path)
	profiles, err := LoadProfilesFromPath(ctx, logger, path)
	if err != nil {
		logger.ErrorContext(ctx, "ScanAndPrintProfiles: failed to load profiles", "path", path, "error", err)
		return err
	}
	logger.InfoContext(ctx, "ScanAndPrintProfiles: loaded profiles", "count", len(profiles))
	return PrintProfilesTable(w, profiles)
}

// LoadProfilesFromPath reads a multi-document YAML file into []api.SessionProfileDoc.
func LoadProfilesFromPath(
	ctx context.Context,
	logger *slog.Logger,
	profilesFile string,
) ([]api.SessionProfileDoc, error) {
	logger.DebugContext(ctx, "LoadProfilesFromPath: opening file", "path", profilesFile)
	f, err := os.Open(profilesFile)
	if err != nil {
		logger.ErrorContext(ctx, "LoadProfilesFromPath: failed to open file", "path", profilesFile, "error", err)
		return nil, fmt.Errorf("open profiles file %q: %w", profilesFile, err)
	}
	defer f.Close()
	logger.InfoContext(ctx, "LoadProfilesFromPath: file opened", "path", profilesFile)
	return LoadProfilesFromReaderWithContext(ctx, logger, f)
}

// LoadProfilesFromReader decodes one or more YAML documents from r.
func LoadProfilesFromReaderWithContext(
	ctx context.Context,
	logger *slog.Logger,
	r io.Reader,
) ([]api.SessionProfileDoc, error) {
	logger.DebugContext(ctx, "LoadProfilesFromReader: decoding YAML documents")
	dec := yaml.NewDecoder(r)

	var out []api.SessionProfileDoc
	docCount := 0
	for {
		var p api.SessionProfileDoc
		if err := dec.Decode(&p); err != nil {
			if errors.Is(err, io.EOF) {
				logger.InfoContext(ctx, "LoadProfilesFromReader: reached EOF", "count", docCount)
				break
			}
			logger.ErrorContext(ctx, "LoadProfilesFromReader: decode error", "error", err)
			return nil, fmt.Errorf("decode profile: %w", err)
		}
		docCount++

		// Basic sanity checks; skip empty docs.
		if p.Metadata.Name == "" || string(p.APIVersion) == "" || string(p.Kind) == "" {
			logger.WarnContext(ctx,
				"LoadProfilesFromReader: skipping empty/invalid profile document",
				"doc",
				docCount,
				"name",
				p.Metadata.Name,
			)
			continue
		}
		logger.DebugContext(ctx, "LoadProfilesFromReader: loaded profile", "name", p.Metadata.Name)
		out = append(out, p)
	}

	logger.InfoContext(ctx, "LoadProfilesFromReader: finished loading profiles", "count", len(out))
	return out, nil
}

// PrintProfilesTable renders a compact table of profiles similar to printSessions().
func PrintProfilesTable(w io.Writer, profiles []api.SessionProfileDoc) error {
	//nolint:mnd // tabwriter padding
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	if len(profiles) == 0 {
		fmt.Fprintln(tw, "no profiles found")
		return tw.Flush()
	}

	fmt.Fprintln(tw, "NAME\tTARGET\tRESTART\tENVVARS\tCMD")
	for _, p := range profiles {
		args := strings.Join(p.Spec.Shell.CmdArgs, " ")
		var cmd string
		if args != "" {
			cmd = fmt.Sprintf("%s %s", p.Spec.Shell.Cmd, args)
		} else {
			cmd = p.Spec.Shell.Cmd
		}
		envCount := 0
		if p.Spec.Shell.Env != nil {
			envCount = len(p.Spec.Shell.Env)
		}
		fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%d vars\t%s\n",
			p.Metadata.Name,
			p.Spec.RunTarget,
			p.Spec.RestartPolicy,
			envCount,
			cmd,
		)
	}

	return tw.Flush()
}

// FindProfileByName scans the YAML file at path and returns the profile whose metadata.name matches.
// The match is case-sensitive; use strings.EqualFold if you prefer case-insensitive lookup.
func FindProfileByName(ctx context.Context, logger *slog.Logger, path, name string) (*api.SessionProfileDoc, error) {
	logger.DebugContext(ctx, "FindProfileByName: searching for profile", "name", name, "path", path)
	profiles, err := LoadProfilesFromPath(ctx, logger, path)
	if err != nil {
		logger.ErrorContext(ctx, "FindProfileByName: failed to load profiles", "error", err)
		return nil, err
	}

	for _, p := range profiles {
		if p.Metadata.Name == name {
			logger.InfoContext(ctx, "FindProfileByName: found profile", "name", name)
			return &p, nil
		}
	}

	logger.WarnContext(ctx, "FindProfileByName: profile not found", "name", name, "path", path)
	return nil, fmt.Errorf("profile %q not found in %s", name, path)
}
