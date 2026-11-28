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

	"github.com/eminwux/sbsh/internal/terminal/terminalrunner"
)

// Client overlays let an embedder show signals inside a running workload's
// stream to attached interactive clients — "another client attached",
// "cell is stopping", a full-screen notice — without writing into the
// workload's terminal. Overlay bytes go to client connections only: they
// never reach the capture file, Screenshot (the screen model) or the
// workload's input, and the server restores each client's screen from the
// model afterwards.
//
// Only the embedder starts an overlay, in response to its own events; sbsh
// adds no RPC for it (expose one through WithHandlers if needed). Overlays
// never read or interpret keystrokes. Clients that attach with
// api.AttachRequest.NoOverlays (pkg/attach Options.DisableOverlays) never
// receive overlay bytes.
//
// Guarantees:
//   - Overlay bytes are injected only between escape sequences. A switch
//     that would split one waits for the next chunk boundary that is not
//     mid-sequence, for at most 100 ms, then aborts the client's partial
//     sequence with CAN (0x18).
//   - A takeover discards (never buffers) live output for its clients, so
//     the workload, sibling clients and the capture are never slowed.
//     Overlay paints (toast draws and erases, the restore after End or a
//     resize) never wait for a client either: a client whose output buffer
//     cannot take one is disconnected as lagged. There are two exceptions.
//     Takeover.Write blocks the caller, never the workload, while a held
//     client's buffer is full. And toast bytes sent with a forwarded chunk
//     of live output (the box erase before it, the redraw after it) follow
//     that output's backpressure, so a live client in backpressure mode is
//     paced rather than dropped.
//   - Redraws use absolute positioning and synchronized output and never
//     touch the workload's saved-cursor slot (no DECSC/DECRC).
//   - Without any overlay call, clients receive exactly the bytes they
//     would without this feature.

type (
	// Target selects attached clients: All, or the IDs in Only, minus
	// Except. Opted-out clients are never selected.
	Target = terminalrunner.Target
	// ClientInfo describes an attached interactive client.
	ClientInfo = terminalrunner.ClientInfo
	// Toast is a non-modal notice drawn in a corner box over live output.
	Toast = terminalrunner.Toast
	// ToastID identifies a shown toast.
	ToastID = terminalrunner.ToastID
	// ToastPosition is the corner a toast is drawn in.
	ToastPosition = terminalrunner.ToastPosition
	// ToastStyle selects a toast's colors.
	ToastStyle = terminalrunner.ToastStyle
	// TakeoverOptions configures BeginTakeover.
	TakeoverOptions = terminalrunner.TakeoverOptions
	// InputMode decides what happens to held clients' keystrokes.
	InputMode = terminalrunner.InputMode
	// Takeover holds live output for its clients while the embedder writes
	// frames. End it to restore their screens and resume live output.
	Takeover = terminalrunner.Takeover
)

const (
	ToastTopRight    = terminalrunner.ToastTopRight
	ToastTopLeft     = terminalrunner.ToastTopLeft
	ToastBottomRight = terminalrunner.ToastBottomRight
	ToastBottomLeft  = terminalrunner.ToastBottomLeft

	ToastInfo    = terminalrunner.ToastInfo
	ToastWarning = terminalrunner.ToastWarning
	ToastError   = terminalrunner.ToastError

	// InputDrop discards held clients' keystrokes during a takeover (the
	// default).
	InputDrop = terminalrunner.InputDrop
	// InputPass forwards them to the workload as usual.
	InputPass = terminalrunner.InputPass
)

var (
	// ErrNotStarted is returned by overlay calls before Serve started the
	// terminal.
	ErrNotStarted = errors.New("server: not started")
	// ErrOverlaysNotRunning is returned when the terminal's PTY is not up.
	ErrOverlaysNotRunning = terminalrunner.ErrOverlaysNotRunning
	// ErrToastNotFound is returned by DismissToast for an unknown toast,
	// one already dismissed or expired, or one replaced by a newer toast on
	// every client that showed it.
	ErrToastNotFound = terminalrunner.ErrToastNotFound
	// ErrTakeoverEnded is returned by Takeover.Write and a second End.
	ErrTakeoverEnded = terminalrunner.ErrTakeoverEnded
	// ErrClientBusy is returned by BeginTakeover when a targeted client is
	// already held by another takeover.
	ErrClientBusy = terminalrunner.ErrClientBusy
	// ErrEmptyToast is returned by ShowToast for a toast with no lines.
	ErrEmptyToast = terminalrunner.ErrEmptyToast
)

// overlayRunner is the overlay surface of the terminal runner Serve starts.
type overlayRunner interface {
	Clients() []ClientInfo
	ShowToast(ctx context.Context, t Target, toast Toast) (ToastID, error)
	DismissToast(id ToastID) error
	BeginTakeover(ctx context.Context, t Target, opts TakeoverOptions) (*Takeover, error)
}

// The runner Serve starts must provide overlays; a refactor that drops a
// method fails to compile here instead of at the type assertion below.
var _ overlayRunner = (*terminalrunner.Exec)(nil)

func (s *Server) overlays() (overlayRunner, error) {
	r := s.getRunner()
	if r == nil {
		return nil, ErrNotStarted
	}
	o, ok := r.(overlayRunner)
	if !ok {
		return nil, errors.New("server: terminal runner does not support overlays")
	}
	return o, nil
}

// Clients lists the attached interactive clients, oldest first, with their
// overlay opt-out state. Returns nil before Serve started the terminal.
func (s *Server) Clients() []ClientInfo {
	o, err := s.overlays()
	if err != nil {
		return nil
	}
	return o.Clients()
}

// ShowToast draws toast on the targeted clients and returns its ID. A
// client already showing a toast has it replaced; once a toast is replaced
// on every client that showed it, it is forgotten and DismissToast of its ID
// returns ErrToastNotFound. The toast's lines are
// plain text (control characters are stripped) and sbsh draws the box
// itself, restoring the cursor and text style after every redraw. It is
// redrawn after each chunk of live output, re-placed on resize, and
// removed (its area repainted from the screen model) on TTL expiry or
// DismissToast. Never blocks on a client.
func (s *Server) ShowToast(ctx context.Context, t Target, toast Toast) (ToastID, error) {
	o, err := s.overlays()
	if err != nil {
		return 0, err
	}
	return o.ShowToast(ctx, t, toast)
}

// DismissToast removes a toast from every client still showing it.
func (s *Server) DismissToast(id ToastID) error {
	o, err := s.overlays()
	if err != nil {
		return err
	}
	return o.DismissToast(id)
}

// BeginTakeover holds live output for the targeted clients and returns once
// each of them is held, so frames written next land on a quiet screen.
// During the takeover the clients' keystrokes are dropped (InputDrop, the
// default) or passed to the workload (InputPass); they are never delivered
// to the embedder, and the client-side detach keystroke keeps working.
// Call End to redraw the clients from the current screen model and resume.
func (s *Server) BeginTakeover(ctx context.Context, t Target, opts TakeoverOptions) (*Takeover, error) {
	o, err := s.overlays()
	if err != nil {
		return nil, err
	}
	return o.BeginTakeover(ctx, t, opts)
}
