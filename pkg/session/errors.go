package session

import "errors"

var ErrFuncNotSet = errors.New("function not set")

var ErrSpecCmdMissing = errors.New("spec is missing Cmd")
var ErrOpenSocketCtrl = errors.New("could not open ctrl socket")
var ErrStartRPCServer = errors.New("error starting RPC server")
var ErrStartSession = errors.New("error starting session")
var ErrContextDone = errors.New("context has been cancelled")
var ErrRPCServerExited = errors.New("RPC Server exited with error")
var ErrOnClose = errors.New("error closing")
