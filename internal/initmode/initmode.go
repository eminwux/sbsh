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

// Package initmode reports whether the current sbsh process should take on
// PID-1 responsibilities (zombie reaping, signal forwarding). It exists so
// those code paths can be exercised from tests without actually being PID 1.
package initmode

import (
	"os"
	"sync/atomic"
)

// Values of override; zero means no override.
const (
	overrideOn  int32 = 1
	overrideOff int32 = 2
)

// override, when non-zero, forces IsInit to return true (1) or false (2)
// regardless of os.Getpid(). Tests flip this via Enable.
//
//nolint:gochecknoglobals // process-wide test override; atomic so readers need no lock
var override atomic.Int32

// IsInit reports whether this process should act as container init. Default
// is os.Getpid() == 1. Tests can force the value via Enable.
func IsInit() bool {
	switch override.Load() {
	case overrideOn:
		return true
	case overrideOff:
		return false
	default:
		return os.Getpid() == 1
	}
}

// Enable forces IsInit to return on. It must be called before any subsystem
// that reads IsInit has started. Passing false forces IsInit off.
func Enable(on bool) {
	if on {
		override.Store(overrideOn)
	} else {
		override.Store(overrideOff)
	}
}

// Reset clears any Enable override, returning IsInit to the os.Getpid()
// default. Intended for test teardown.
func Reset() {
	override.Store(0)
}
