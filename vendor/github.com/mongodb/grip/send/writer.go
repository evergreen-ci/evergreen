package send

import (
	"bufio"
	"bytes"
	"io"
	"sync"
	"unicode"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

// WriterSender wraps another sender and also provides an io.Writer.
// (and because Sender is an io.Closer) the type also implements
// io.WriteCloser.
type WriterSender struct {
	Sender
	writer   *bufio.Writer
	buffer   *bytes.Buffer
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
// called, this sender flushes the buffer.
func NewWriterSender(s Sender) *WriterSender { return MakeWriterSender(s, s.Level().Default) }

// MakeWriterSender returns an sender interface that also implements
// io.Writer. Specify a priority used as the level for messages sent.
func MakeWriterSender(s Sender, p level.Priority) *WriterSender {
	buffer := new(bytes.Buffer)

	return &WriterSender{
		Sender:   s,
		priority: p,
		writer:   bufio.NewWriter(buffer),
		buffer:   buffer,
	}
}

// Write captures a sequence of bytes to the send interface. It never errors.
func (s *WriterSender) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := s.writer.Write(p)
	if err != nil {
		return n, err
	}
	_ = s.writer.Flush()

	if s.buffer.Len() > 80 {
		err = s.doSend()
	}

	return n, err
}

func (s *WriterSender) doSend() error {
	for {
		line, err := s.buffer.ReadBytes('\n')
		if err == io.EOF {
			s.buffer.Write(line)
			return nil
		}

		lncp := make([]byte, len(line))
		copy(lncp, line)

		if err == nil {
			s.Send(message.NewBytesMessage(s.priority, bytes.TrimRightFunc(lncp, unicode.IsSpace)))
			continue
		}

		s.Send(message.NewBytesMessage(s.priority, bytes.TrimRightFunc(lncp, unicode.IsSpace)))
		return err
	}
}

// Close writes any buffered messages to the underlying Sender. Does
// not close the underlying sender.
func (s *WriterSender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.writer.Flush(); err != nil {
		return err
	}

	s.Send(message.NewBytesMessage(s.priority, bytes.TrimRightFunc(s.buffer.Bytes(), unicode.IsSpace)))
	s.buffer.Reset()
	s.writer.Reset(s.buffer)
	return nil
}
