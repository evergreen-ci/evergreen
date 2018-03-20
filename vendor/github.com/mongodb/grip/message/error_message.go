package message

import (
	"fmt"

	"github.com/mongodb/grip/level"
)

type errorComposerWrap struct {
	err    error
	cached string
	Composer
}

// NewErrorWrappedComposer provvides a way to construct a log message
// that annotates an error.
func NewErrorWrappedComposer(err error, m Composer) Composer {
	return &errorComposerWrap{
		err:      err,
		Composer: m,
	}
}

// NewErrorWrapMessage produces a fully configured message.Composer
// that combines the functionality of an Error composer that renders a
// loggable error message for non-nil errors with a normal formatted
// message (e.g. fmt.Sprintf). These messages only log if the error is
// non-nil.
func NewErrorWrapMessage(p level.Priority, err error, base string, args ...interface{}) Composer {
	return NewErrorWrappedComposer(err, NewFormattedMessage(p, base, args...))
}

// NewErrorWrap produces a message.Composer that combines the
// functionality of an Error composer that renders a loggable error
// message for non-nil errors with a normal formatted message
// (e.g. fmt.Sprintf). These messages only log if the error is
// non-nil.
func NewErrorWrap(err error, base string, args ...interface{}) Composer {
	return NewErrorWrappedComposer(err, NewFormatted(base, args...))
}

// WrapError wraps an error and creates a composer converting the
// argument into a composer in the same manner as the front end logging methods.
func WrapError(err error, m interface{}) Composer {
	return NewErrorWrappedComposer(err, ConvertToComposer(level.Priority(0), m))
}

// WrapErrorf wraps an error and creates a composer using a
// Sprintf-style formated composer.
func WrapErrorf(err error, msg string, args ...interface{}) Composer {
	return NewErrorWrappedComposer(err, NewFormatted(msg, args...))
}

func (m *errorComposerWrap) String() string {
	if m.cached == "" {
		context := m.Composer.String()
		if context != "" {
			m.cached = fmt.Sprintf("%s: %v", context, m.err.Error())
		} else {
			m.cached = m.err.Error()
		}
	}

	return m.cached
}

func (m *errorComposerWrap) Loggable() bool {
	return m.err != nil
}

func (m *errorComposerWrap) Raw() interface{} {
	errStr := m.err.Error()
	out := Fields{
		"error": errStr,
	}

	if m.Composer.Loggable() {
		// special handling for fields - merge keys in with output keys
		switch t := m.Composer.(type) {
		case *fieldMessage:
			t.fields["error"] = errStr
			out = t.fields
		default:
			out["context"] = m.Composer.Raw()
		}
	}

	ext := fmt.Sprintf("%+v", m.err)
	if ext != errStr {
		out["extended"] = ext
	}

	return out
}

func (m *errorComposerWrap) Annotate(k string, v interface{}) error {
	return m.Composer.Annotate(k, v)
}
