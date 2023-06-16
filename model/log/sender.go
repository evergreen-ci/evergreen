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

const (
	defaultMaxBufferSize int = 1e7
	defaultFlushInterval     = time.Minute
)

type sender struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	opts       LoggerOptions
	writeLog   func(context.Context, []LogLine) error
	buffer     []LogLine
	bufferSize int
	lastFlush  time.Time
	timer      *time.Timer
	closed     bool
	*send.Base
}

// LoggerOptions support the use and creation of an Evergreen log Sender.
type LoggerOptions struct {
	LogType LogType
	LogName string
	Parser  LineParser

	// Configure a local sender for "fallback" operations and to collect
	// the location of the logger output.
	Local send.Sender

	// The number max number of bytes to buffer before persisting log data.
	// Defaults to 10MB.
	MaxBufferSize int
	// The interval at which to flush log lines, regardless of whether the
	// max buffer size has been reached or not. Setting FlushInterval to a
	// duration less than 0 will disable timed flushes. Defaults to 1
	// minute.
	FlushInterval time.Duration
}

/*
// NewLogger returns a grip Sender backed by the Evergreen log service.
func NewLogger(name string, l send.LevelInfo, opts LoggerOptions) (send.Sender, error) {
	return NewLoggerWithContext(context.Background(), name, l, opts)
}

// NewLoggerWithContext returns a grip Sender backed by the Evergreen log
// service with level information set, using the passed in context.
func NewLoggerWithContext(ctx context.Context, name string, l send.LevelInfo, opts LoggerOptions) (send.Sender, error) {
	b, err := MakeLoggerWithContext(ctx, name, opts)
	if err != nil {
		return nil, errors.Wrap(err, "making new logger")
	}

	if err := b.SetLevel(l); err != nil {
		return nil, errors.Wrap(err, "setting grip level")
	}

	return b, nil
}

// MakeLogger returns a grip Sender backed by the Evergreen log service.
func MakeLogger(name string, opts LoggerOptions) (send.Sender, error) {
	return MakeLoggerWithContext(context.Background(), name, opts)
}

// MakeLoggerWithContext returns a grip Sender backed by the Evergreen log
// service using the passed in context.
func MakeLoggerWithContext(ctx context.Context, name string, taskOpts TaskOptions, loggerOpts LoggerOptions) (send.Sender, error) {
	if err := loggerOpts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid logger options")
	}

	service := &logServiceV0{bucket: opts.Bucket}
	ctx, cancel := context.WithCancel(ctx)
	s := &sender{
		ctx:       ctx,
		cancel:    cancel,
		opts:      opts,
		formatter: service.getFormatter(),
		keyPrefix: service,
		buffer:    new(bytes.Buffer),
		Base:      send.NewBase(name),
	}

	if err := s.SetErrorHandler(send.ErrorHandlerFromSender(s.opts.Local)); err != nil {
		return nil, errors.Wrap(err, "setting default error handler")
	}

	if opts.FlushInterval > 0 {
		go s.timedFlush()
	}

	return s, nil
}
*/

func MakeTaskLogger(ctx context.Context, name string, taskOpts TaskOptions, loggerOpts LoggerOptions) (send.Sender, error) {
	service := &logServiceV0{}
	writeLog := func(ctx context.Context, lines []LogLine) error {
		return service.WriteTaskLog(ctx, taskOpts, loggerOpts.LogType, loggerOpts.LogName, lines)
	}

	return makeLogger(ctx, name, loggerOpts, writeLog)
}

// makeLoggerWithContext returns a grip Sender backed by the Evergreen log
// service using the passed in context.
func makeLogger(ctx context.Context, name string, loggerOpts LoggerOptions, writeLog func(context.Context, []LogLine) error) (send.Sender, error) {
	ctx, cancel := context.WithCancel(ctx)
	s := &sender{
		ctx:      ctx,
		cancel:   cancel,
		opts:     loggerOpts,
		writeLog: writeLog,
		Base:     send.NewBase(name),
	}

	if err := s.SetErrorHandler(send.ErrorHandlerFromSender(s.opts.Local)); err != nil {
		return nil, errors.Wrap(err, "setting default error handler")
	}

	if opts.FlushInterval > 0 {
		go s.timedFlush()
	}

	return s, nil
}

// Send sends the given message with a timestamp created when the function is
// called to the cedar Buildlogger backend. This function buffers the messages
// until the maximum allowed buffer size is reached, at which point the
// messages in the buffer are sent to the Buildlogger server via RPC. Send is
// thread safe.
func (s *sender) Send(m message.Composer) {
	if !s.Level().ShouldLog(m) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ts := time.Now()
	if s.closed {
		s.opts.Local.Send(message.NewErrorMessage(level.Error, errors.New("cannot call Send on a closed Buildlogger Sender")))
		return
	}

	_, ok := m.(*message.GroupComposer)
	lines = strings.Split(m.String(), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		logLine, err := s.opts.Parser(line)
		if err != nil {
			s.opts.Local.Send(message.NewErrorMessage(level.Error, errors.Wrap(err, "parsing log line")))
		}

		s.buffer = append(s.buffer, logLine)
		s.bufferSize += len(line)
		if s.buffer.Len() > s.opts.MaxBufferSize {
			if err := s.flush(s.ctx); err != nil {
				s.opts.Local.Send(message.NewErrorMessage(level.Error, err))
				return
			}
		}
	}
}

// Flush flushes anything messages that may be in the buffer to the pail-backed
// bucket storage.
func (s *sender) Flush(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	return s.flush(ctx)
}

// Close flushes anything that may be left in the underlying buffer and closes
// out the log with a completed at timestamp and the exit code. If the gRPC
// client connection was created in NewLogger or MakeLogger, this connection is
// also closed. Close is thread safe but should only be called once no more
// calls to Send are needed; after Close has been called any subsequent calls
// to Send will error. After the first call to Close subsequent calls will
// no-op.
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
			s.opts.Local.Send(message.NewErrorMessage(level.Error, err))
			catcher.Add(errors.Wrap(err, "flushing buffer"))
		}
	}

	return catcher.Resolve()
}

func (s *sender) timedFlush() {
	s.mu.Lock()
	s.timer = time.NewTimer(b.opts.FlushInterval)
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
	s.writeLog(ctx, s.buffer)
	if err != nil {
		return errors.Wrap(err, "writing log")
	}

	s.buffer = []LogLine{}
	s.lastFlush = time.Now()

	return nil
}
