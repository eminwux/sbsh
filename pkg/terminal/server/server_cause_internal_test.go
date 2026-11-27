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

package server

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/eminwux/sbsh/internal/terminal/terminalrunner"
)

// TestTerminalCause_PTYErrorIsNotExitError: a genuine PTY failure with no
// following EvCmdExited stays the PTY error, which errors.As does not
// match as an ExitError.
func TestTerminalCause_PTYErrorIsNotExitError(t *testing.T) {
	s := &Server{}
	ptyErr := fmt.Errorf("%w: read /dev/ptmx: input/output error", terminalrunner.ErrPipeRead)
	cause := s.terminalCause(
		context.Background(),
		terminalrunner.Event{Type: terminalrunner.EvError, Err: ptyErr},
		make(chan terminalrunner.Event),
	)
	if !errors.Is(cause, terminalrunner.ErrPipeRead) {
		t.Fatalf("cause = %v, want the PTY error", cause)
	}
	var ee *ExitError
	if errors.As(cause, &ee) {
		t.Fatalf("PTY error matched *ExitError: %v", cause)
	}
}

// TestTerminalCause_ExitAfterPTYErrorIsExitError: the #438 race, where a
// benign PTY EIO precedes the exit, still yields the ExitError.
func TestTerminalCause_ExitAfterPTYErrorIsExitError(t *testing.T) {
	s := &Server{}
	code := 3
	exitErr := &ExitError{Code: &code}
	events := make(chan terminalrunner.Event, 1)
	events <- terminalrunner.Event{Type: terminalrunner.EvCmdExited, Err: exitErr}
	cause := s.terminalCause(
		context.Background(),
		terminalrunner.Event{Type: terminalrunner.EvError, Err: terminalrunner.ErrPipeRead},
		events,
	)
	var ee *ExitError
	if !errors.As(cause, &ee) || ee.Code == nil || *ee.Code != 3 {
		t.Fatalf("cause = %v, want ExitError with code 3", cause)
	}
}
