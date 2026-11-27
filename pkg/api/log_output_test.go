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

import (
	"strings"
	"testing"
)

func assertListsValidLogOutputs(t *testing.T, in, got string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("NormalizeLogOutput(%q) = %q, nil; want error", in, got)
	}
	for _, valid := range []string{LogOutputFile, LogOutputStdout} {
		if !strings.Contains(err.Error(), valid) {
			t.Errorf("error %q does not list valid value %q", err, valid)
		}
	}
}

func TestNormalizeLogOutput(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: LogOutputFile},
		{in: "file", want: LogOutputFile},
		{in: "stdout", want: LogOutputStdout},
		{in: "stderr", wantErr: true},
		{in: "STDOUT", wantErr: true},
		{in: "journald", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := NormalizeLogOutput(tc.in)
			if tc.wantErr {
				assertListsValidLogOutputs(t, tc.in, got, err)
				return
			}
			if err != nil {
				t.Fatalf("NormalizeLogOutput(%q) error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeLogOutput(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}
