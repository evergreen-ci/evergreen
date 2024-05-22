package client

import (
	"context"
	"sync"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// Standard/Default Production LoggerProducer

// logHarness provides a straightforward implementation of the
// plugin.LoggerProducer interface.
type logHarness struct {
	execution grip.Journaler
	task      grip.Journaler
	system    grip.Journaler
	mu        sync.RWMutex
	closed    bool
}

func (l *logHarness) Execution() grip.Journaler { return l.execution }
func (l *logHarness) Task() grip.Journaler      { return l.task }
func (l *logHarness) System() grip.Journaler    { return l.system }

// Flush flushes the current buffered task logs. This is safe to call multiple
// times. Once the task logger is closed, Flush will no-op, so it's important
// to ensure that Close is only called when the task is completely done logging.
func (l *logHarness) Flush(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(l.execution.GetSender().Flush(ctx))
	catcher.Add(l.task.GetSender().Flush(ctx))
	catcher.Add(l.system.GetSender().Flush(ctx))

	return catcher.Resolve()
}

// Close closes all the task loggers and prevents further writes to it. Note
// that this should not be called until the task is completely finished and will
// not log any more; otherwise, any further logs may be lost. To flush the
// current buffered task logs, call Flush instead.
func (l *logHarness) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true

	catcher := grip.NewBasicCatcher()

	catcher.Add(l.execution.GetSender().Close())
	catcher.Add(l.task.GetSender().Close())
	catcher.Add(l.system.GetSender().Close())

	return errors.Wrap(catcher.Resolve(), "closing log harness")
}

func (l *logHarness) Closed() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.closed
}

////////////////////////////////////////////////////////////////////////
//
// Single Channel LoggerProducer

type singleChannelLogHarness struct {
	logger grip.Journaler
	mu     sync.RWMutex
	closed bool
}

// NewSingleChannelLogHarnness returns a log implementation that uses
// a LoggerProducer where Execution, Task, and System systems all use
// the same sender. The Local channel still wraps the default global
// sender.
//
// This implementation is primarily for testing and should be used
// with the InternalSender, which permits introspection of log messages.
func NewSingleChannelLogHarness(name string, sender send.Sender) LoggerProducer {
	sender.SetName(name)

	l := &singleChannelLogHarness{
		logger: logging.MakeGrip(sender),
	}

	return l
}

func (l *singleChannelLogHarness) Execution() grip.Journaler { return l.logger }
func (l *singleChannelLogHarness) Task() grip.Journaler      { return l.logger }
func (l *singleChannelLogHarness) System() grip.Journaler    { return l.logger }

func (l *singleChannelLogHarness) Flush(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}

	return l.logger.GetSender().Flush(ctx)
}

func (l *singleChannelLogHarness) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true

	catcher := grip.NewBasicCatcher()

	catcher.Add(l.logger.GetSender().Close())

	return errors.Wrap(catcher.Resolve(), "closing log harness")
}

func (l *singleChannelLogHarness) Closed() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.closed
}
