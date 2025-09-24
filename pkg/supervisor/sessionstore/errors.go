package sessionstore

import "errors"

var ErrFuncNotSet = errors.New("function not set")

var ErrSessionExists error = errors.New("session id already exists in store")
