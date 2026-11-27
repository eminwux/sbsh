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

import "github.com/eminwux/sbsh/pkg/api"

// Handler is a custom JSON-RPC service an embedder registers alongside
// the built-in attach/control protocol, served on the same listener.
//
// Name is the net/rpc service name — the part before the dot in a
// "Service.Method" wire call. A client invokes a custom verb with
// method "<Name>.<MethodName>". Receiver is any value whose exported
// methods follow net/rpc's signature scheme:
//
//	func (r *Recv) MethodName(args ArgT, reply *ReplyT) error
//
// where ArgT and *ReplyT are exported (or builtin) types.
//
// Custom services are namespaced by Name, so their methods never
// collide with the built-in TerminalController methods (Ping, Attach,
// Subscribe, …). A Name equal to the built-in service is rejected — see
// WithHandlers.
type Handler struct {
	Name     string
	Receiver any
}

// Option configures a Server at construction time. Pass options to New.
type Option func(*Server)

// WithHandlers registers one or more custom JSON-RPC services to be
// served on the same listener as the built-in attach/control protocol.
// This is the extension point for embedders (e.g. kuketty surfacing
// container-setup status to its daemon) that need an extra verb on the
// already-permissioned control socket without forking the package or
// standing up a second listener.
//
// The built-in protocol is unaffected when no handler is registered.
// New validates the supplied handlers and returns an error if any has
// an empty Name, a nil Receiver, a Name that collides with the built-in
// service, or a Name duplicated within the set. A receiver whose
// methods don't match net/rpc's scheme surfaces as an error from Serve
// at registration time.
func WithHandlers(handlers ...Handler) Option {
	return func(s *Server) {
		s.handlers = append(s.handlers, handlers...)
	}
}

// WithStatusObserver registers fn to receive a copy of the terminal
// document (the same shape Metadata returns) after every committed status
// change: each state transition, each attach and detach, and the exit
// details written when the terminal closes.
//
// fn runs on a dedicated goroutine, so it never blocks PTY I/O, the RPC
// server or shutdown. Documents arrive in commit order, but while fn is
// busy only the latest pending document is kept: a slow observer gets the
// most recent state, not every intermediate one. The final call, carrying
// State Exited and any exit details, may arrive after Serve returns; call
// Metadata after Serve for a synchronous read of the final document. A
// panic in fn is recovered and logged. Treat the document as read-only.
//
// New returns an error when fn is nil. Without this option nothing
// changes.
func WithStatusObserver(fn func(api.TerminalDoc)) Option {
	return func(s *Server) {
		s.statusObserver = fn
		s.statusObserverSet = true
	}
}
