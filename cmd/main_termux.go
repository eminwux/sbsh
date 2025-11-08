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

package main

import (
	"os"
	"path/filepath"
)

func filterTermuxArgs() []string {
	// Check if running in Termux
	if os.Getenv("TERMUX_VERSION") == "" {
		return os.Args // No filtering needed
	}

	// Get absolute path of current executable
	exePath, err := os.Executable()
	if err != nil {
		return os.Args // Fallback to original args
	}

	exeAbs, err := filepath.Abs(exePath)
	if err != nil {
		return os.Args
	}

	exeResolved, err := filepath.EvalSymlinks(exeAbs)
	if err != nil {
		exeResolved = exeAbs
	}

	// Filter os.Args[1] if it matches the executable path
	filtered := make([]string, 0, len(os.Args))
	filtered = append(filtered, os.Args[0]) // Always keep os.Args[0]

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		argAbs, errAbs := filepath.Abs(arg)
		if errAbs != nil {
			filtered = append(filtered, arg)
			continue
		}
		argResolved, errResolved := filepath.EvalSymlinks(argAbs)
		if errResolved != nil {
			argResolved = argAbs
		}

		// Skip if this argument matches the executable path
		if argResolved == exeResolved || argAbs == exeAbs || arg == exePath || arg == exeAbs {
			continue
		}
		filtered = append(filtered, arg)
	}

	return filtered
}
