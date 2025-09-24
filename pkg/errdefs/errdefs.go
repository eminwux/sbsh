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
	ErrRPCServerExited = errors.New("RPC Server exited with error")
	ErrOnClose         = errors.New("error closing")
	ErrCloseReq        = errors.New("close requested")
	ErrSessionStore    = errors.New("error in session store")
	ErrSessionCmdStart = errors.New("error in shell cmd")
	ErrSessionExists   = errors.New("session id already exists in store")
	ErrWriteMetadata   = errors.New("could not write metadata file")
)
