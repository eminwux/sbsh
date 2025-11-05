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

package terminalrunner

import "errors"

var (
	ErrCloseReq  error = errors.New("received close order")
	ErrPipeWrite error = errors.New("could not write pty->pipe")
	ErrPipeRead  error = errors.New("could not read pipe->pipe")
)

var (
	ErrTerminalRead  error = errors.New("could not read pipe->pty")
	ErrTerminalWrite error = errors.New("could not write pty->pipe")
)
