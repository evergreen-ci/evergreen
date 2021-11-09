package birch

import (
	"github.com/pkg/errors"
)

var errTooSmall = errors.New("error: too small")

func newErrTooSmall() error { return errors.WithStack(errTooSmall) }
