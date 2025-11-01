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

package filter

// Filter can transform or consume input before writing to dst.
// It may keep internal state across calls (escape prefixes, paste mode, etc.).
type Filter interface {
	// Process buf[0:n]. It can:
	//  - return out == nil, nwrite == 0 to swallow,
	//  - return a subslice of buf to write in place (no alloc),
	//  - or return an alternate slice (alloc) if it had to edit.
	// It may be called repeatedly until all input is consumed.
	Process(buf []byte, n int) ([]byte, int, int, error)
}
