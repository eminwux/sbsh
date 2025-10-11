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
func ScanAndPrintProfiles(ctx context.Context, path string, w io.Writer) error {
	profiles, err := LoadProfilesFromPath(ctx, path)
	if err != nil {
		return err
	}
	return PrintProfilesTable(w, profiles)
}

// LoadProfilesFromPath reads a multi-document YAML file into []api.SessionProfileDoc.
func LoadProfilesFromPath(_ context.Context, path string) ([]api.SessionProfileDoc, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open profiles file %q: %w", path, err)
	}
	defer f.Close()
	return LoadProfilesFromReader(f)
}

// LoadProfilesFromReader decodes one or more YAML documents from r.
func LoadProfilesFromReader(r io.Reader) ([]api.SessionProfileDoc, error) {
	dec := yaml.NewDecoder(r)

	var out []api.SessionProfileDoc
	for {
		var p api.SessionProfileDoc
		if err := dec.Decode(&p); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode profile: %w", err)
		}

		// Basic sanity checks; skip empty docs.
		if p.Metadata.Name == "" || string(p.APIVersion) == "" || string(p.Kind) == "" {
			slog.Debug("skipping empty/invalid profile document", "name", p.Metadata.Name)
			continue
		}
		out = append(out, p)
	}

	return out, nil
}

// PrintProfilesTable renders a compact table of profiles similar to printSessions().
func PrintProfilesTable(w io.Writer, profiles []api.SessionProfileDoc) error {
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
func FindProfileByName(ctx context.Context, path, name string) (*api.SessionProfileDoc, error) {
	profiles, err := LoadProfilesFromPath(ctx, path)
	if err != nil {
		return nil, err
	}

	for _, p := range profiles {
		if p.Metadata.Name == name {
			return &p, nil
		}
	}

	return nil, fmt.Errorf("profile %q not found in %s", name, path)
}
