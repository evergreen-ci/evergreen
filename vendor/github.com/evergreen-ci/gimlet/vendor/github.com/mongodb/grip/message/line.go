package message

import (
	"fmt"
	"strings"

	"github.com/mongodb/grip/level"
)

type lineMessenger struct {
	lines   []interface{}
	Base    `bson:"metadata" json:"metadata" yaml:"metadata"`
	Message string `bson:"message" json:"message" yaml:"message"`
}

// NewLineMessage is a basic constructor for a type that, given a
// bunch of arguments, calls fmt.Sprintln() on the arguments passed to
// the constructor during the String() operation. Use in combination
// with Compose[*] logging methods.
func NewLineMessage(p level.Priority, args ...interface{}) Composer {
	m := NewLine(args...)
	_ = m.SetPriority(p)
	return m
}

// NewLine returns a message Composer roughly equivalent to
// fmt.Sprintln().
func NewLine(args ...interface{}) Composer {
	m := &lineMessenger{}
	for _, arg := range args {
		if arg != nil {
			m.lines = append(m.lines, arg)
		}
	}

	return m
}

func newLinesFromStrings(p level.Priority, args []string) Composer {
	m := &lineMessenger{}
	_ = m.SetPriority(p)
	for _, arg := range args {
		if arg != "" {
			m.lines = append(m.lines, arg)
		}
	}

	return m
}

func (l *lineMessenger) Loggable() bool {
	if len(l.lines) <= 0 {
		return false
	}

	for idx := range l.lines {
		if l.lines[idx] != "" {
			return true
		}
	}

	return false
}

func (l *lineMessenger) String() string {
	if l.Message == "" {
		l.Message = strings.Trim(fmt.Sprintln(l.lines...), "\n ")
	}

	return l.Message
}

func (l *lineMessenger) Raw() interface{} {
	_ = l.Collect()
	_ = l.String()

	return l
}
