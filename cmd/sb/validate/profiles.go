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
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/eminwux/sbsh/cmd/config"
	"github.com/eminwux/sbsh/cmd/types"
	"github.com/eminwux/sbsh/internal/errdefs"
	"github.com/eminwux/sbsh/internal/profile"
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

The path argument is optional; when omitted, the profiles file from
configuration (SB_PROFILES_FILE or --config) is validated.`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidateProfiles(cmd, args, os.Stdout, os.Stderr)
		},
	}
	return cmd
}

func runValidateProfiles(cmd *cobra.Command, args []string, stdout, stderr io.Writer) error {
	logger, ok := cmd.Context().Value(types.CtxLogger).(*slog.Logger)
	if !ok || logger == nil {
		return errdefs.ErrLoggerNotFound
	}

	path := resolveProfilesPath(args)
	logger.Debug("validate profiles invoked", "path", path)

	results, err := profile.ValidateProfilesFromPath(cmd.Context(), logger, path)
	if err != nil && len(results) == 0 {
		return err
	}

	return reportResults(cmd.Context(), stdout, stderr, path, results, err)
}

func resolveProfilesPath(args []string) string {
	if len(args) == 1 && args[0] != "" {
		return args[0]
	}
	return viper.GetString(config.SB_GET_PROFILES_FILE.ViperKey)
}

func reportResults(
	_ context.Context,
	stdout, stderr io.Writer,
	path string,
	results []profile.ProfileValidationResult,
	decodeErr error,
) error {
	fmt.Fprintf(stdout, "Validating %s\n", path)

	invalid := 0
	for _, r := range results {
		label := formatLabel(r)
		if r.OK() {
			fmt.Fprintf(stdout, "  [OK]     %s\n", label)
			continue
		}
		invalid++
		fmt.Fprintf(stdout, "  [INVALID] %s\n", label)
		for _, e := range r.Errors {
			fmt.Fprintf(stdout, "    - %s\n", e)
		}
	}

	total := len(results)
	valid := total - invalid
	fmt.Fprintf(stdout, "%d profile(s): %d valid, %d invalid\n", total, valid, invalid)

	if decodeErr != nil {
		fmt.Fprintf(stderr, "warning: stopped at unrecoverable parse error: %v\n", decodeErr)
	}

	if invalid > 0 || decodeErr != nil {
		return errdefs.ErrInvalidProfiles
	}
	return nil
}

func formatLabel(r profile.ProfileValidationResult) string {
	if r.ProfileName == "" {
		return fmt.Sprintf("document %d", r.DocIndex)
	}
	return fmt.Sprintf("document %d (%s)", r.DocIndex, r.ProfileName)
}
