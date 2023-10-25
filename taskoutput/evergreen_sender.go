package taskoutput

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

const defaultMaxBufferSize = 1e7

// logLineParser functions parse a raw log line into the service representation
// of a log line for uniform ingestion of logs by the Evergreen log sender.
// Parsers need not set the log name or, in most cases, the priority.
type logLineParser func(string) (log.LogLine, error)

// logLineAppender appends a chunk of lines to the underlying log store.
type logLineAppender func(context.Context, []log.LogLine) error

// EvergreenSenderOptions support the use and creation of an Evergreen sender.
type EvergreenSenderOptions struct {
	// Local is the sender for "fallback" operations and to collect any
	// logger error output.
	Local send.Sender
	// MaxBufferSize is the maximum number of bytes to buffer before
	// persisting log data. Defaults to 10MB.
	MaxBufferSize int
	// FlushInterval is time interval at which to flush log lines,
	// regardless of whether the max buffer size has been reached. A flush
	// interval equal to 0 will disable timed flushes.
	FlushInterval time.Duration

	// the injectable line parser allows the sender to be agnostic to the
	// raw log line formats it ingests. Defaults to a basic line parser
	// that adds the raw string as the log line data field.
	parse       logLineParser
	appendLines logLineAppender
}

func (opts *EvergreenSenderOptions) validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(opts.appendLines == nil, "must provide an appender function")
	catcher.NewWhen(opts.MaxBufferSize < 0, "max buffer size cannot be negative")
	catcher.NewWhen(opts.FlushInterval < 0, "flush interval cannot be negative")

	if opts.parse == nil {
		opts.parse = func(rawLine string) (log.LogLine, error) {
			return log.LogLine{Data: rawLine}, nil
		}
	}

	if opts.Local == nil {
		opts.Local = send.MakeNative()
		opts.Local.SetName("local")
	}

	if opts.MaxBufferSize == 0 {
		opts.MaxBufferSize = defaultMaxBufferSize
	}

	return catcher.Resolve()
}

// evergreenSender implements the send.Sender interface for persisting
// Evergreen logs.
type evergreenSender struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	opts       EvergreenSenderOptions
	buffer     []log.LogLine
	bufferSize int
	lastFlush  time.Time
	closed     bool
	*send.Base
}

// newEvergreeSender creates a new sender for Evergreen logs.
func newEvergreenSender(ctx context.Context, name string, opts EvergreenSenderOptions) (*evergreenSender, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	s := &evergreenSender{
		ctx:    ctx,
		cancel: cancel,
		opts:   opts,
		Base:   send.NewBase(name),
	}

	if err := s.SetErrorHandler(send.ErrorHandlerFromSender(s.opts.Local)); err != nil {
		return nil, errors.Wrap(err, "setting default error handler")
	}

	if opts.FlushInterval > 0 {
		go s.timedFlush()
	}

	return s, nil
}

// Send sends the given message to the backing log service. This function
// buffers the messages until the maximum allowed buffer size is reached, at
// which point the messages in the buffer are written to persistent storage by
// the backing log service. Send is thread safe.
func (s *evergreenSender) Send(m message.Composer) {
	ts := time.Now().UnixNano()

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

		logLine, err := s.opts.parse(line)
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
		if logLine.Timestamp == 0 {
			logLine.Timestamp = ts
		}
		if logLine.Timestamp < 0 {
			s.opts.Local.Send(message.NewErrorMessage(level.Error, errors.Errorf("invalid log line timestamp %d", logLine.Timestamp)))
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
func (s *evergreenSender) Flush(ctx context.Context) error {
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
func (s *evergreenSender) Close() error {
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

func (s *evergreenSender) timedFlush() {
	ticker := time.NewTicker(s.opts.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			if len(s.buffer) > 0 && time.Since(s.lastFlush) >= s.opts.FlushInterval {
				if err := s.flush(s.ctx); err != nil {
					s.opts.Local.Send(message.NewErrorMessage(level.Error, err))
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *evergreenSender) flush(ctx context.Context) error {
	if err := s.opts.appendLines(ctx, s.buffer); err != nil {
		return errors.Wrap(err, "appending lines to log")
	}

	s.buffer = []log.LogLine{}
	s.bufferSize = 0
	s.lastFlush = time.Now()

	return nil
}
