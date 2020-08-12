package message

import (
	"github.com/mongodb/grip/level"
)

type genericMessage struct {
	Base
	raw         Generic
	description string
}

type Generic interface {
	Send() error
	Valid() bool
}

// NewGenericMessage returns a composer for Generic messages
func NewGenericMessage(p level.Priority, g Generic, description string) Composer {
	m := &genericMessage{}
	if err := m.SetPriority(p); err != nil {
		_ = m.SetPriority(level.Notice)
	}

	m.raw = g
	m.description = description
	return m
}

func (m *genericMessage) Loggable() bool {
	return m.raw.Valid()
}

func (m *genericMessage) String() string {
	return m.description
}

func (m *genericMessage) Raw() interface{} {
	return m.raw
}
