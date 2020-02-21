package grip

import (
	"fmt"
	"time"

	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
)

type errorCauser interface {
	Cause() error
}

// ErrorTimeFinder unwraps a timestamp annotated error if possible and
// is capable of finding a timestamp in an error that has been
// annotated using pkg/errors.
func ErrorTimeFinder(err error) (time.Time, bool) {
	if err == nil {
		return time.Time{}, false
	}

	if tserr, ok := err.(*timestampError); ok {
		if tserr == nil {
			return time.Time{}, false
		}
		return tserr.time, true
	}

	for {
		if e, ok := err.(errorCauser); ok {
			if tserr, ok := e.(*timestampError); ok {
				if tserr == nil {
					break
				}
				return tserr.time, true
			}
			err = e.Cause()
			continue
		}
		break
	}

	return time.Time{}, false
}

type timestampError struct {
	err      error
	time     time.Time
	extended bool

	// message.Composer data
	ErrorValue string `bson:"error" json:"error" yaml:"error"`
	Extended   string `bson:"extended,omitempty" json:"extended,omitempty" yaml:"extended,omitempty"`
	Metadata   struct {
		Level   level.Priority         `bson:"level,omitempty" json:"level,omitempty" yaml:"level,omitempty"`
		Context map[string]interface{} `bson:"context,omitempty" json:"context,omitempty" yaml:"context,omitempty"`
	} `bson:"metadata,omitempty" json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

func newTimeStampError(err error) *timestampError {
	if err == nil {
		return nil
	}

	switch v := err.(type) {
	case *timestampError:
		return v
	default:
		return &timestampError{
			err:  err,
			time: time.Now(),
		}
	}
}

func (e *timestampError) setExtended(v bool) *timestampError { e.extended = v; return e }

// WrapErrorTime annotates an error with the timestamp. The underlying
// concrete object implements message.Composer as well as error.
func WrapErrorTime(err error) error {
	return newTimeStampError(err)
}

// WrapErrorTimeMessage annotates an error with the timestamp and a
// string form. The underlying concrete object implements
// message.Composer as well as error.
func WrapErrorTimeMessage(err error, m string) error {
	return newTimeStampError(errors.WithMessage(err, m))
}

// WrapErrorTimeMessagef annotates an error with a timestamp and a
// string formated message, like fmt.Sprintf or fmt.Errorf. The
// underlying concrete object implements  message.Composer as well as
// error.
func WrapErrorTimeMessagef(err error, m string, args ...interface{}) error {
	return newTimeStampError(errors.WithMessage(err, fmt.Sprintf(m, args...)))
}

func (e *timestampError) String() string {
	if e.err == nil {
		return ""
	}

	if e.extended {
		return fmt.Sprintf("%+v", e.err)
	}

	return e.err.Error()
}

func (e *timestampError) Cause() error { return e.err }

func (e *timestampError) Error() string {
	return fmt.Sprintf("[%s], %s", e.time.Format(time.RFC3339), e.String())
}

func (e *timestampError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = fmt.Fprintf(s, "[%s] %+v", e.time.Format(time.RFC3339), e.Cause())
		}
		fallthrough
	case 's':
		_, _ = fmt.Fprintf(s, "[%s] %s", e.time.Format(time.RFC3339), e.String())
	case 'q':
		_, _ = fmt.Fprintf(s, "[%s] %q", e.time.Format(time.RFC3339), e.String())
	}
}

func (e *timestampError) Loggable() bool { return e.err != nil }
func (e *timestampError) Raw() interface{} {
	if e.ErrorValue == "" {
		e.ErrorValue = e.String()

		extended := fmt.Sprintf("%+v", e.err)
		if extended != e.ErrorValue {
			e.Extended = extended
		}
	}

	return e
}

func (e *timestampError) Annotate(key string, v interface{}) error {
	if e.Metadata.Context == nil {
		e.Metadata.Context = map[string]interface{}{
			key: v,
		}
		return nil
	}

	if _, ok := e.Metadata.Context[key]; ok {
		return fmt.Errorf("key '%s' already exists", key)
	}

	e.Metadata.Context[key] = v

	return nil
}

func (e *timestampError) Priority() level.Priority {
	return e.Metadata.Level
}
func (e *timestampError) SetPriority(l level.Priority) error {
	if !l.IsValid() {
		return fmt.Errorf("%s (%d) is not a valid priority", l, l)
	}

	e.Metadata.Level = l
	return nil
}
