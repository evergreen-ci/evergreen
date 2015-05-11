package util

import (
	"errors"
	"time"
)

var (
	// ErrTimedOut is returned by a function that timed out.
	ErrTimedOut = errors.New("Function timed out")
)

// RunFunctionWithTimeout runs a function, timing out after the specified time.
// The error returned will be the return value of the function if it completes,
// or ErrTimedOut if it times out.
func RunFunctionWithTimeout(f func() error, timeout time.Duration) error {

	// the error channel that the function's return value will be sent on
	errChan := make(chan error)

	// kick off the function
	go func() {
		errChan <- f()
	}()

	// wait, or timeout
	var errResult error
	select {
	case errResult = <-errChan:
		return errResult
	case <-time.After(timeout):
		return ErrTimedOut
	}

}
