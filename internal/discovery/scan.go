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
)

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
