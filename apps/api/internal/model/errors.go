package model

import "errors"


var ErrSandboxNotFound = errors.New("sandbox not found")
var ErrSandboxAlreadyRunning = errors.New("sandbox is already running")
var ErrSandboxNotStopped = errors.New("sandbox is not stopped")
var ErrSandboxNotRunning = errors.New("sandbox is not running")
var ErrExecFailed = errors.New("exec command failed")
var ErrUnauthorized = errors.New("unauthorized")
