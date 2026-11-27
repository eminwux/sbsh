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

package terminalrunner

import (
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

// Activity returns the terminal's in-memory I/O counters. It takes only
// obsMu (read side) and touches neither the PTY, metadata, ptyPipes nor
// the attacher bookkeeping, so it answers before StartTerminal, after the
// child exits, and never blocks on PTY I/O.
//
// lastRead is stamped from time.Now() in terminalManagerReader and keeps
// its monotonic clock reading, so time.Since is immune to wall-clock jumps.
func (sr *Exec) Activity(_ *api.ActivityArgs) (*api.ActivityResult, error) {
	sr.obsMu.RLock()
	res := &api.ActivityResult{
		StartedAt:    sr.startedAt,
		BytesOut:     sr.bytesOut,
		BytesIn:      sr.bytesIn,
		LastOutputAt: sr.lastRead,
	}
	sr.obsMu.RUnlock()

	if !res.LastOutputAt.IsZero() {
		res.IdleFor = time.Since(res.LastOutputAt)
	}
	return res, nil
}
