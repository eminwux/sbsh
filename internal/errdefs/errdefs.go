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
	ErrFuncNotSet               = errors.New("function not set")
	ErrContextDone              = errors.New("context has been cancelled")
	ErrWaitOnReady              = errors.New("waiting for readiness has failed")
	ErrWaitOnClose              = errors.New("waiting for close has failed")
	ErrChildExit                = errors.New("child routine exited")
	ErrSpecCmdMissing           = errors.New("spec is missing Cmd")
	ErrOpenSocketCtrl           = errors.New("could not open ctrl socket")
	ErrStartRPCServer           = errors.New("error starting RPC server")
	ErrStartTerminal            = errors.New("error starting terminal terminal")
	ErrAttach                   = errors.New("error attaching supervisor")
	ErrSupervisorKind           = errors.New("error supervisor kind not implemented")
	ErrRPCServerExited          = errors.New("RPC Server exited with error")
	ErrOnClose                  = errors.New("error closing")
	ErrCloseReq                 = errors.New("close requested")
	ErrTerminalStore            = errors.New("error in terminal store")
	ErrTerminalCmdStart         = errors.New("error in shell cmd")
	ErrTerminalExists           = errors.New("terminal id already exists in store")
	ErrWriteMetadata            = errors.New("could not write metadata file")
	ErrStartCmd                 = errors.New("could not start cmd")
	ErrDetachTerminal           = errors.New("could not detach terminal")
	ErrSetupShell               = errors.New("could not setup shell")
	ErrInitShell                = errors.New("error on init shell stage")
	ErrProgramExited            = errors.New("program exited")
	ErrNoSpecDefined            = errors.New("no spec provided")
	ErrAttachNoTerminalSpec     = errors.New("no terminal ID or Name provided for attach")
	ErrTerminalNotFoundByID     = errors.New("could not find terminal by ID")
	ErrTerminalNotFoundByName   = errors.New("could not find terminal by Name")
	ErrTerminalMetadataNotFound = errors.New("no terminal metadata found to attach")
	ErrNoTerminalSpec           = errors.New("no terminal spec found")
	ErrConfig                   = errors.New("config error")
	ErrLoggerNotFound           = errors.New("logger not found in context")
	ErrInvalidFlag              = errors.New("invalid flag usage")
	ErrStdinStat                = errors.New("failed to stat stdin")
	ErrStdinEmpty               = errors.New("no data on stdin: use a pipe or redirect when using '-'")
	ErrInvalidArgument          = errors.New("invalid positional argument")
	ErrOpenSpecFile             = errors.New("failed to open spec file")
	ErrInvalidJSONSpec          = errors.New("invalid JSON spec")
	ErrTerminalSpecNotFound     = errors.New("terminal spec not found in context")
	ErrBuildTerminalSpec        = errors.New("failed to build terminal spec from flags")
	ErrNoTerminalIdentifier     = errors.New("no terminal identifier provided; terminal name or ID must be specified")
	ErrTooManyArguments         = errors.New("too many arguments; only one terminal name is allowed")
	ErrCreateSupervisorDir      = errors.New("failed to create supervisor directory")
	ErrResolveTerminalName      = errors.New("cannot resolve terminal name to ID")
	ErrNoTerminalIdentification = errors.New("no terminal identification method provided, cannot attach")
	ErrNoSupervisorIdentifier   = errors.New(
		"no supervisor identifier provided; supervisor name, ID, or socket must be specified",
	)
	ErrConflictingFlags           = errors.New("conflicting flags provided; only one flag option can be used")
	ErrBuildSocketPath            = errors.New("cannot build socket path")
	ErrDetermineRunPath           = errors.New("cannot determine run path")
	ErrNoSupervisorIdentification = errors.New("no supervisor identification method provided, cannot detach")
	ErrPositionalWithFlags        = errors.New("positional argument cannot be used with flags")
	ErrInvalidOutputFormat        = errors.New("invalid output format")
	ErrGetRunPath                 = errors.New("failed to get run path from env and flags")
	ErrNoTerminalsFound           = errors.New("no terminals found")
	ErrNoSupervisorsFound         = errors.New("no supervisors found")
	ErrTerminalNotFound           = errors.New("terminal not found")
	ErrSupervisorNotFound         = errors.New("supervisor not found")
)
