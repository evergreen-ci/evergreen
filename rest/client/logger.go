package client

import (
	"io"
	"sync"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

// LoggerProducer provides a mechanism for agents (and command pluings) to access the
// process' logging facilities. The interfaces are all based on grip
// interfaces and abstractions, and the behavior of the interfaces is
// dependent on the configuration and implementation of the
// LoggerProducer instance.
type LoggerProducer interface {
	// The Execution/Task/System loggers provide a grip-like
	// logging interface for the distinct logging channels that the
	// Evergreen agent provides to tasks
	Execution() grip.Journaler
	Task() grip.Journaler
	System() grip.Journaler

	// The writer functions return an io.Writer for use with
	// exec.Cmd operations for capturing standard output and standard
	// error from sbprocesses.
	TaskWriter(level.Priority) io.WriteCloser
	SystemWriter(level.Priority) io.WriteCloser

	// Close releases all resources by calling Close on all underlying senders.
	Close() error
	// Closed returns true if this logger has been closed, false otherwise.
	Closed() bool
}

////////////////////////////////////////////////////////////////////////
//
// Standard/Default Production  LoggerProducer

// logHarness provides a straightforward implementation of the
// plugin.LoggerProducer interface.
type logHarness struct {
	execution grip.Journaler
	task      grip.Journaler
	system    grip.Journaler
	mu        sync.RWMutex
	writers   []io.WriteCloser
	closed    bool
}

func (l *logHarness) Execution() grip.Journaler { return l.execution }
func (l *logHarness) Task() grip.Journaler      { return l.task }
func (l *logHarness) System() grip.Journaler    { return l.system }

func (l *logHarness) TaskWriter(p level.Priority) io.WriteCloser {
	l.mu.Lock()
	defer l.mu.Unlock()

	w := send.MakeWriterSender(l.task.GetSender(), p)
	l.writers = append(l.writers, w)
	return w
}

func (l *logHarness) SystemWriter(p level.Priority) io.WriteCloser {
	l.mu.Lock()
	defer l.mu.Unlock()

	w := send.MakeWriterSender(l.system.GetSender(), p)
	l.writers = append(l.writers, w)
	return w
}

func (l *logHarness) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true

	catcher := grip.NewBasicCatcher()

	for _, w := range l.writers {
		catcher.Add(w.Close())
	}

	catcher.Add(l.execution.GetSender().Close())
	catcher.Add(l.task.GetSender().Close())
	catcher.Add(l.system.GetSender().Close())

	return errors.Wrap(catcher.Resolve(), "problem closing log harness")
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
	logger  grip.Journaler
	mu      sync.RWMutex
	writers []io.WriteCloser
	closed  bool
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

func (l *singleChannelLogHarness) TaskWriter(p level.Priority) io.WriteCloser {
	l.mu.Lock()
	defer l.mu.Unlock()

	w := send.MakeWriterSender(l.logger.GetSender(), p)
	l.writers = append(l.writers, w)
	return w
}

func (l *singleChannelLogHarness) SystemWriter(p level.Priority) io.WriteCloser {
	l.mu.Lock()
	defer l.mu.Unlock()

	w := send.MakeWriterSender(l.logger.GetSender(), p)
	l.writers = append(l.writers, w)
	return w
}

func (l *singleChannelLogHarness) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true

	catcher := grip.NewBasicCatcher()

	for _, w := range l.writers {
		catcher.Add(w.Close())
	}

	catcher.Add(l.logger.GetSender().Close())

	return errors.Wrap(catcher.Resolve(), "problem closing log harness")
}

func (l *singleChannelLogHarness) Closed() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.closed
}
