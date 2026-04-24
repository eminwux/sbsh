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
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eminwux/sbsh/pkg/api"
	"gopkg.in/yaml.v3"
)

// ProfileWarning describes a non-fatal problem encountered while loading
// profiles from a directory. Warnings never cause the loader to abort: they
// are surfaced alongside whatever profiles did load successfully so callers
// (interactive CLI, shell completion, `sb validate profiles`) can decide how
// to present them.
type ProfileWarning struct {
	// File is the absolute path of the file the warning applies to.
	File string
	// DocIndex is the 1-based index of the offending YAML document within
	// File, or 0 when the warning is file-level (open error, malformed stream).
	DocIndex int
	// Reason is a human-readable description of the problem.
	Reason string
}

// String formats the warning as a single-line diagnostic suitable for stderr.
func (w ProfileWarning) String() string {
	if w.DocIndex > 0 {
		return fmt.Sprintf("%s (doc %d): %s", w.File, w.DocIndex, w.Reason)
	}
	return fmt.Sprintf("%s: %s", w.File, w.Reason)
}

// LoadProfilesFromDir walks dir recursively and loads every *.yaml / *.yml
// file into []api.TerminalProfileDoc. Files are processed in deterministic
// lexicographic order by absolute path, which makes first-wins behaviour for
// duplicate profile names reproducible.
//
// A missing or empty dir is not an error: the function returns
// (nil, nil, nil) so callers can start with zero profiles. An empty dir
// string is also treated as "nothing to load".
//
// Any problem that is scoped to a single file or document becomes a
// ProfileWarning rather than aborting the scan:
//   - cannot open a file,
//   - malformed YAML,
//   - missing required apiVersion/kind/metadata.name,
//   - unsupported apiVersion or kind,
//   - duplicate profile name (keeps the first and skips subsequent).
//
// An error is returned only for unrecoverable conditions such as a walk
// failure on the directory itself.
func LoadProfilesFromDir(
	ctx context.Context,
	logger *slog.Logger,
	dir string,
) ([]api.TerminalProfileDoc, []ProfileWarning, error) {
	if dir == "" {
		logger.DebugContext(ctx, "LoadProfilesFromDir: empty dir, nothing to load")
		return nil, nil, nil
	}

	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.DebugContext(ctx, "LoadProfilesFromDir: directory not found", "dir", dir)
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("stat profiles dir %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("profiles path %q is not a directory", dir)
	}

	files, err := collectYAMLFiles(dir)
	if err != nil {
		return nil, nil, err
	}

	var (
		profiles []api.TerminalProfileDoc
		warnings []ProfileWarning
		// seen maps profile name -> file that first loaded that name.
		seen = make(map[string]string)
	)

	for _, file := range files {
		docs, fileWarnings := loadProfilesFromFile(ctx, logger, file)
		warnings = append(warnings, fileWarnings...)
		for _, doc := range docs {
			if owner, dup := seen[doc.Metadata.Name]; dup {
				warnings = append(warnings, ProfileWarning{
					File: file,
					Reason: fmt.Sprintf(
						"duplicate profile name %q already loaded from %s; this copy skipped",
						doc.Metadata.Name, owner,
					),
				})
				continue
			}
			seen[doc.Metadata.Name] = file
			profiles = append(profiles, doc)
		}
	}

	logger.InfoContext(ctx, "LoadProfilesFromDir: finished",
		"dir", dir,
		"profiles", len(profiles),
		"warnings", len(warnings),
	)
	return profiles, warnings, nil
}

// collectYAMLFiles returns the lexicographically sorted list of *.yaml and
// *.yml files found under dir (recursively).
func collectYAMLFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".yaml" || ext == ".yml" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk profiles dir %q: %w", dir, err)
	}
	sort.Strings(files)
	return files, nil
}

// loadProfilesFromFile decodes every YAML document in a single file and
// returns the ones that pass minimal schema checks plus any warnings
// collected along the way.
func loadProfilesFromFile(
	ctx context.Context,
	logger *slog.Logger,
	file string,
) ([]api.TerminalProfileDoc, []ProfileWarning) {
	f, err := os.Open(file)
	if err != nil {
		return nil, []ProfileWarning{{
			File:   file,
			Reason: fmt.Sprintf("open: %v", err),
		}}
	}
	defer f.Close()

	logger.DebugContext(ctx, "loadProfilesFromFile: decoding", "path", file)
	return decodeProfileStream(ctx, logger, file, f)
}

func decodeProfileStream(
	ctx context.Context,
	logger *slog.Logger,
	file string,
	r io.Reader,
) ([]api.TerminalProfileDoc, []ProfileWarning) {
	dec := yaml.NewDecoder(r)

	var (
		profiles []api.TerminalProfileDoc
		warnings []ProfileWarning
	)
	docIndex := 0
	for {
		var p api.TerminalProfileDoc
		err := dec.Decode(&p)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			docIndex++
			warnings = append(warnings, ProfileWarning{
				File:     file,
				DocIndex: docIndex,
				Reason:   fmt.Sprintf("malformed YAML: %v", err),
			})
			// A malformed stream can throw off subsequent document
			// boundaries, so stop at the first parse failure in this file.
			break
		}
		docIndex++

		// Skip empty YAML documents (separators with no content).
		if p.APIVersion == "" && p.Kind == "" && p.Metadata.Name == "" {
			logger.DebugContext(ctx, "decodeProfileStream: skipping empty document", "file", file, "doc", docIndex)
			continue
		}

		if warn, ok := validateRequiredFields(file, docIndex, &p); !ok {
			warnings = append(warnings, warn)
			continue
		}

		profiles = append(profiles, p)
	}

	return profiles, warnings
}

func validateRequiredFields(file string, docIndex int, p *api.TerminalProfileDoc) (ProfileWarning, bool) {
	switch {
	case p.APIVersion == "":
		return ProfileWarning{File: file, DocIndex: docIndex, Reason: "missing required apiVersion"}, false
	case p.APIVersion != api.APIVersionV1Beta1:
		return ProfileWarning{
			File: file, DocIndex: docIndex,
			Reason: fmt.Sprintf("unsupported apiVersion %q (expected %q)", p.APIVersion, api.APIVersionV1Beta1),
		}, false
	case p.Kind == "":
		return ProfileWarning{File: file, DocIndex: docIndex, Reason: "missing required kind"}, false
	case p.Kind != api.KindTerminalProfile:
		return ProfileWarning{
			File: file, DocIndex: docIndex,
			Reason: fmt.Sprintf("unsupported kind %q (expected %q)", p.Kind, api.KindTerminalProfile),
		}, false
	case p.Metadata.Name == "":
		return ProfileWarning{File: file, DocIndex: docIndex, Reason: "missing required metadata.name"}, false
	}
	return ProfileWarning{}, true
}

// FindProfileByNameInDir returns the profile whose Metadata.Name matches
// name, along with every warning produced while scanning dir. The match is
// case-sensitive; when no profile matches, the returned error names the
// target.
func FindProfileByNameInDir(
	ctx context.Context,
	logger *slog.Logger,
	dir, name string,
) (*api.TerminalProfileDoc, []ProfileWarning, error) {
	profiles, warnings, err := LoadProfilesFromDir(ctx, logger, dir)
	if err != nil {
		return nil, warnings, err
	}
	for i := range profiles {
		if profiles[i].Metadata.Name == name {
			return &profiles[i], warnings, nil
		}
	}
	return nil, warnings, fmt.Errorf("profile %q not found in %s", name, dir)
}

// LoadProfilesFromReaderWithContext decodes one or more YAML documents from r
// into []api.TerminalProfileDoc. It exists primarily for tests and callers
// that already have a stream in hand. Documents missing required fields are
// logged and skipped; malformed YAML aborts the scan and returns an error.
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
				"doc", docCount,
				"name", p.Metadata.Name,
			)
			continue
		}
		logger.DebugContext(ctx, "LoadProfilesFromReader: loaded profile", "name", p.Metadata.Name)
		out = append(out, p)
	}

	logger.InfoContext(ctx, "LoadProfilesFromReader: finished loading profiles", "count", len(out))
	return out, nil
}
