package message

import "github.com/mongodb/grip/level"

type stringMessage struct {
	Message string `bson:"message" json:"message" yaml:"message"`
	Base    `bson:"metadata" json:"metadata" yaml:"metadata"`

	skipMetadata bool
}

// NewDefaultMessage provides a Composer interface around a single
// string, which are always logable unless the string is empty.
func NewDefaultMessage(p level.Priority, message string) Composer {
	m := &stringMessage{Message: message}
	_ = m.SetPriority(p)
	return m
}

// NewString provides a basic message consisting of a single line.
func NewString(m string) Composer {
	return &stringMessage{Message: m}
}

// NewSimpleString produces a string message that does not attach
// process metadata.
func NewSimpleString(m string) Composer {
	return &stringMessage{Message: m, skipMetadata: true}
}

// NewSimpleStringMessage produces a string message with a priority
// that does not attach process metadata.
func NewSimpleStringMessage(p level.Priority, message string) Composer {
	m := &stringMessage{Message: message, skipMetadata: true}
	_ = m.SetPriority(p)
	return m
}

func (s *stringMessage) String() string {
	return s.Message
}

func (s *stringMessage) Loggable() bool {
	return s.Message != ""
}

func (s *stringMessage) Raw() interface{} {
	if !s.skipMetadata {
		_ = s.Collect()
	}
	return s
}
