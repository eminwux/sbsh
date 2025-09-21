package sessionrunner

import "errors"

var ErrCloseReq error = errors.New("received close order")
var ErrPipeWrite error = errors.New("could not write pty->pipe")
var ErrPipeRead error = errors.New("could not read pipe->pipe")

var ErrTerminalRead error = errors.New("could not read pipe->pty")
var ErrTerminalWrite error = errors.New("could not write pty->pipe")
