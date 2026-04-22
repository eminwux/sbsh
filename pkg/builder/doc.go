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

// Package builder exposes library-level helpers for constructing
// TerminalSpec and ClientDoc values from a profile name or inline
// fields, without importing internal/ packages or round-tripping
// through YAML. It is the public counterpart of the CLI flag
// plumbing today and the intended building block for spawn calls
// (see pkg/spawn).
//
// Stability: pre-v1. Option names and defaults in this package may
// change between minor releases until the umbrella issue (#118)
// closes.
package builder
