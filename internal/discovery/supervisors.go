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
	"os"
	"sort"
	"text/tabwriter"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/pkg/api"
)

// ScanAndPrintSupervisors finds all metadata.json under runPath/supervisors/*,
// unmarshals them into api.SupervisorSpec, and prints a table to w.
func ScanAndPrintSupervisors(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	w io.Writer,
	printAll bool,
) error {
	logger.DebugContext(ctx, "ScanAndPrintSupervisors: scanning supervisors", "runPath", runPath)
	supervisors, err := ScanSupervisors(ctx, logger, runPath)
	if err != nil {
		logger.ErrorContext(ctx, "ScanAndPrintSupervisors: failed to scan supervisors", "error", err)
		return err
	}
	logger.InfoContext(ctx, "ScanAndPrintSupervisors: scanned supervisors", "count", len(supervisors))
	return printSupervisors(w, supervisors, printAll)
}

// ScanAndPruneSupervisors finds all metadata.json under runPath/supervisors/*,
// unmarshals them into api.SupervisorSpec, and removes the supervisor folders
// for supervisors that are in Exited state.
func ScanAndPruneSupervisors(ctx context.Context, logger *slog.Logger, runPath string, w io.Writer) error {
	supervisors, err := ScanSupervisors(ctx, logger, runPath)
	if err != nil {
		logger.ErrorContext(ctx, "ScanAndPruneSupervisors: failed to scan supervisors", "error", err)
		return err
	}
	_, err = scanAndPruneMetadata(
		ctx,
		logger,
		w,
		supervisors,
		"ScanAndPruneSupervisors",
		"supervisor",
		func(s api.SupervisorDoc) bool { return s.Status.State == api.SupervisorExited },
		PruneSupervisor,
		supervisorID,
		supervisorName,
	)
	return err
}

func ScanSupervisors(ctx context.Context, logger *slog.Logger, runPath string) ([]api.SupervisorDoc, error) {
	out, err := scanMetadataFiles(
		ctx,
		logger,
		runPath,
		defaults.SupervisorsRunPath,
		"ScanSupervisors",
		supervisorID,
		supervisorName,
	)
	if err != nil {
		return nil, err
	}

	// Optional: stable order by ID (fallback to Name if ID empty)
	sort.Slice(out, func(i, j int) bool {
		idi, idj := supervisorID(out[i]), supervisorID(out[j])
		if idi != idj {
			return idi < idj
		}
		return supervisorName(out[i]) < supervisorName(out[j])
	})

	return out, nil
}

func PruneSupervisor(logger *slog.Logger, metadata *api.SupervisorDoc) error {
	logger.DebugContext(
		context.Background(),
		"PruneSupervisor: pruning supervisor folder",
		"path",
		metadata.Status.BaseRunPath,
	)
	err := os.RemoveAll(metadata.Status.SupervisorRunPath)
	if err != nil {
		logger.ErrorContext(
			context.Background(),
			"PruneSupervisor: failed to remove supervisor folder",
			"path",
			metadata.Status.SupervisorRunPath,
			"error",
			err,
		)
	} else {
		logger.InfoContext(context.Background(), "PruneSupervisor: supervisor folder removed", "path", metadata.Status.SupervisorRunPath)
	}
	return err
}

func supervisorID(s api.SupervisorDoc) string {
	// If your type uses Id instead of ID, change to: return s.Id
	return string(s.Spec.ID)
}

func supervisorName(s api.SupervisorDoc) string {
	return s.Spec.Name
}

func supervisorLabels(s api.SupervisorDoc) map[string]string {
	if len(s.Spec.Labels) != 0 {
		return s.Spec.Labels
	}
	return map[string]string{}
}

func printSupervisors(w io.Writer, supervisors []api.SupervisorDoc, printAll bool) error {
	//nolint:mnd // tabwriter padding
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	activeCount := 0
	for _, s := range supervisors {
		if s.Status.State != api.SupervisorExited {
			activeCount++
		}
	}

	if len(supervisors) == 0 {
		fmt.Fprint(tw, NoSupervisorsString)
		return tw.Flush()
	}

	if !printAll && activeCount == 0 {
		fmt.Fprintln(tw, "no active supervisors found")
		return tw.Flush()
	}

	fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tLABELS")
	for _, s := range supervisors {
		if printAll || s.Status.State != api.SupervisorExited {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				supervisorID(s),
				supervisorName(s),
				s.Status.State.String(),
				joinLabels(supervisorLabels(s)),
			)
		}
	}
	return tw.Flush()
}

// FindSupervisorByName scans runPath/supervisors/*/metadata.json and returns
// the supervisor whose Spec.Name matches the given name. If not found, returns nil.
func FindSupervisorByName(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	name string,
) (*api.SupervisorDoc, error) {
	supervisors, err := ScanSupervisors(ctx, logger, runPath)
	if err != nil {
		return nil, err
	}
	return findMetadataBy(
		supervisors,
		func(s api.SupervisorDoc) bool { return supervisorName(s) == name },
		fmt.Sprintf("supervisor with name %q not found", name),
	)
}

// FindAndPrintSupervisorMetadata finds all metadata.json under runPath/supervisors/*,
// unmarshals them into api.SupervisorSpec, and prints a table to w.
func FindAndPrintSupervisorMetadata(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	w io.Writer,
	terminalName string,
	format string,
) error {
	logger.DebugContext(ctx, "FindAndPrintSupervisorMetadata: scanning supervisors", "runPath", runPath)
	supervisors, err := FindSupervisorByName(ctx, logger, runPath, terminalName)
	if err != nil {
		logger.ErrorContext(ctx, "FindAndPrintSupervisorMetadata: failed to scan supervisors", "error", err)
		return err
	}
	return printTerminalMetadata(w, supervisors, format)
}
