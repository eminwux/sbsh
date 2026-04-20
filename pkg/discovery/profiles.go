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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/eminwux/sbsh/pkg/api"
	"gopkg.in/yaml.v3"
)

// LoadProfilesFromPath reads a multi-document YAML file into []api.TerminalProfileDoc.
// Documents missing APIVersion, Kind, or Metadata.Name are logged and skipped.
func LoadProfilesFromPath(
	ctx context.Context,
	logger *slog.Logger,
	profilesFile string,
) ([]api.TerminalProfileDoc, error) {
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

// LoadProfilesFromReaderWithContext decodes one or more YAML documents from r
// into []api.TerminalProfileDoc. Documents missing APIVersion, Kind, or
// Metadata.Name are logged and skipped.
func LoadProfilesFromReaderWithContext(
	ctx context.Context,
	logger *slog.Logger,
	r io.Reader,
) ([]api.TerminalProfileDoc, error) {
	logger.DebugContext(ctx, "LoadProfilesFromReader: decoding YAML documents")
	dec := yaml.NewDecoder(r)

	var out []api.TerminalProfileDoc
	docCount := 0
	for {
		var p api.TerminalProfileDoc
		if err := dec.Decode(&p); err != nil {
			if errors.Is(err, io.EOF) {
				logger.InfoContext(ctx, "LoadProfilesFromReader: reached EOF", "count", docCount)
				break
			}
			logger.ErrorContext(ctx, "LoadProfilesFromReader: decode error", "error", err)
			return nil, fmt.Errorf("decode profile: %w", err)
		}
		docCount++

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

// FindProfileByName scans the YAML file at path and returns the profile
// whose Metadata.Name matches. The match is case-sensitive. Returns an
// error if no profile with that name is found.
func FindProfileByName(ctx context.Context, logger *slog.Logger, path, name string) (*api.TerminalProfileDoc, error) {
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
