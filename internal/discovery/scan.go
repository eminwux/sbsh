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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// scanMetadataFiles reads all metadata.json files matching the pattern and unmarshals them.
// It returns a slice of unmarshaled metadata structs of type T.
func scanMetadataFiles[T any](
	ctx context.Context,
	logger *slog.Logger,
	runPath string,
	subDir string,
	logPrefix string,
	idGetter func(T) string,
	nameGetter func(T) string,
) ([]T, error) {
	pattern := filepath.Join(runPath, subDir, "*", "metadata.json")
	logger.DebugContext(ctx, fmt.Sprintf("%s: globbing for metadata", logPrefix), "pattern", pattern)
	paths, err := filepath.Glob(pattern)
	if err != nil {
		logger.ErrorContext(ctx, fmt.Sprintf("%s: glob failed", logPrefix), "error", err)
		return nil, fmt.Errorf("glob %q: %w", pattern, err)
	}

	out := make([]T, 0, len(paths))
	for _, p := range paths {
		select {
		case <-ctx.Done():
			logger.WarnContext(ctx, fmt.Sprintf("%s: context done while reading", logPrefix))
			return nil, ctx.Err()
		default:
		}
		b, errRead := os.ReadFile(p)
		if errRead != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("%s: failed to read file", logPrefix), "file", p, "error", errRead)
			return nil, fmt.Errorf("read %s: %w", p, errRead)
		}
		var s T
		if errUnmarshal := json.Unmarshal(b, &s); errUnmarshal != nil {
			logger.ErrorContext(
				ctx,
				fmt.Sprintf("%s: failed to decode file", logPrefix),
				"file",
				p,
				"error",
				errUnmarshal,
			)
			return nil, fmt.Errorf("decode %s: %w", p, errUnmarshal)
		}
		logger.DebugContext(
			ctx,
			fmt.Sprintf("%s: loaded metadata", logPrefix),
			"id",
			idGetter(s),
			"name",
			nameGetter(s),
		)
		out = append(out, s)
	}

	logger.InfoContext(ctx, fmt.Sprintf("%s: finished scanning", logPrefix), "count", len(out))
	return out, nil
}

// findMetadataBy searches through a slice of metadata items and returns the first one that matches the predicate.
// It returns a copy of the item to avoid referencing the loop variable.
func findMetadataBy[T any](
	items []T,
	predicate func(T) bool,
	notFoundErrMsg string,
) (*T, error) {
	for _, item := range items {
		if predicate(item) {
			// return a copy to avoid referencing the loop variable
			result := item
			return &result, nil
		}
	}
	//nolint:goerr113 // error message is constructed from parameter
	return nil, errors.New(notFoundErrMsg)
}

// scanAndPruneMetadata scans metadata items and prunes those that match the isExited predicate.
// It returns the number of items pruned and any error encountered.
func scanAndPruneMetadata[T any](
	ctx context.Context,
	logger *slog.Logger,
	w io.Writer,
	items []T,
	logPrefix string,
	entityName string,
	isExited func(T) bool,
	pruneFunc func(*slog.Logger, *T) error,
	idGetter func(T) string,
	nameGetter func(T) string,
) (int, error) {
	logger.DebugContext(ctx, fmt.Sprintf("%s: scanning %s for pruning", logPrefix, entityName))
	pruned := 0
	for _, item := range items {
		if isExited(item) {
			logger.InfoContext(ctx, fmt.Sprintf("%s: pruning %s", logPrefix, entityName), "id", idGetter(item))
			if errC := pruneFunc(logger, &item); errC != nil {
				logger.ErrorContext(
					ctx,
					fmt.Sprintf("%s: failed to prune %s", logPrefix, entityName),
					"id",
					idGetter(item),
					"error",
					errC,
				)
				return pruned, fmt.Errorf("prune %s %s: %w", entityName, idGetter(item), errC)
			}
			pruned++
			if w != nil {
				fmt.Fprintf(w, "Pruned %s %s (%s)\n", entityName, idGetter(item), nameGetter(item))
			}
		}
	}
	logger.InfoContext(ctx, fmt.Sprintf("%s: prune complete", logPrefix), "pruned", pruned)
	return pruned, nil
}
