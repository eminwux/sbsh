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

package initmode

import (
	"os"
	"testing"
)

func TestIsInit_DefaultMatchesPID(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	want := os.Getpid() == 1
	if got := IsInit(); got != want {
		t.Fatalf("IsInit()=%v, want %v (pid=%d)", got, want, os.Getpid())
	}
}

func TestEnable_ForcesValue(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	Enable(true)
	if !IsInit() {
		t.Fatalf("Enable(true): IsInit()=false, want true")
	}

	Enable(false)
	if IsInit() {
		t.Fatalf("Enable(false): IsInit()=true, want false")
	}

	Reset()
	want := os.Getpid() == 1
	if got := IsInit(); got != want {
		t.Fatalf("after Reset: IsInit()=%v, want %v", got, want)
	}
}
