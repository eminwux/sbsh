package supervisorrunner

import "errors"

var ErrContextDone = errors.New("context has been cancelled")
var ErrSessionCmdStart = errors.New("error in shell cmd")
