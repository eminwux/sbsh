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
	"log/slog"
	"sort"

	"github.com/eminwux/sbsh/internal/defaults"
	"github.com/eminwux/sbsh/pkg/api"
)

// ScanTerminals enumerates every terminal metadata.json under
// runPath/terminals/*. Results are sorted by Spec.ID (then Spec.Name
// for ties) so consecutive scans produce stable output.
func ScanTerminals(ctx context.Context, logger *slog.Logger, runPath string) ([]api.TerminalDoc, error) {
	out, err := scanMetadataFiles(
		ctx,
		logger,
		runPath,
		defaults.TerminalsRunPath,
		"ScanTerminals",
		terminalID,
		terminalName,
	)
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		idi, idj := terminalID(out[i]), terminalID(out[j])
		if idi != idj {
			return idi < idj
		}
		return terminalName(out[i]) < terminalName(out[j])
	})

	return out, nil
}

// FindTerminalByID scans runPath/terminals/*/metadata.json and returns
// the terminal whose Spec.ID matches the given id. Returns an error if
// no terminal with that ID is found.
func FindTerminalByID(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	id string,
) (*api.TerminalDoc, error) {
	terminals, err := ScanTerminals(ctx, logger, runPath)
	if err != nil {
		return nil, err
	}
	return findMetadataBy(
		terminals,
		func(t api.TerminalDoc) bool { return string(t.Spec.ID) == id },
		fmt.Sprintf("terminal %q not found", id),
	)
}

// FindTerminalByName scans runPath/terminals/*/metadata.json and returns
// the terminal whose Spec.Name matches the given name. Returns an error
// if no terminal with that name is found.
func FindTerminalByName(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	name string,
) (*api.TerminalDoc, error) {
	terminals, err := ScanTerminals(ctx, logger, runPath)
	if err != nil {
		return nil, err
	}
	return findMetadataBy(
		terminals,
		func(t api.TerminalDoc) bool { return terminalName(t) == name },
		fmt.Sprintf("terminal with name %q not found", name),
	)
}

func terminalID(s api.TerminalDoc) string {
	return string(s.Spec.ID)
}

func terminalName(s api.TerminalDoc) string {
	return s.Spec.Name
}
