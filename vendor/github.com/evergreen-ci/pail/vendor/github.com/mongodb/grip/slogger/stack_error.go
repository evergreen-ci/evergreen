package slogger

import (
	"fmt"
	"strings"

	"github.com/mongodb/grip/message"
)

// StackError is a grip re implementation of a type from legacy
// slogger. It combines a stacktrace of the call site of the logged
// message and a message composer. StackError also implements the
// error interface. The composer's "loggability" is derived from the
// embedded composer.
type StackError struct {
	message.Composer
	Stacktrace []string
	message    string
}

// NewStackError produces a StackError object, collecting the
// stacktrace and building a Formatted message composer
// (e.g. fmt.Sprintf).
func NewStackError(messageFmt string, args ...interface{}) *StackError {
	return &StackError{
		Composer:   message.NewFormatted(messageFmt, args...),
		Stacktrace: stacktrace(),
	}
}

// Raw produces a structure that contains the mesage, stacktrace, and
// raw metadata from the embedded composer for use by some logging
// backends.
func (s *StackError) Raw() interface{} {
	return &struct {
		Message    string      `bson:"message" json:"message" yaml:"message"`
		Stacktrace []string    `bson:"stacktrace" json:"stacktrace" yaml:"stacktrace"`
		Metadata   interface{} `bson:"metadata" json:"metadata" yaml:"metadata"`
	}{
		Message:    s.Composer.String(),
		Stacktrace: s.Stacktrace,
		Metadata:   s.Composer.Raw(),
	}
}

// Error returns the resolved error message for the StackError
// instance and satisfies the error interface.
func (s *StackError) Error() string { return s.String() }

// String lazily resolves a message for the instance and caches that
// message internally.
func (s *StackError) String() string {
	if s.message != "" {
		return s.message
	}

	s.message = fmt.Sprintf("%s\n\t%s",
		s.Composer.String(),
		strings.Join(s.Stacktrace, "\n\t"))

	return s.message
}
