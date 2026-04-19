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

// ScanAndPrintClients finds all metadata.json under runPath/clients/*,
// unmarshals them into api.ClientSpec, and prints them to w.
// format: "" (compact table), "wide" (full table), "json", "yaml".
func ScanAndPrintClients(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	w io.Writer,
	printAll bool,
	format string,
) error {
	logger.DebugContext(ctx, "ScanAndPrintClients: scanning clients", "runPath", runPath)
	clients, err := ScanClients(ctx, logger, runPath)
	if err != nil {
		logger.ErrorContext(ctx, "ScanAndPrintClients: failed to scan clients", "error", err)
		return err
	}
	ReconcileClients(ctx, logger, runPath, clients)
	logger.InfoContext(ctx, "ScanAndPrintClients: scanned clients", "count", len(clients))

	switch format {
	case "json", "yaml":
		filtered := filterClients(clients, printAll)
		return printTerminalMetadata(w, filtered, format)
	case "", "wide":
		return printClients(w, clients, printAll, format == "wide")
	default:
		return fmt.Errorf("unknown output format: %q (use wide|json|yaml)", format)
	}
}

func filterClients(clients []api.ClientDoc, printAll bool) []api.ClientDoc {
	if printAll {
		return clients
	}
	out := make([]api.ClientDoc, 0, len(clients))
	for _, c := range clients {
		if c.Status.State != api.ClientExited {
			out = append(out, c)
		}
	}
	return out
}

// ScanAndPruneClients finds all metadata.json under runPath/clients/*,
// unmarshals them into api.ClientSpec, and removes the client folders
// for clients that are in Exited state.
func ScanAndPruneClients(ctx context.Context, logger *slog.Logger, runPath string, w io.Writer) error {
	clients, err := ScanClients(ctx, logger, runPath)
	if err != nil {
		logger.ErrorContext(ctx, "ScanAndPruneClients: failed to scan clients", "error", err)
		return err
	}
	_, err = scanAndPruneMetadata(
		ctx,
		logger,
		w,
		clients,
		"ScanAndPruneClients",
		"client",
		func(s api.ClientDoc) bool { return s.Status.State == api.ClientExited },
		PruneClient,
		clientID,
		clientName,
	)
	return err
}

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

	// Optional: stable order by ID (fallback to Name if ID empty)
	sort.Slice(out, func(i, j int) bool {
		idi, idj := clientID(out[i]), clientID(out[j])
		if idi != idj {
			return idi < idj
		}
		return clientName(out[i]) < clientName(out[j])
	})

	return out, nil
}

func PruneClient(logger *slog.Logger, metadata *api.ClientDoc) error {
	logger.DebugContext(
		context.Background(),
		"PruneClient: pruning client folder",
		"path",
		metadata.Status.BaseRunPath,
	)
	err := os.RemoveAll(metadata.Status.ClientRunPath)
	if err != nil {
		logger.ErrorContext(
			context.Background(),
			"PruneClient: failed to remove client folder",
			"path",
			metadata.Status.ClientRunPath,
			"error",
			err,
		)
	} else {
		logger.InfoContext(context.Background(), "PruneClient: client folder removed", "path", metadata.Status.ClientRunPath)
	}
	return err
}

func clientID(s api.ClientDoc) string {
	// If your type uses Id instead of ID, change to: return s.Id
	return string(s.Spec.ID)
}

func clientName(s api.ClientDoc) string {
	return s.Metadata.Name
}

func clientLabels(s api.ClientDoc) map[string]string {
	if len(s.Metadata.Labels) != 0 {
		return s.Metadata.Labels
	}
	return map[string]string{}
}

func printClients(w io.Writer, clients []api.ClientDoc, printAll, wide bool) error {
	//nolint:mnd // tabwriter padding
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	activeCount := 0
	for _, s := range clients {
		if s.Status.State != api.ClientExited {
			activeCount++
		}
	}

	if len(clients) == 0 {
		fmt.Fprint(tw, NoClientsString)
		return tw.Flush()
	}

	if !printAll && activeCount == 0 {
		fmt.Fprintln(tw, "no active clients found")
		return tw.Flush()
	}

	if wide {
		fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tLABELS")
	} else {
		fmt.Fprintln(tw, "NAME\tSTATUS")
	}
	for _, s := range clients {
		if s.Status.State == api.ClientExited && !printAll {
			continue
		}
		if wide {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				clientID(s),
				clientName(s),
				s.Status.State.String(),
				joinLabels(clientLabels(s)),
			)
		} else {
			fmt.Fprintf(tw, "%s\t%s\n",
				clientName(s),
				s.Status.State.String(),
			)
		}
	}
	return tw.Flush()
}

// FindClientByName scans runPath/clients/*/metadata.json and returns
// the client whose Metadata.Name matches the given name. If not found, returns nil.
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

// FindAndPrintClientMetadata finds all metadata.json under runPath/clients/*,
// unmarshals them into api.ClientSpec, and prints a table to w.
func FindAndPrintClientMetadata(
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	w io.Writer,
	terminalName string,
	format string,
) error {
	logger.DebugContext(ctx, "FindAndPrintClientMetadata: scanning clients", "runPath", runPath)
	clients, err := FindClientByName(ctx, logger, runPath, terminalName)
	if err != nil {
		logger.ErrorContext(ctx, "FindAndPrintClientMetadata: failed to scan clients", "error", err)
		return err
	}
	if clients != nil {
		reconcileClient(ctx, logger, runPath, clients)
	}
	return printTerminalMetadata(w, clients, format)
}
