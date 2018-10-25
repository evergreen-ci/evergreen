package send

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/mongodb/grip/message"
)

// InMemorySender represents an in-memory buffered sender with a fixed message capacity.
type InMemorySender struct {
	Base
	buffer         []message.Composer
	mutex          sync.RWMutex
	head           int
	totalBytesSent int64
}

// NewInMemorySender creates an in-memory buffered sender with the given capacity.
func NewInMemorySender(name string, info LevelInfo, capacity int) (Sender, error) {
	if capacity <= 0 {
		return nil, errors.New("cannot have capacity <= 0")
	}

	s := &InMemorySender{Base: *NewBase(name), buffer: make([]message.Composer, 0, capacity)}
	if err := s.Base.SetLevel(info); err != nil {
		return nil, err
	}

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	if err := s.SetErrorHandler(ErrorHandlerFromLogger(fallback)); err != nil {
		return nil, err
	}

	if err := s.SetFormatter(MakeDefaultFormatter()); err != nil {
		return nil, err
	}

	s.reset = func() {
		fallback.SetPrefix(fmt.Sprintf("[%s] ", s.Name()))
	}

	return s, nil
}

// Get returns the messages in the buffer.
func (s *InMemorySender) Get() []message.Composer {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var start int
	var tmp []message.Composer

	start = s.head - len(s.buffer)
	if start < 0 {
		start = start + len(s.buffer)
		tmp = append(tmp, s.buffer[start:len(s.buffer)]...)
		return append(tmp, s.buffer[:s.head]...)
	}
	return append(tmp, s.buffer[start:s.head]...)
}

// GetString returns the messages in the buffer as formatted strings.
func (s *InMemorySender) GetString() ([]string, error) {
	msgs := s.Get()
	strs := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		str, err := s.Formatter(msg)
		if err != nil {
			return nil, err
		}
		strs = append(strs, str)
	}
	return strs, nil
}

// GetRaw returns the messages in the buffer as empty interfaces.
func (s *InMemorySender) GetRaw() []interface{} {
	msgs := s.Get()
	raw := make([]interface{}, 0, len(msgs))
	for _, msg := range msgs {
		raw = append(raw, msg.Raw())
	}
	return raw
}

// Send adds the given message to the buffer. If the buffer is at max capacity, it removes the oldest
// message.
func (s *InMemorySender) Send(msg message.Composer) {
	if !s.Level().ShouldLog(msg) {
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if len(s.buffer) < cap(s.buffer) {
		s.buffer = append(s.buffer, msg)
	} else {
		s.buffer[s.head] = msg
	}
	s.head = (s.head + 1) % cap(s.buffer)

	s.totalBytesSent += int64(len(msg.String()))
}

// TotalBytesSent returns the total number of bytes sent.
func (s *InMemorySender) TotalBytesSent() int64 {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.totalBytesSent
}
