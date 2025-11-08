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

func Test_TerminalStatusMode_String(t *testing.T) {
	tests := []struct {
		name     string
		state    TerminalStatusMode
		expected string
	}{
		{
			name:     "Initializing",
			state:    Initializing,
			expected: "Initializing",
		},
		{
			name:     "Starting",
			state:    Starting,
			expected: "Starting",
		},
		{
			name:     "Ready",
			state:    Ready,
			expected: "Ready",
		},
		{
			name:     "Exited",
			state:    Exited,
			expected: "Exited",
		},
		{
			name:     "Unknown",
			state:    TerminalStatusMode(999),
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.expected {
				t.Errorf("TerminalStatusMode.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}
