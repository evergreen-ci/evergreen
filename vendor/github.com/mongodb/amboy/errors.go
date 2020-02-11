package amboy

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
)

// EnqueueUniqueJob is a generic wrapper for adding jobs to queues
// (using the Put() method), but that ignores duplicate job errors.
func EnqueueUniqueJob(ctx context.Context, queue Queue, job Job) error {
	err := queue.Put(ctx, job)

	if IsDuplicateJobError(err) {
		return nil
	}

	return errors.WithStack(err)
}

type duplJobError struct {
	msg string
}

func (e *duplJobError) Error() string { return e.msg }

// NewDuplicateJobError creates a new error object to represent a
// duplicate job error, for use by queue implementations.
func NewDuplicateJobError(msg string) error { return &duplJobError{msg: msg} }

// NewDuplicateJobErrorf creates a new error object to represent a
// duplicate job error with a formatted message, for use by queue
// implementations.
func NewDuplicateJobErrorf(msg string, args ...interface{}) error {
	return NewDuplicateJobError(fmt.Sprintf(msg, args...))
}

// MakeDuplicateJobError constructs a duplicate job error from an
// existing error of any type, for use by queue implementations.
func MakeDuplicateJobError(err error) error {
	if err == nil {
		return nil
	}

	return NewDuplicateJobError(err.Error())
}

// IsDuplicateJobError tests an error object to see if it is a
// duplicate job error.
func IsDuplicateJobError(err error) bool {
	if err == nil {
		return false
	}

	_, ok := errors.Cause(err).(*duplJobError)
	return ok
}
