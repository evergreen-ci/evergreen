package pail

import (
	"fmt"

	"github.com/pkg/errors"
)

type keyNotFoundError struct {
	msg string
}

func (e *keyNotFoundError) Error() string { return e.msg }

// NewKeyNotFoundError creates a new error object to represent a key not found
// error.
func NewKeyNotFoundError(msg string) error { return &keyNotFoundError{msg: msg} }

// NewKeyNotFoundErrorf creates a new error object to represent a key not found
// error with a formatted message.
func NewKeyNotFoundErrorf(msg string, args ...interface{}) error {
	return NewKeyNotFoundError(fmt.Sprintf(msg, args...))
}

// MakeKeyNotFoundError constructs a key not found error from an existing error
// of any type.
func MakeKeyNotFoundError(err error) error {
	if err == nil {
		return nil
	}

	return NewKeyNotFoundError(err.Error())
}

// IsKeyNotFoundError checks an error object to see if it is a key not found
// error.
func IsKeyNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	_, ok := errors.Cause(err).(*keyNotFoundError)
	return ok
}
