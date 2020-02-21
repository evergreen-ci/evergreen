package send

import (
	"errors"
	"fmt"
	"sync"

	"github.com/mongodb/grip/message"
)

// Base provides most of the functionality of the Sender interface,
// except for the Send method, to facilitate writing novel Sender
// implementations. All implementations of the functions
type Base struct {
	// data exposed via the interface and tools to track them
	name  string
	level LevelInfo
	mutex sync.RWMutex

	// function literals which allow customizable functionality.
	// they are set either in the constructor (e.g. MakeBase) of
	// via the SetErrorHandler/SetFormatter injector.
	errHandler ErrorHandler
	reset      func()
	closer     func() error
	formatter  MessageFormatter
}

// NewBase constructs a basic Base structure with no op functions for
// reset, close, and error handling.
func NewBase(n string) *Base {
	return &Base{
		name:       n,
		reset:      func() {},
		closer:     func() error { return nil },
		errHandler: func(error, message.Composer) {},
	}
}

// MakeBase constructs a Base structure that allows callers to specify
// the reset and caller function.
func MakeBase(n string, reseter func(), closer func() error) *Base {
	b := NewBase(n)
	b.reset = reseter
	b.closer = closer

	return b
}

// Close calls the closer function.
func (b *Base) Close() error { return b.closer() }

// Name returns the name of the Sender.
func (b *Base) Name() string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.name
}

// SetName allows clients to change the name of the Sender.
func (b *Base) SetName(name string) {
	b.mutex.Lock()
	b.name = name
	b.mutex.Unlock()

	b.reset()
}

// SetFormatter users to set the formatting function used to construct log messages.
func (b *Base) SetFormatter(mf MessageFormatter) error {
	if mf == nil {
		return errors.New("cannot set message formatter to nil")
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.formatter = mf

	return nil
}

// Formatter returns the formatter, defaulting to using the string
// form of the message if no formatter is configured.
func (b *Base) Formatter() MessageFormatter {
	return func(m message.Composer) (string, error) {
		b.mutex.RLock()
		defer b.mutex.RUnlock()

		if b.formatter == nil {
			return m.String(), nil
		}

		return b.formatter(m)
	}
}

// SetErrorHandler configures the error handling function for this Sender.
func (b *Base) SetErrorHandler(eh ErrorHandler) error {
	if eh == nil {
		return errors.New("error handler must be non-nil")
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.errHandler = eh

	return nil
}

// ErrorHandler returns an error handling functioncalls the error handler, and is a wrapper around the
// embedded ErrorHandler function.
func (b *Base) ErrorHandler() ErrorHandler {
	return func(err error, m message.Composer) {
		if err == nil {
			return
		}

		b.mutex.RLock()
		defer b.mutex.RUnlock()

		b.errHandler(err, m)
	}
}

// SetLevel configures the level (default levels and threshold levels)
// for the Sender.
func (b *Base) SetLevel(l LevelInfo) error {
	if !l.Valid() {
		return fmt.Errorf("level settings are not valid: %+v", l)
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.level = l

	return nil
}

// Level reports the currently configured level for the Sender.
func (b *Base) Level() LevelInfo {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.level
}
