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

// Package api defines the declarative document types that external
// library consumers use to address sbsh objects: TerminalDoc,
// TerminalSpec, TerminalProfileDoc, ClientDoc, ConfigurationDoc, and the
// request/response payloads exchanged over the JSON-RPC control channel.
//
// This package is the type-system counterpart of the YAML/JSON manifests
// that the sbsh CLI accepts on its own command line. Library callers use
// it together with pkg/builder (to construct specs/docs), pkg/spawn (to
// run terminals and clients as subprocesses), pkg/discovery (to enumerate
// live objects under a run path), pkg/rpcclient (to drive them over
// their control sockets), and pkg/errors (to branch on well-known
// sentinels).
//
// # Pre-v1 stability policy
//
// sbsh is pre-v1. The intent of this policy is to give library
// consumers — notably the kukeon integration that drives umbrella
// issue #118 — enough stability to ship without freezing the surface
// before v1 has been validated in anger.
//
// Packages covered by this policy:
//
//   - pkg/api
//   - pkg/builder
//   - pkg/discovery
//   - pkg/errors
//   - pkg/rpcclient (and its /client and /terminal subpackages)
//   - pkg/spawn
//
// What will not change between minor releases (0.x → 0.x+1):
//
//   - Wire schema of documents whose apiVersion is sbsh/v1beta1
//     (TerminalProfileDoc, TerminalDoc, ClientDoc, ConfigurationDoc).
//     Existing fields will not be removed, renamed, or re-typed within
//     the v1beta1 apiVersion; new fields may be added.
//   - The identity of error sentinels re-exported from pkg/errors.
//     errors.Is against those names will continue to match errors
//     produced anywhere in sbsh.
//   - The shape of exported interfaces in pkg/api
//     (TerminalController, ClientController): method removals and
//     signature-breaking changes will not happen within a minor
//     release.
//
// What may change between minor releases:
//
//   - Option/With* function names and defaults in pkg/builder and
//     pkg/spawn. These are deliberately flagged pre-v1 by their own
//     doc.go files; callers should expect churn as the library
//     ergonomics settle.
//   - Additions to the curated sentinel set in pkg/errors.
//   - Additions to exported interfaces in pkg/api (new methods), and
//     additions of new request/response payload types.
//
// What will not change without a deprecation cycle:
//
//   - Removal of any symbol currently documented in pkg/api,
//     pkg/errors, or the subpackages listed above. Removals will be
//     staged by marking the symbol deprecated in a minor release and
//     removing it no earlier than the next minor release.
//
// v1 will be cut when the umbrella issue #118 closes and the library
// surface has shipped at least one downstream integration. At v1 the
// apiVersion advances out of v1beta1 and the above "may change" list
// collapses into "will not change without a deprecation cycle".
package api
