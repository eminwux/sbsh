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

package types_test

import (
	"context"
	"testing"

	"github.com/eminwux/sbsh/cmd/types"
)

// The types package declares only context-key constants and a key type; it
// has no executable statements, so `go test -cover` reports "no statements"
// for it (exempt from the ≥80% bar per the issue). These assertions guard
// that the keys stay distinct and round-trip through context.WithValue —
// the property logging.SetupFileLogger relies on.

func TestCtxKeys_Distinct(t *testing.T) {
	keys := []types.CtxLoggerType{
		types.CtxLogger,
		types.CtxLevelVar,
		types.CtxHandler,
		types.CtxCloser,
		types.CtxTerminalSpec,
	}
	seen := make(map[types.CtxLoggerType]bool, len(keys))
	for _, k := range keys {
		if seen[k] {
			t.Errorf("duplicate context key %q", k)
		}
		seen[k] = true
	}
	if len(seen) != len(keys) {
		t.Errorf("got %d distinct keys, want %d", len(seen), len(keys))
	}
}

func TestCtxKeys_RoundTripThroughContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.CtxLogger, "logger-value")
	ctx = context.WithValue(ctx, types.CtxCloser, "closer-value")

	if got := ctx.Value(types.CtxLogger); got != "logger-value" {
		t.Errorf("ctx.Value(CtxLogger) = %v, want logger-value", got)
	}
	if got := ctx.Value(types.CtxCloser); got != "closer-value" {
		t.Errorf("ctx.Value(CtxCloser) = %v, want closer-value", got)
	}
	// A bare string with the same text must NOT collide with the typed key.
	if got := ctx.Value("logger"); got != nil {
		t.Errorf("ctx.Value(\"logger\") = %v, want nil (typed key must not collide)", got)
	}
}
