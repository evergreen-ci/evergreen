package message

import (
	"fmt"
	"strings"

	"github.com/tychoish/grip/level"
)

type errorWrapMessage struct {
	base     string
	args     []interface{}
	err      error
	Message  string `bson:"message,omitempty" json:"message,omitempty" yaml:"message,omitempty"`
	Extended string `bson:"extended,omitempty" json:"extended,omitempty" yaml:"extended,omitempty"`
	Base     `bson:"metadata" json:"metadata" yaml:"metadata"`
}

// NewErrorWrapMessage produces a fully configured message.Composer
// that combines the functionality of an Error composer that renders a
// loggable error message for non-nil errors with a normal formatted
// message (e.g. fmt.Sprintf). These messages only log if the error is
// non-nil.
func NewErrorWrapMessage(p level.Priority, err error, base string, args ...interface{}) Composer {
	m := &errorWrapMessage{
		base: base,
		args: args,
		err:  err,
	}

	_ = m.SetPriority(p)

	return m
}

// NewErrorWrapDefault produces a message.Composer that combines the
// functionality of an Error composer that renders a loggable error
// message for non-nil errors with a normal formatted message
// (e.g. fmt.Sprintf). These messages only log if the error is
// non-nil.
func NewErrorWrap(err error, base string, args ...interface{}) Composer {
	return &errorWrapMessage{
		base: base,
		args: args,
		err:  err,
	}
}

func (m *errorWrapMessage) String() string {
	if m.Message != "" {
		return m.Message
	}

	lines := []string{}

	if m.base != "" {
		lines = append(lines, fmt.Sprintf(m.base, m.args...))
	}

	if m.err != nil {
		lines = append(lines, fmt.Sprintf("%+v", m.err))
	}

	if len(lines) > 0 {
		m.Message = strings.Join(lines, "\n")
	} else {
		m.Message = "<none>"

	}

	return m.Message
}
func (m *errorWrapMessage) Raw() interface{} {
	_ = m.String()
	_ = m.Collect()

	return m
}

func (m *errorWrapMessage) Loggable() bool {
	return m.err != nil
}
