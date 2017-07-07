package send

import (
	"bytes"
	"sync"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

var newLine = []byte{'\n'}

// WriterSender wraps another sender and also provides an io.Writer.
// (and because Sender is an io.Closer) the type also implements
// io.WriteCloser.
type WriterSender struct {
	Sender
	buffer   []byte
	priority level.Priority
	mu       sync.Mutex
}

// NewWriterSender wraps another sender and also provides an io.Writer.
// (and because Sender is an io.Closer) the type also implements
// io.WriteCloser.
//
// While WriteSender itself implements Sender, it also provides a
// Writer method, which allows you to use this Sender to capture
// file-like write operations.
//
// Data sent via the Write method is buffered internally until its
// passed a byte slice that ends with the new line character. If the
// string form of the bytes passed to the write method (including all
// buffered messages) is only whitespace, then it is not sent.
//
// If there are any bytes in the buffer when the Close method is
// called, this sender flushes the buffer before closing the underlying sender.
func NewWriterSender(s Sender) *WriterSender {
	return &WriterSender{Sender: s, priority: s.Level().Default}
}

// MakeWriterSender returns an sender interface that also implements
// io.Writer. Specify a priority used as the level for messages sent.
func MakeWriterSender(s Sender, p level.Priority) *WriterSender {
	return &WriterSender{Sender: s, priority: p}
}

// Write captures a sequence of bytes to the send interface. It never errors.
func (s *WriterSender) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// capture the number of bytes in the input so we can return
	// the proper amount
	n := len(p)

	// if the logged message does *not* end in a new line, we
	// should buffer it until we find one that does.
	if !bytes.HasSuffix(p, newLine) {
		s.buffer = append(s.buffer, p...)
		return n, nil
	}
	// we're ready to write the log message

	// if we had something in the buffer, we should prepend it to
	// the current message, and clear the buffer.
	if len(s.buffer) >= 1 {
		p = append(s.buffer, p...)
		s.buffer = []byte{}
	}

	// Now send each log message:
	s.doSend(s.priority, p)
	return n, nil
}

func (s *WriterSender) doSend(l level.Priority, buf []byte) {
	for _, val := range bytes.Split(buf, newLine) {
		if len(bytes.Trim(val, " \t")) != 0 {
			s.Send(message.NewBytesMessage(l, val))
		}
	}
}

// Close writes any buffered messages to the underlying Sender before
// closing that Sender.
func (s *WriterSender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.buffer) >= 1 {
		s.doSend(s.Level().Default, s.buffer)
		s.buffer = []byte{}
	}

	return s.Sender.Close()
}
