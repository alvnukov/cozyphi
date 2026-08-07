package job

import "errors"

var (
	ErrInvalid     = errors.New("job: invalid request")
	ErrNotFound    = errors.New("job: not found")
	ErrDepth       = errors.New("job: nesting depth exceeded")
	ErrBusy        = errors.New("job: concurrency limit reached")
	ErrClosed      = errors.New("job: manager closed")
	ErrNotRunning  = errors.New("job: not running")
	ErrWaitTimeout = errors.New("job: wait timed out")
)
