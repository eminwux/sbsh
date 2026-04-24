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

package errors_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/eminwux/sbsh/internal/errdefs"
	sbsherrors "github.com/eminwux/sbsh/pkg/errors"
)

// TestSentinelIdentity asserts that each curated re-export in pkg/errors
// is the same sentinel value as its internal/errdefs counterpart. This is
// the "identity preserved" contract promised to external library callers:
// errors.Is(err, sbsherrors.ErrX) must match errors produced with
// errdefs.ErrX anywhere in the codebase.
func TestSentinelIdentity(t *testing.T) {
	cases := []struct {
		name     string
		reexport error
		internal error
	}{
		{"ErrTerminalNotFoundByID", sbsherrors.ErrTerminalNotFoundByID, errdefs.ErrTerminalNotFoundByID},
		{"ErrTerminalNotFoundByName", sbsherrors.ErrTerminalNotFoundByName, errdefs.ErrTerminalNotFoundByName},
		{"ErrTerminalNotFound", sbsherrors.ErrTerminalNotFound, errdefs.ErrTerminalNotFound},
		{"ErrClientNotFound", sbsherrors.ErrClientNotFound, errdefs.ErrClientNotFound},
		{"ErrStopTerminal", sbsherrors.ErrStopTerminal, errdefs.ErrStopTerminal},
		{"ErrStopTimeout", sbsherrors.ErrStopTimeout, errdefs.ErrStopTimeout},
		{"ErrWaitOnReady", sbsherrors.ErrWaitOnReady, errdefs.ErrWaitOnReady},
		{"ErrSubscriberLagged", sbsherrors.ErrSubscriberLagged, errdefs.ErrSubscriberLagged},
		{"ErrRunPathRequired", sbsherrors.ErrRunPathRequired, errdefs.ErrRunPathRequired},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.reexport != tc.internal {
				t.Fatalf("pkg/errors.%s != internal/errdefs.%s: identity not preserved",
					tc.name, tc.name)
			}
			if !errors.Is(tc.reexport, tc.internal) {
				t.Errorf("errors.Is(pkg/errors.%s, errdefs.%s) = false, want true", tc.name, tc.name)
			}
			if !errors.Is(tc.internal, tc.reexport) {
				t.Errorf("errors.Is(errdefs.%s, pkg/errors.%s) = false, want true", tc.name, tc.name)
			}
		})
	}
}

// TestWrappedErrorsMatchReexport asserts that an error produced internally
// (wrapped with %w around an errdefs sentinel) is matched by errors.Is
// against the pkg/errors re-export — i.e. consumers do not have to reach
// into internal/errdefs to branch on sbsh errors.
func TestWrappedErrorsMatchReexport(t *testing.T) {
	cases := []struct {
		name     string
		internal error
		reexport error
	}{
		{"ErrTerminalNotFoundByID", errdefs.ErrTerminalNotFoundByID, sbsherrors.ErrTerminalNotFoundByID},
		{"ErrStopTerminal", errdefs.ErrStopTerminal, sbsherrors.ErrStopTerminal},
		{"ErrSubscriberLagged", errdefs.ErrSubscriberLagged, sbsherrors.ErrSubscriberLagged},
		{"ErrRunPathRequired", errdefs.ErrRunPathRequired, sbsherrors.ErrRunPathRequired},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("operation failed: %w", tc.internal)
			if !errors.Is(wrapped, tc.reexport) {
				t.Errorf("errors.Is(wrapped, pkg/errors.%s) = false, want true", tc.name)
			}
		})
	}
}
