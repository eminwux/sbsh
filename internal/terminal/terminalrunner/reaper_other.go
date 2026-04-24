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

//go:build !linux

package terminalrunner

import "log/slog"

// reaper is a no-op on non-Linux builds. The PID-1 init-mode paths are only
// reachable on Linux because that is the only OS sbsh supports as a container
// PID 1.
type reaper struct{}

func newReaper(_ *slog.Logger) *reaper          { return &reaper{} }
func (r *reaper) Start()                        {}
func (r *reaper) Stop()                         {}
func (r *reaper) RegisterChild(_ int)           {}
func (r *reaper) TrackedExitCh() <-chan int     { return nil }
