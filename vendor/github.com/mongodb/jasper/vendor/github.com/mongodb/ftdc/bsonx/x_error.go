package bsonx

import (
	"github.com/pkg/errors"
)

var errTooSmall = errors.New("error: too small")

func IsTooSmall(err error) bool { return errors.Cause(err) == errTooSmall }

func NewErrTooSmall() error { return errors.WithStack(errTooSmall) }
