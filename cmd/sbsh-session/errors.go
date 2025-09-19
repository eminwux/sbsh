package main

import "errors"

var ErrContextDone = errors.New("context has been cancelled")
var ErrWaitOnReady = errors.New("waiting for readiness has failed")
var ErrWaitOnClose = errors.New("waiting for close has failed")
