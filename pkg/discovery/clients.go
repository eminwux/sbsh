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

// ScanClients enumerates every client metadata.json under
// runPath/clients/*. Results are sorted by Spec.ID (then Metadata.Name
// for ties) so consecutive scans produce stable output.
func ScanClients(ctx context.Context, logger *slog.Logger, runPath string) ([]api.ClientDoc, error) {
	out, err := scanMetadataFiles(
		ctx,
		logger,
		runPath,
		defaults.ClientsRunPath,
		"ScanClients",
		clientID,
		clientName,
	)
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		idi, idj := clientID(out[i]), clientID(out[j])
		if idi != idj {
			return idi < idj
		}
		return clientName(out[i]) < clientName(out[j])
	})

	return out, nil
}

// FindClientByName scans runPath/clients/*/metadata.json and returns
// the client whose Metadata.Name matches the given name. Returns an
// error if no client with that name is found.
func FindClientByName(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	name string,
) (*api.ClientDoc, error) {
	clients, err := ScanClients(ctx, logger, runPath)
	if err != nil {
		return nil, err
	}
	return findMetadataBy(
		clients,
		func(s api.ClientDoc) bool { return clientName(s) == name },
		fmt.Sprintf("client with name %q not found", name),
	)
}

func clientID(s api.ClientDoc) string {
	return string(s.Spec.ID)
}

func clientName(s api.ClientDoc) string {
	return s.Metadata.Name
}
