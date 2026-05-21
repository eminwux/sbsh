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

package naming

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestRandomName_FormatAndWordlists(t *testing.T) {
	leftSet := make(map[string]bool, len(left))
	for _, w := range left {
		leftSet[w] = true
	}
	rightSet := make(map[string]bool, len(right))
	for _, w := range right {
		rightSet[w] = true
	}

	for i := 0; i < 200; i++ {
		name := RandomName()
		parts := strings.SplitN(name, "_", 2)
		if len(parts) != 2 {
			t.Fatalf("RandomName() = %q; want exactly one '_' separator", name)
		}
		if !leftSet[parts[0]] {
			t.Errorf("RandomName() left token %q not in left wordlist", parts[0])
		}
		if !rightSet[parts[1]] {
			t.Errorf("RandomName() right token %q not in right wordlist", parts[1])
		}
	}
}

func TestRandomName_ProducesVariety(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		seen[RandomName()] = true
	}
	// With ~100x100 combinations, 100 draws should yield well more than one
	// distinct value; a single value would signal a broken seed.
	if len(seen) < 2 {
		t.Fatalf("RandomName() produced %d distinct values over 100 draws; want variety", len(seen))
	}
}

func TestRandomID_LengthAndHex(t *testing.T) {
	for i := 0; i < 50; i++ {
		id := RandomID()
		// 4 random bytes hex-encoded → 8 hex chars.
		if len(id) != 8 {
			t.Fatalf("RandomID() = %q (len %d); want 8 hex chars", id, len(id))
		}
		if _, err := hex.DecodeString(id); err != nil {
			t.Errorf("RandomID() = %q is not valid hex: %v", id, err)
		}
	}
}

func TestRandomID_ProducesVariety(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		seen[RandomID()] = true
	}
	if len(seen) < 2 {
		t.Fatalf("RandomID() produced %d distinct values over 100 draws; want variety", len(seen))
	}
}

func TestRandSeed_NonTrivial(t *testing.T) {
	// randSeed reads from crypto/rand; over several calls it should not return
	// a constant value. This exercises the success path of the helper.
	seeds := make(map[int64]bool)
	for i := 0; i < 20; i++ {
		seeds[randSeed()] = true
	}
	if len(seeds) < 2 {
		t.Fatalf("randSeed() returned %d distinct values over 20 calls; want variety", len(seeds))
	}
}
