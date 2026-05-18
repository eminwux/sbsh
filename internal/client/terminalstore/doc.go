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

// Package terminalstore is the client-side, in-memory registry of
// terminals known to a single sbsh client process.
//
// # Scope and lifetime
//
// A TerminalStore is created fresh inside client.Controller.Run (see
// internal/client/controller.go) and dies with the controller goroutine
// when the client process exits or its context is cancelled. The store
// holds *api.AttachedTerminal values keyed by api.ID; it is not
// persisted to disk.
//
// The authoritative inventory of terminals across the host lives on
// disk under <runPath>/terminals/<id>/. That directory tree is the
// source of truth — owned by the per-terminal supervisor process and
// read by package internal/discovery. Stale-entry reconciliation for
// terminals whose supervisor crashed without a graceful stop happens
// there (discovery.ReconcileTerminals + internal/pidutil for
// PID-reuse safety), not in this package. Likewise, name-collision
// rejection on terminal creation lives in
// discovery.VerifyTerminalNameAvailable (issue #114, PR #182). This
// package never touches the filesystem and never inspects PIDs.
//
// # Concurrency
//
// All TerminalStore operations on the production Exec implementation
// are guarded by a single sync.RWMutex. Read methods (Get, ListLive,
// Current) take the read lock; mutators (Add, Remove, SetCurrent)
// take the write lock. Each compound check-then-write sequence —
// Add's "exists? then insert (and maybe set current)" and
// SetCurrent's "exists? then assign" — runs under one write-lock
// acquisition, so they are atomic with respect to concurrent callers.
// There is no internal TOCTOU window.
//
// The store is keyed by api.ID exclusively. There is no name-keyed
// lookup, and no callers route from a store entry to a kill(2)
// target — the PID-reuse-safe signal path lives in
// internal/pidutil.Match, gated by the start-time token persisted in
// TerminalStatus.PidStart (issue #109 tracks the pidfd_open
// alternative). A TOCTOU between "find by name" and "send signal"
// cannot arise through this store.
//
// # Test double
//
// session_store_fake.go provides Test, a function-stub fake intended
// for unit tests of code that depends on the TerminalStore interface.
package terminalstore
