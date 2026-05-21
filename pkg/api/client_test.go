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

package api

import "testing"

func Test_ClientStatusMode_String(t *testing.T) {
	tests := []struct {
		name     string
		state    ClientStatusMode
		expected string
	}{
		{name: "Initializing", state: ClientInitializing, expected: "Initializing"},
		{name: "Ready", state: ClientReady, expected: "Ready"},
		{name: "Attached", state: ClientAttached, expected: "Attached"},
		{name: "Exiting", state: ClientExiting, expected: "Exiting"},
		{name: "Exited", state: ClientExited, expected: "Exited"},
		{name: "Unknown", state: ClientStatusMode(999), expected: "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("ClientStatusMode.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}
