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

package profile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"

	"github.com/eminwux/sbsh/pkg/api"
	"gopkg.in/yaml.v3"
)

// ProfileValidationResult holds the outcome of validating a single YAML document.
type ProfileValidationResult struct {
	DocIndex    int     // 1-based index of the document within the file
	ProfileName string  // metadata.name, empty if decoding failed
	Errors      []error // all issues found for this document; nil when valid
}

// OK reports whether the profile document passed all checks.
func (r ProfileValidationResult) OK() bool { return len(r.Errors) == 0 }

// ValidateProfilesFromPath validates every profile document in the YAML file at
// profilesFile. See ValidateProfilesFromReader for the semantics.
func ValidateProfilesFromPath(
	ctx context.Context,
	logger *slog.Logger,
	profilesFile string,
) ([]ProfileValidationResult, error) {
	logger.DebugContext(ctx, "ValidateProfilesFromPath: opening file", "path", profilesFile)
	f, err := os.Open(profilesFile)
	if err != nil {
		return nil, fmt.Errorf("open profiles file %q: %w", profilesFile, err)
	}
	defer f.Close()
	return ValidateProfilesFromReader(ctx, logger, f)
}

// ValidateProfilesFromReader decodes each YAML document in r strictly (unknown
// fields are rejected) and runs per-document validation: required apiVersion /
// kind / metadata.name / shell.cmd, enum checks on runTarget and restartPolicy,
// and a PATH-resolution check on shell.cmd when runTarget is "local" (or unset).
//
// It returns one ProfileValidationResult per document. Decode errors on a doc
// produce a result with empty ProfileName and the decode error attached; when
// the underlying YAML stream becomes unparsable the function stops and returns
// the partial results together with a non-nil error.
func ValidateProfilesFromReader(
	ctx context.Context,
	logger *slog.Logger,
	r io.Reader,
) ([]ProfileValidationResult, error) {
	dec := yaml.NewDecoder(r)

	var results []ProfileValidationResult
	docIndex := 0
	for {
		var node yaml.Node
		if err := dec.Decode(&node); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			docIndex++
			logger.ErrorContext(ctx, "ValidateProfilesFromReader: unrecoverable parse error", "doc", docIndex, "error", err)
			results = append(results, ProfileValidationResult{
				DocIndex: docIndex,
				Errors:   []error{err},
			})
			return results, fmt.Errorf("parse profile document %d: %w", docIndex, err)
		}
		docIndex++

		results = append(results, validateOneDocument(ctx, logger, docIndex, &node))
	}

	logger.InfoContext(ctx, "ValidateProfilesFromReader: finished", "count", len(results))
	return results, nil
}

// validateOneDocument runs the strict-decode pass (to surface unknown fields)
// alongside a tolerant decode (to exercise enum + PATH checks even when strict
// decoding fails). Both error sets are aggregated on the returned result.
func validateOneDocument(ctx context.Context, logger *slog.Logger, docIndex int, node *yaml.Node) ProfileValidationResult {
	res := ProfileValidationResult{DocIndex: docIndex}

	var tolerant api.TerminalProfileDoc
	if err := node.Decode(&tolerant); err != nil {
		logger.DebugContext(ctx, "ValidateProfilesFromReader: tolerant decode failed", "doc", docIndex, "error", err)
		res.Errors = append(res.Errors, err)
	}
	res.ProfileName = tolerant.Metadata.Name

	if strictErr := decodeStrict(node); strictErr != nil {
		res.Errors = append(res.Errors, strictErr)
	}

	res.Errors = append(res.Errors, validateProfileDoc(&tolerant)...)
	return res
}

func decodeStrict(node *yaml.Node) error {
	buf, err := yaml.Marshal(node)
	if err != nil {
		return fmt.Errorf("re-marshal for strict decode: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(buf))
	dec.KnownFields(true)
	var strict api.TerminalProfileDoc
	if err := dec.Decode(&strict); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func validateProfileDoc(p *api.TerminalProfileDoc) []error {
	var errs []error

	if p.APIVersion == "" {
		errs = append(errs, errors.New("apiVersion is required"))
	} else if p.APIVersion != api.APIVersionV1Beta1 {
		errs = append(errs, fmt.Errorf("apiVersion %q not supported (expected %q)", p.APIVersion, api.APIVersionV1Beta1))
	}

	if p.Kind == "" {
		errs = append(errs, errors.New("kind is required"))
	} else if p.Kind != api.KindTerminalProfile {
		errs = append(errs, fmt.Errorf("kind %q not supported (expected %q)", p.Kind, api.KindTerminalProfile))
	}

	if p.Metadata.Name == "" {
		errs = append(errs, errors.New("metadata.name is required"))
	}

	if p.Spec.Shell.Cmd == "" {
		errs = append(errs, errors.New("spec.shell.cmd is required"))
	}

	if err := validateRunTarget(p.Spec.RunTarget); err != nil {
		errs = append(errs, err)
	}
	if err := validateRestartPolicy(p.Spec.RestartPolicy); err != nil {
		errs = append(errs, err)
	}

	// Only exercise the PATH check when runTarget targets the local host.
	if p.Spec.Shell.Cmd != "" && isLocalRunTarget(p.Spec.RunTarget) {
		if err := validateCmdInPath(p.Spec.Shell.Cmd); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

func isLocalRunTarget(rt api.RunTarget) bool {
	return rt == api.RunTargetLocal || rt == ""
}

func validateRunTarget(rt api.RunTarget) error {
	switch rt {
	case api.RunTargetLocal, "":
		return nil
	}
	return fmt.Errorf("spec.runTarget %q not one of [%q]", rt, api.RunTargetLocal)
}

func validateRestartPolicy(rp api.RestartPolicy) error {
	switch rp {
	case api.RestartExit, api.RestartUnlimited, api.RestartOnError, "":
		return nil
	}
	return fmt.Errorf("spec.restartPolicy %q not one of [%q, %q, %q]",
		rp, api.RestartExit, api.RestartUnlimited, api.RestartOnError)
}

func validateCmdInPath(cmd string) error {
	if _, err := exec.LookPath(cmd); err != nil {
		return fmt.Errorf("spec.shell.cmd %q: %w", cmd, err)
	}
	return nil
}
