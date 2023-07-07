package log

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

const defaultMaxBufferSize = 1e7 //nolint: unused

// LineParser functions parse a raw log line into the service representation of
// a log line for uniform ingestion of logs by the Evergreen log sender.
// Parsers need not set the log name or, in most cases, the priority.
type LineParser func(string) (LogLine, error)

// LoggerOptions support the use and creation of an Evergreen log sender.
type LoggerOptions struct {
	// LogName is the identifying name of the log to use when persisting
	// data.
	LogName string
	// Parse is the function for parsing raw log lines collected by the
	// sender.
	Parse LineParser

	// Local is the sender for "fallback" operations and to collect the
	// location of the logger output.
	Local send.Sender

	// MaxBufferSize is the maximum number of bytes to buffer before
	// persisting log data. Defaults to 10MB.
	MaxBufferSize int
	// FlushInterval is time interval at which to flush log lines,
	// regardless of whether the max buffer size has been reached. A flush
	// interval less than or equal to 0 will disable timed flushes.
	FlushInterval time.Duration
}

// sender implements the send.Sender interface for persisting Evergreen logs.
type sender struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	opts       LoggerOptions
	write      logWriter
	buffer     []LogLine
	bufferSize int
	lastFlush  time.Time
	timer      *time.Timer
	closed     bool
	*send.Base
}

type logWriter func(context.Context, []LogLine) error

// makeLogger returns a sender backed by the Evergreen log service.
func makeLogger(ctx context.Context, name string, opts LoggerOptions, write logWriter) (send.Sender, error) { //nolint: unused
	ctx, cancel := context.WithCancel(ctx)
	s := &sender{
		ctx:    ctx,
		cancel: cancel,
		opts:   opts,
		write:  write,
		Base:   send.NewBase(name),
	}

	if err := s.SetErrorHandler(send.ErrorHandlerFromSender(s.opts.Local)); err != nil {
		return nil, errors.Wrap(err, "setting default error handler")
	}

	if opts.MaxBufferSize <= 0 {
		opts.MaxBufferSize = defaultMaxBufferSize
	}

	if opts.FlushInterval > 0 {
		go s.timedFlush()
	}

	return s, nil
}

// Send sends the given message to the Evergreen log service. This function
// buffers the messages until the maximum allowed buffer size is reached, at
// which point the messages in the buffer are written to persistent storage by
// the backing log service. Send is thread safe.
func (s *sender) Send(m message.Composer) {
	if !s.Level().ShouldLog(m) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		s.opts.Local.Send(message.NewErrorMessage(level.Error, errors.New("cannot call Send on a closed sender")))
		return
	}

	for _, line := range strings.Split(m.String(), "\n") {
		if line == "" {
			continue
		}

		logLine, err := s.opts.Parse(line)
		if err != nil {
			s.opts.Local.Send(message.NewErrorMessage(level.Error, errors.Wrap(err, "parsing log line")))
			return
		}
		if logLine.Priority == 0 {
			logLine.Priority = m.Priority()
		}
		if !logLine.Priority.IsValid() {
			s.opts.Local.Send(message.NewErrorMessage(level.Error, errors.Errorf("invalid log line priority %d", logLine.Priority)))
			return
		}

		s.buffer = append(s.buffer, logLine)
		s.bufferSize += len(line)
		if s.bufferSize > s.opts.MaxBufferSize {
			if err := s.flush(s.ctx); err != nil {
				s.opts.Local.Send(message.NewErrorMessage(level.Error, err))
				return
			}
		}
	}
}

// Flush flushes anything messages that may be in the buffer to persistent
// storage determined by the backing log service.
func (s *sender) Flush(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	return s.flush(ctx)
}

// Close flushes anything that may be left in the underlying buffer and
// terminates all background operations of the sender. Close is thread safe but
// should only be called once no more calls to Send are needed; after Close has
// been called any subsequent calls to Send will error while subsequent calls
// to Close will no-op.
func (s *sender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.cancel()

	if s.closed {
		return nil
	}
	s.closed = true

	if len(s.buffer) > 0 {
		if err := s.flush(s.ctx); err != nil {
			return errors.Wrap(err, "flushing buffer")
		}
	}

	return nil
}

func (s *sender) timedFlush() {
	s.mu.Lock()
	s.timer = time.NewTimer(s.opts.FlushInterval)
	s.mu.Unlock()
	defer s.timer.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.timer.C:
			s.mu.Lock()
			if len(s.buffer) > 0 && time.Since(s.lastFlush) >= s.opts.FlushInterval {
				if err := s.flush(s.ctx); err != nil {
					s.opts.Local.Send(message.NewErrorMessage(level.Error, err))
				}
			}
			_ = s.timer.Reset(s.opts.FlushInterval)
			s.mu.Unlock()
		}
	}
}

func (s *sender) flush(ctx context.Context) error {
	if err := s.write(ctx, s.buffer); err != nil {
		return errors.Wrap(err, "writing log")
	}

	s.buffer = []LogLine{}
	s.bufferSize = 0
	s.lastFlush = time.Now()

	return nil
}
