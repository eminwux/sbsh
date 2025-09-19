package main

import "errors"

var ErrContextCancelled = errors.New("context has been cancelled")
var ErrWaitOnReady = errors.New("waiting for readiness has failed")
var ErrExit = errors.New("error on close")
var ErrGracefulExit = errors.New("closed gracefully")
var ErrWaitOnClose = errors.New("waiting for close has failed")
