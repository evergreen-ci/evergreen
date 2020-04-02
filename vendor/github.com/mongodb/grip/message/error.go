// Error Messages
//
// The error message composers underpin the Catch<> logging messages,
// which allow you to log error messages but let the logging system
// elide logging for nil errors.
package message

import (
	"fmt"
	"io"

	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
)

type errorMessage struct {
	err        error
	ErrorValue string `bson:"error" json:"error" yaml:"error"`
	Extended   string `bson:"extended,omitempty" json:"extended,omitempty" yaml:"extended,omitempty"`
	Base       `bson:"metadata" json:"metadata" yaml:"metadata"`
}

// NewErrorMessage takes an error object and returns a Composer
// instance that only renders a loggable message when the error is
// non-nil.
//
// These composers also implement the error interface and the
// pkg/errors.Causer interface and so can be passed as errors and used
// with existing error-wrapping mechanisms.
func NewErrorMessage(p level.Priority, err error) Composer {
	m := &errorMessage{
		err: err,
	}

	_ = m.SetPriority(p)
	return m
}

// NewError returns an error composer, like NewErrorMessage, but
// without the requirement to specify priority, which you may wish to
// specify directly.
func NewError(err error) Composer {
	return &errorMessage{err: err}
}

func (e *errorMessage) String() string {
	if e.err == nil {
		return ""
	}
	e.ErrorValue = e.err.Error()
	return e.ErrorValue
}

func (e *errorMessage) Loggable() bool {
	return e.err != nil
}

func (e *errorMessage) Raw() interface{} {
	_ = e.Collect()
	_ = e.String()

	extended := fmt.Sprintf("%+v", e.err)
	if extended != e.ErrorValue {
		e.Extended = extended
	}

	return e
}

func (e *errorMessage) Error() string { return e.String() }
func (e *errorMessage) Cause() error  { return e.err }
func (e *errorMessage) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			fmt.Fprintf(s, "%+v\n", errors.Cause(e.err))
			_, _ = io.WriteString(s, e.String())
			return
		}
		fallthrough
	case 's', 'q':
		_, _ = io.WriteString(s, e.Error())
	}
}
