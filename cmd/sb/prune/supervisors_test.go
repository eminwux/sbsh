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

package prune

import (
	"context"
	"errors"
	"testing"

	"github.com/eminwux/sbsh/internal/errdefs"
)

func Test_ErrLoggerNotFound_Supervisor_RunE(t *testing.T) {
	cmd := NewPruneSupervisorsCmd()
	ctx := context.Background()
	// Don't set CtxLogger, so it will be nil
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, errdefs.ErrLoggerNotFound) {
		t.Fatalf("expected '%v'; got: '%v'", errdefs.ErrLoggerNotFound, err)
	}
}
