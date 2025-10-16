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
)

func AutoCompleteProfiles(ctx context.Context, logger *slog.Logger, profilesFile string) ([]string, error) {
	// logger is not set on autocomplete calls
	if logger == nil {
		logger = logging.NewNoopLogger()
	}

	profiles, err := discovery.LoadProfilesFromPath(ctx, logger, profilesFile)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, "ListProfiles: failed to load profiles", "path", profilesFile, "error", err)
		}
		return nil, err
	}
	if profiles == nil {
		return nil, errors.New("no profiles found")
	}

	var names []string
	for _, p := range profiles {
		names = append(names, p.Metadata.Name)
	}
	return names, nil
}

func AutoCompleteListSessions(ctx context.Context, logger *slog.Logger, runPath string) ([]string, error) {
	// logger is not set on autocomplete calls
	if logger == nil {
		logger = logging.NewNoopLogger()
	}
	sessions, err := discovery.ScanSessions(ctx, logger, runPath)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, "ListSessions: failed to load sessions", "path", runPath, "error", err)
		}
		return nil, err
	}
	if sessions == nil {
		return nil, errors.New("no sessions found")
	}

	var names []string
	for _, s := range sessions {
		names = append(names, s.Spec.Name)
	}
	return names, nil
}
