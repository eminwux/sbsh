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

package attach

import "errors"

// ErrSocketPathRequired is returned from Run when Options.SocketPath is
// empty. Embedders branching on error type can errors.Is against this
// sentinel to distinguish a missing required option from any other
// failure surfaced by the underlying controller.
var ErrSocketPathRequired = errors.New("pkg/attach: Options.SocketPath is required")

// ErrDetached signals that Run returned because the operator issued a
// clean detach — either by pressing the in-band ^]^] keystroke or by
// some peer triggering a Detach RPC against the controller. The remote
// terminal is still alive; embedders can re-attach to the same
// SocketPath. Match with errors.Is to branch on this case (e.g. mapping
// it to a zero exit code or leaving a managed cell running).
var ErrDetached = errors.New("pkg/attach: client detached")

// ErrPeerClosed signals that Run returned because the remote terminal
// dropped the IO connection — workload exited, peer crashed, or the
// session otherwise ended on the far side. Re-attaching to the same
// SocketPath will fail. Match with errors.Is to branch on this case
// (e.g. tearing down a managed cell or surfacing a non-zero exit).
var ErrPeerClosed = errors.New("pkg/attach: peer closed connection")
