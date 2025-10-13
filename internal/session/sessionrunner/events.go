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

package sessionrunner

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

type SessionRunnerEventType int

type SessionRunnerEvent struct {
	ID    api.ID
	Type  SessionRunnerEventType
	Bytes int   // for EvData
	Err   error // for EvClosed/EvError
	When  time.Time
}

const (
	EvError SessionRunnerEventType = iota // abnormal error
	EvCmdExited
)

// helper: non-blocking event send so the PTY reader never stalls
func trySendEvent(logger *slog.Logger, ch chan<- SessionRunnerEvent, ev SessionRunnerEvent) {
	logger.Debug(
		fmt.Sprintf(
			"[session] send event: id=%s type=%v err=%v when=%s\r\n",
			ev.ID,
			ev.Type,
			ev.Err,
			ev.When.Format(time.RFC3339Nano),
		),
	)

	select {
	case ch <- ev:
	default:
		// drop on the floor if controller is momentarily busy; channel should be buffered
	}
}
