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

package pidutil

import (
	"os"
	"runtime"
	"testing"
)

func TestStartTime_SelfStable(t *testing.T) {
	a, err := StartTime(os.Getpid())
	if err != nil {
		t.Fatalf("StartTime(self) error: %v", err)
	}
	b, err := StartTime(os.Getpid())
	if err != nil {
		t.Fatalf("StartTime(self) second call error: %v", err)
	}
	if a != b {
		t.Fatalf("StartTime(self) is not stable: %d vs %d", a, b)
	}
	if runtime.GOOS == "linux" && a == 0 {
		t.Fatalf("StartTime(self) on linux returned 0; expected a non-zero token")
	}
}

func TestStartTime_NonPositivePid(t *testing.T) {
	got, err := StartTime(0)
	if err != nil || got != 0 {
		t.Fatalf("StartTime(0) = (%d, %v); want (0, nil)", got, err)
	}
	got, err = StartTime(-1)
	if err != nil || got != 0 {
		t.Fatalf("StartTime(-1) = (%d, %v); want (0, nil)", got, err)
	}
}

func TestMatch_ZeroExpectedAlwaysTrue(t *testing.T) {
	ok, err := Match(os.Getpid(), 0)
	if err != nil || !ok {
		t.Fatalf("Match(self, 0) = (%v, %v); want (true, nil)", ok, err)
	}
}

func TestMatch_SelfMatches(t *testing.T) {
	tok, err := StartTime(os.Getpid())
	if err != nil {
		t.Fatalf("StartTime(self) error: %v", err)
	}
	if tok == 0 {
		t.Skip("platform exposes no start-time token; nothing to match")
	}
	ok, err := Match(os.Getpid(), tok)
	if err != nil {
		t.Fatalf("Match(self, tok) error: %v", err)
	}
	if !ok {
		t.Fatalf("Match(self, tok) = false; want true")
	}
}

func TestMatch_DifferentToken(t *testing.T) {
	tok, err := StartTime(os.Getpid())
	if err != nil {
		t.Fatalf("StartTime(self) error: %v", err)
	}
	if tok == 0 {
		t.Skip("platform exposes no start-time token; mismatch is unreachable")
	}
	ok, err := Match(os.Getpid(), tok+1)
	if err != nil {
		t.Fatalf("Match(self, tok+1) error: %v", err)
	}
	if ok {
		t.Fatalf("Match(self, tok+1) = true; want false")
	}
}
