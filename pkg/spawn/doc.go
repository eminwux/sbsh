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

// Package spawn exposes library-level helpers for launching sbsh Terminal
// and Client subprocesses and driving their lifecycle from external Go
// callers. It wraps the existing "sbsh terminal -" and "sb attach" CLI
// entry points so library consumers (e.g. kukeon) can start, observe and
// stop Terminal/Client processes without shelling out by hand.
//
// Stability: pre-v1. Signatures and option fields in this package may
// change between minor releases until the umbrella issue (#118) closes.
package spawn
