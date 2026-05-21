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

package errdefs

import (
	"errors"
	"fmt"
	"testing"
)

// The errdefs package declares only sentinel error values and has no
// executable statements, so `go test -cover` reports "no statements" for it
// (exempt from the ≥80% bar per the issue). These assertions guard the two
// properties callers rely on: every sentinel is non-nil with a stable
// message, and errors.Is matches through a %w wrap.

func TestSentinels_NonNilWithMessage(t *testing.T) {
	sentinels := []error{
		ErrFuncNotSet, ErrContextDone, ErrWaitOnReady, ErrWaitOnClose, ErrChildExit,
		ErrSpecCmdMissing, ErrOpenSocketCtrl, ErrStartRPCServer, ErrStartTerminal, ErrAttach,
		ErrClientMode, ErrRPCServerExited, ErrOnClose, ErrCloseReq, ErrTerminalCmdStart,
		ErrWriteMetadata, ErrStartCmd, ErrDetachTerminal, ErrSetupShell, ErrInitShell,
		ErrProgramExited, ErrNoSpecDefined, ErrAttachNoTerminalSpec, ErrTerminalNotFoundByID,
		ErrTerminalNotFoundByName, ErrTerminalNameInUse, ErrTerminalMetadataNotFound,
		ErrNoTerminalSpec, ErrConfig, ErrLoggerNotFound, ErrInvalidFlag, ErrInvalidOption,
		ErrStdinStat, ErrStdinEmpty, ErrInvalidArgument, ErrOpenSpecFile, ErrInvalidJSONSpec,
		ErrTerminalSpecNotFound, ErrBuildTerminalSpec, ErrNoTerminalIdentifier,
		ErrTooManyArguments, ErrCreateClientDir, ErrResolveTerminalName,
		ErrNoTerminalIdentification, ErrNoClientIdentifier, ErrConflictingFlags,
		ErrBuildSocketPath, ErrDetermineRunPath, ErrRunPathRequired, ErrNoClientIdentification,
		ErrPositionalWithFlags, ErrInvalidOutputFormat, ErrGetRunPath, ErrNoTerminalsFound,
		ErrNoClientsFound, ErrTerminalNotFound, ErrClientNotFound, ErrStopTerminal,
		ErrStopTimeout, ErrSignalProcess, ErrTerminalStdinClosed, ErrSubscriberLagged,
		ErrInvalidProfiles, ErrClientDetached, ErrPeerClosed,
	}
	seen := make(map[string]bool, len(sentinels))
	for i, e := range sentinels {
		if e == nil {
			t.Errorf("sentinel index %d is nil", i)
			continue
		}
		msg := e.Error()
		if msg == "" {
			t.Errorf("sentinel index %d has empty message", i)
		}
		if seen[msg] {
			t.Errorf("duplicate sentinel message %q", msg)
		}
		seen[msg] = true
	}
}

func TestSentinels_MatchThroughWrap(t *testing.T) {
	wrapped := fmt.Errorf("context: %w", ErrTerminalNotFound)
	if !errors.Is(wrapped, ErrTerminalNotFound) {
		t.Error("errors.Is(wrapped, ErrTerminalNotFound) = false, want true")
	}
	if errors.Is(wrapped, ErrClientNotFound) {
		t.Error("errors.Is matched the wrong sentinel")
	}
}
