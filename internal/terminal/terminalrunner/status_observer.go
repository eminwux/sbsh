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
	"maps"
	"slices"

	"github.com/eminwux/sbsh/pkg/api"
)

// ExecOption configures an Exec at construction time. See
// NewTerminalRunnerExecWithOptions.
type ExecOption func(*Exec)

// WithStatusObserver registers fn to receive a copy of the terminal
// document after every committed status change: each state transition,
// each attach and detach, and the exit details written on close. fn runs
// on a dedicated goroutine and never blocks the runner. Documents are
// delivered in commit order through a single-slot mailbox, so while fn is
// busy only the most recent pending document is kept and intermediate ones
// are dropped. After Close posts the final document no further calls are
// made; the final call may still be running, or not yet started, when
// Close returns. A nil fn is ignored.
func WithStatusObserver(fn func(api.TerminalDoc)) ExecOption {
	return func(sr *Exec) {
		sr.statusObserver = fn
	}
}

// startStatusDispatcher starts the delivery goroutine when an observer is
// registered. Called once from the constructor; Close stops it via
// closeStatusObserver.
func (sr *Exec) startStatusDispatcher() {
	if sr.statusObserver == nil {
		return
	}
	sr.statusCh = make(chan api.TerminalDoc, 1)
	go sr.runStatusDispatcher(sr.statusObserver, sr.statusCh)
}

func (sr *Exec) runStatusDispatcher(fn func(api.TerminalDoc), ch <-chan api.TerminalDoc) {
	for doc := range ch {
		sr.deliverStatus(fn, doc)
	}
}

// deliverStatus calls fn, turning an embedder panic into a WARN so it
// cannot take the runner down.
func (sr *Exec) deliverStatus(fn func(api.TerminalDoc), doc api.TerminalDoc) {
	defer func() {
		if r := recover(); r != nil {
			sr.logger.Warn("status observer panicked", "id", sr.id, "panic", r)
		}
	}()
	fn(doc)
}

// notifyStatus posts a copy of the current document to the observer
// mailbox, replacing any document still pending. It never blocks: statusMu
// is held only for the snapshot and the drain-then-send, and the sole
// sender always finds the one-slot buffer empty after draining. Taking the
// snapshot under statusMu keeps posts in commit order, so the last
// document delivered is the latest state. Callers must not hold
// metadataMu. No-op without an observer or after closeStatusObserver.
func (sr *Exec) notifyStatus() {
	if sr.statusCh == nil {
		return
	}
	sr.statusMu.Lock()
	defer sr.statusMu.Unlock()
	if sr.statusClosed {
		return
	}
	sr.metadataMu.RLock()
	doc := cloneTerminalDoc(sr.metadata)
	sr.metadataMu.RUnlock()

	select {
	case <-sr.statusCh:
	default:
	}
	sr.statusCh <- doc
}

// closeStatusObserver closes the mailbox. The dispatcher delivers the
// document still pending, if any, and exits. Close does not wait for it,
// so a slow observer cannot delay shutdown. Idempotent.
func (sr *Exec) closeStatusObserver() {
	if sr.statusCh == nil {
		return
	}
	sr.statusMu.Lock()
	defer sr.statusMu.Unlock()
	if sr.statusClosed {
		return
	}
	sr.statusClosed = true
	close(sr.statusCh)
}

// cloneTerminalDoc copies doc deeply enough that the observer and the
// runner share no mutable memory in the fields either side may touch:
// labels and annotations, the attacher list and the exit code.
func cloneTerminalDoc(doc api.TerminalDoc) api.TerminalDoc {
	doc.Metadata.Labels = maps.Clone(doc.Metadata.Labels)
	doc.Metadata.Annotations = maps.Clone(doc.Metadata.Annotations)
	doc.Spec.Labels = maps.Clone(doc.Spec.Labels)
	doc.Status.Attachers = slices.Clone(doc.Status.Attachers)
	if doc.Status.ExitCode != nil {
		code := *doc.Status.ExitCode
		doc.Status.ExitCode = &code
	}
	return doc
}
