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

package config

import (
	"context"
	"errors"
	"log/slog"

	"github.com/eminwux/sbsh/internal/discovery"
	"github.com/eminwux/sbsh/internal/logging"
	"github.com/eminwux/sbsh/pkg/api"
)

// AutoCompleteListProfileNames returns every profile name successfully loaded
// from profilesDir. Warnings (malformed files, duplicate names, schema-invalid
// documents) are swallowed so completion never fails or returns an empty list
// just because one file is broken — users can diagnose those with
// `sb validate profiles`.
func AutoCompleteListProfileNames(ctx context.Context, logger *slog.Logger, profilesDir string) ([]string, error) {
	// logger is not set on autocomplete calls
	if logger == nil {
		logger = logging.NewNoopLogger()
	}

	profiles, _, err := discovery.LoadProfilesFromDir(ctx, logger, profilesDir)
	if err != nil {
		logger.ErrorContext(ctx, "ListProfiles: failed to load profiles", "dir", profilesDir, "error", err)
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, errors.New("no profiles found")
	}

	var names []string
	for _, p := range profiles {
		names = append(names, p.Metadata.Name)
	}
	return names, nil
}

func AutoCompleteListTerminalNames(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	showExited bool,
) ([]string, error) {
	// logger is not set on autocomplete calls
	if logger == nil {
		logger = logging.NewNoopLogger()
	}
	terminals, err := discovery.ScanTerminals(ctx, logger, runPath)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, "ListTerminals: failed to load terminals", "path", runPath, "error", err)
		}
		return nil, err
	}
	if terminals == nil {
		return nil, errors.New("no terminals found")
	}

	var names []string
	for _, t := range terminals {
		if showExited || t.Status.State != api.Exited {
			names = append(names, t.Spec.Name)
		}
	}
	return names, nil
}

func AutoCompleteListTerminalIDs(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	showExited bool,
) ([]string, error) {
	// logger is not set on autocomplete calls
	if logger == nil {
		logger = logging.NewNoopLogger()
	}
	terminals, err := discovery.ScanTerminals(ctx, logger, runPath)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, "ListTerminals: failed to load terminals", "path", runPath, "error", err)
		}
		return nil, err
	}
	if terminals == nil {
		return nil, errors.New("no terminals found")
	}

	var ids []string
	for _, t := range terminals {
		if showExited || t.Status.State != api.Exited {
			ids = append(ids, string(t.Spec.ID))
		}
	}
	return ids, nil
}

func AutoCompleteListClientNames(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	showExited bool,
) ([]string, error) {
	// logger is not set on autocomplete calls
	if logger == nil {
		logger = logging.NewNoopLogger()
	}
	clients, err := discovery.ScanClients(ctx, logger, runPath)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, "ListClients: failed to load clients", "path", runPath, "error", err)
		}
		return nil, err
	}
	if clients == nil {
		return nil, errors.New("no clients found")
	}

	var names []string
	for _, t := range clients {
		if showExited || t.Status.State != api.ClientExited {
			names = append(names, t.Metadata.Name)
		}
	}
	return names, nil
}
