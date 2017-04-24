// Bytes Messages
//
// The bytes types make it possible to send a byte slice as a message.
package message

import "github.com/mongodb/grip/level"

type bytesMessage struct {
	data []byte
	Base
}

// NewBytesMessage provides a Composer interface around a byte slice,
// which are always logable unless the string is empty.
func NewBytesMessage(p level.Priority, b []byte) Composer {
	m := &bytesMessage{
		data: b,
	}

	_ = m.SetPriority(p)

	return m
}

// NewBytes provides a basic message consisting of a single line.
func NewBytes(b []byte) Composer {
	return &bytesMessage{data: b}
}

func (s *bytesMessage) String() string {
	return string(s.data)
}

func (s *bytesMessage) Loggable() bool {
	return len(s.data) > 0
}

func (s *bytesMessage) Raw() interface{} {
	_ = s.Collect()
	return struct {
		Metadata *Base  `bson:"metadata" json:"metadata" yaml:"metadata"`
		Message  string `bson:"message" json:"message" yaml:"message"`
	}{
		Metadata: &s.Base,
		Message:  string(s.data),
	}
}
