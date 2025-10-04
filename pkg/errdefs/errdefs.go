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

package errdefs

import "errors"

var (
	ErrFuncNotSet      = errors.New("function not set")
	ErrContextDone     = errors.New("context has been cancelled")
	ErrWaitOnReady     = errors.New("waiting for readiness has failed")
	ErrWaitOnClose     = errors.New("waiting for close has failed")
	ErrChildExit       = errors.New("child routine exited")
	ErrSpecCmdMissing  = errors.New("spec is missing Cmd")
	ErrOpenSocketCtrl  = errors.New("could not open ctrl socket")
	ErrStartRPCServer  = errors.New("error starting RPC server")
	ErrStartSession    = errors.New("error starting session")
	ErrAttach          = errors.New("error attaching supervisor")
	ErrStartSessionCmd = errors.New("error starting session cmd")
	ErrSupervisorKind  = errors.New("error supervisor kind not implemented")
	ErrRPCServerExited = errors.New("RPC Server exited with error")
	ErrOnClose         = errors.New("error closing")
	ErrCloseReq        = errors.New("close requested")
	ErrSessionStore    = errors.New("error in session store")
	ErrSessionCmdStart = errors.New("error in shell cmd")
	ErrSessionExists   = errors.New("session id already exists in store")
	ErrWriteMetadata   = errors.New("could not write metadata file")
	ErrStartCmd        = errors.New("could not start cmd")
	ErrDetachSession   = errors.New("could not detach session")
)
