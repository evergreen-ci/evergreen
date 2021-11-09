package systemmetrics

import (
	"context"
	"sync"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// WriteCloserOptions allow maximum buffer size and the maximum time between
// flushes to be specified. If NoTimedFlush is true, then the timed flush will
// not occur.
type WriteCloserOptions struct {
	FlushInterval time.Duration
	NoTimedFlush  bool
	MaxBufferSize int
}

func (s *WriteCloserOptions) validate() error {
	if s.FlushInterval < 0 {
		return errors.New("flush interval must not be negative")
	}
	if s.FlushInterval == 0 {
		s.FlushInterval = defaultFlushInterval
	}

	if s.MaxBufferSize < 0 {
		return errors.New("buffer size must not be negative")
	}
	if s.MaxBufferSize == 0 {
		s.MaxBufferSize = defaultMaxBufferSize
	}

	return nil
}

// systemMetricsWriteCloser is a wrapper around a SystemMetricsClient that
// implements buffering with timed flushes and the WriteCloser interface.
type systemMetricsWriteCloser struct {
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	catcher       grip.Catcher
	client        *SystemMetricsClient
	opts          MetricDataOptions
	buffer        []byte
	maxBufferSize int
	lastFlush     time.Time
	timer         *time.Timer
	flushInterval time.Duration
	closed        bool
}

// Write adds provided data to the buffer, flushing it if it exceeds its max
// size. Write will fail if the writer was previously closed or if a timed
// flush encountered an error.
func (s *systemMetricsWriteCloser) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.catcher.HasErrors() {
		return 0, errors.Wrapf(s.catcher.Resolve(), "writer already closed due to error")
	}
	if s.closed {
		return 0, errors.New("writer already closed")
	}
	if len(data) == 0 {
		return 0, errors.New("must provide data to write")
	}

	s.buffer = append(s.buffer, data...)
	if len(s.buffer) > s.maxBufferSize {
		if err := s.flush(); err != nil {
			s.catcher.Add(err)
			s.close()
			return 0, errors.Wrapf(s.catcher.Resolve(), "problem writing data")
		}
	}
	return len(data), nil
}

// Close flushes any remaining data in the buffer. If the write closer was
// previously closed, this returns an error.
func (s *systemMetricsWriteCloser) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.catcher.HasErrors() {
		return errors.Wrapf(s.catcher.Resolve(), "writer already closed due to error")
	}
	if s.closed {
		return errors.New("writer already closed")
	}

	s.catcher.Add(s.flush())
	s.close()
	return s.catcher.Resolve()
}

func (s *systemMetricsWriteCloser) flush() error {
	if len(s.buffer) > 0 {
		if err := s.client.AddSystemMetrics(s.ctx, s.opts, s.buffer); err != nil {
			return errors.Wrapf(err, "problem sending data for id %s", s.opts.Id)
		}
		s.buffer = []byte{}
	}

	s.lastFlush = time.Now()
	return nil
}

func (s *systemMetricsWriteCloser) close() {
	s.cancel()
	s.closed = true
}

func (s *systemMetricsWriteCloser) timedFlush() {
	defer func() {
		message := "panic in systemMetrics timedFlush"
		err := recovery.HandlePanicWithError(recover(), nil, message)
		if err != nil {
			grip.Error(message)
			s.catcher.Add(err)
		}
	}()
	s.mu.Lock()
	s.timer = time.NewTimer(s.flushInterval)
	s.mu.Unlock()
	defer s.timer.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.timer.C:
			func() {
				s.mu.Lock()
				defer s.mu.Unlock()
				if len(s.buffer) > 0 && time.Since(s.lastFlush) >= s.flushInterval {
					if err := s.flush(); err != nil {
						s.catcher.Add(errors.Wrapf(err, "problem with timed flush for id %s", s.opts.Id))
						s.close()
					}
				}
				_ = s.timer.Reset(s.flushInterval)
			}()
		}
	}
}
