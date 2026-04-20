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

//go:build linux
// +build linux

package pidutil

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// readStartTime returns field 22 of /proc/<pid>/stat (starttime: clock ticks
// after boot when the process started). The comm field is wrapped in parens
// and may itself contain spaces or parens, so we split after the LAST ')'.
func readStartTime(pid int) (uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	rparen := strings.LastIndexByte(string(data), ')')
	if rparen < 0 || rparen+1 >= len(data) {
		return 0, fmt.Errorf("pidutil: malformed /proc/%d/stat", pid)
	}
	fields := strings.Fields(string(data[rparen+1:]))
	// After the closing ')' the next field is #3 (state), so #22 is index 19.
	const startTimeIdx = 19
	if len(fields) <= startTimeIdx {
		return 0, fmt.Errorf("pidutil: /proc/%d/stat: only %d fields after comm", pid, len(fields))
	}
	return strconv.ParseUint(fields[startTimeIdx], 10, 64)
}
