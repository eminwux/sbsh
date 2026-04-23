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

package validate

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/profile"
	pkgdiscovery "github.com/eminwux/sbsh/pkg/discovery"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewValidateProfilesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "profiles [path]",
		Aliases: []string{"profile", "prof", "pro", "p"},
		Short:   "Validate profile YAML",
		Long: `Validate one or more TerminalProfile YAML documents.

Each document is strictly decoded (unknown fields are rejected), required
fields are checked, runTarget and restartPolicy are validated against the
accepted enum values, and shell.cmd is checked against $PATH when
runTarget is "local".

The path argument is optional. When omitted, the profiles directory from
configuration (SB_PROFILES_DIR / --profiles-dir / config.yaml) is scanned
recursively and every *.yaml / *.yml file is validated. When a file is
passed explicitly, only that file is validated.

Along with per-document validation errors, this command surfaces
directory-level warnings such as malformed YAML files, documents missing
required fields, and duplicate profile names — so users whose shell
completion looks empty can run this to see exactly what the loader
rejected.`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidateProfiles(cmd, args, os.Stdout, os.Stderr)
		},
	}
	return cmd
}

// fileValidation groups the per-document validation results for a single
// YAML file, together with the (partial) parse error that aborted reading
// that file if any.
type fileValidation struct {
	Path       string
	Results    []profile.ProfileValidationResult
	DecodeErr  error
}

func runValidateProfiles(cmd *cobra.Command, args []string, stdout, stderr io.Writer) error {
	logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errdefs.ErrLoggerNotFound
	}

	path := resolveProfilesPath(args)
	logger.Debug("validate profiles invoked", "path", path)

	files, warnings, err := expandPath(cmd, logger, path)
	if err != nil {
		return err
	}

	validations, err := validateFiles(cmd, logger, files)
	if err != nil {
		return err
	}

	return reportResults(stdout, stderr, path, validations, warnings)
}

// resolveProfilesPath picks the path to validate: CLI arg wins, otherwise
// the configured profiles directory.
func resolveProfilesPath(args []string) string {
	if len(args) == 1 && args[0] != "" {
		return args[0]
	}
	return viper.GetString(config.SB_GET_PROFILES_DIR.ViperKey)
}

// expandPath resolves the target to concrete files to validate. When path is
// a directory, files are discovered via the same loader the runtime uses, so
// any duplicate-name / malformed-YAML warnings surface here identically to
// how the CLI would see them. When path is a file, the list is just that one
// file.
func expandPath(cmd *cobra.Command, logger *slog.Logger, path string) ([]string, []pkgdiscovery.ProfileWarning, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("stat %q: %w", path, err)
	}

	if !info.IsDir() {
		return []string{path}, nil, nil
	}

	_, warnings, err := pkgdiscovery.LoadProfilesFromDir(cmd.Context(), logger, path)
	if err != nil {
		return nil, warnings, err
	}

	var files []string
	walkErr := filepath.WalkDir(path, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext == ".yaml" || ext == ".yml" {
			files = append(files, p)
		}
		return nil
	})
	if walkErr != nil {
		return nil, warnings, fmt.Errorf("walk %q: %w", path, walkErr)
	}
	sort.Strings(files)
	return files, warnings, nil
}

func validateFiles(cmd *cobra.Command, logger *slog.Logger, files []string) ([]fileValidation, error) {
	var out []fileValidation
	for _, f := range files {
		results, err := profile.ValidateProfilesFromPath(cmd.Context(), logger, f)
		// Surface the error only when nothing could be validated, so one
		// corrupt file does not silence every other.
		if err != nil && len(results) == 0 {
			out = append(out, fileValidation{Path: f, DecodeErr: err})
			continue
		}
		out = append(out, fileValidation{Path: f, Results: results, DecodeErr: err})
	}
	return out, nil
}

func reportResults(
	stdout, stderr io.Writer,
	target string,
	validations []fileValidation,
	warnings []pkgdiscovery.ProfileWarning,
) error {
	fmt.Fprintf(stdout, "Validating %s\n", target)

	var (
		totalDocs      int
		invalidDocs    int
		hadDecodeError bool
	)
	for _, v := range validations {
		if v.DecodeErr != nil {
			hadDecodeError = true
		}
		if len(validations) > 1 {
			fmt.Fprintf(stdout, "\n%s\n", v.Path)
		}
		for _, r := range v.Results {
			totalDocs++
			label := formatLabel(r)
			if r.OK() {
				fmt.Fprintf(stdout, "  [OK]     %s\n", label)
				continue
			}
			invalidDocs++
			fmt.Fprintf(stdout, "  [INVALID] %s\n", label)
			for _, e := range r.Errors {
				fmt.Fprintf(stdout, "    - %s\n", e)
			}
		}
		if v.DecodeErr != nil {
			fmt.Fprintf(stderr, "warning: %s: %v\n", v.Path, v.DecodeErr)
		}
	}

	// Directory-level warnings (malformed files, duplicate names, unsupported
	// kind, missing required fields) go to stdout so `sb validate profiles`
	// is a self-contained diagnostic report for shell-completion users, then
	// also to stderr so interactive pipelines see them.
	if len(warnings) > 0 {
		fmt.Fprintln(stdout, "\nLoader warnings:")
		for _, w := range warnings {
			fmt.Fprintf(stdout, "  - %s\n", w)
		}
	}

	valid := totalDocs - invalidDocs
	fmt.Fprintf(stdout, "\n%d profile(s): %d valid, %d invalid; %d loader warning(s)\n",
		totalDocs, valid, invalidDocs, len(warnings))

	if invalidDocs > 0 || hadDecodeError || hasBlockingWarning(warnings) {
		return errdefs.ErrInvalidProfiles
	}
	return nil
}

// hasBlockingWarning returns true when at least one loader warning indicates
// that a document or file was actually rejected (as opposed to a harmless
// duplicate note). We treat every warning as blocking today: a duplicate
// profile name still means a profile got shadowed, which is almost certainly
// something the user wants to know before shipping.
func hasBlockingWarning(warnings []pkgdiscovery.ProfileWarning) bool {
	return len(warnings) > 0
}

func formatLabel(r profile.ProfileValidationResult) string {
	if r.ProfileName == "" {
		return fmt.Sprintf("document %d", r.DocIndex)
	}
	return fmt.Sprintf("document %d (%s)", r.DocIndex, r.ProfileName)
}

