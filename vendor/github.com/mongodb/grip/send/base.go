package send

import (
	"errors"
	"fmt"
	"sync"

	"github.com/mongodb/grip/message"
)

type BaseResetFunc func()
type BaseCloseFunc func() error

type Base struct {
	// data exposed via the interface and tools to track them
	name  string
	level LevelInfo
	mutex sync.RWMutex

	// function literals which allow customizable functionality.
	// they are set either in the constructor (e.g. MakeBase) of
	// via the SetErrorHandler/SetFormatter injector.
	errHandler ErrorHandler
	reset      BaseResetFunc
	closer     BaseCloseFunc
	formatter  MessageFormatter
}

func NewBase(n string) *Base {
	return &Base{
		name:       n,
		reset:      func() {},
		closer:     func() error { return nil },
		errHandler: func(error, message.Composer) {},
	}
}

func MakeBase(n string, r BaseResetFunc, c BaseCloseFunc) *Base {
	b := NewBase(n)
	b.reset = r
	b.closer = c

	return b
}

func (b *Base) Close() error { return b.closer() }

func (b *Base) Name() string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.name
}

func (b *Base) SetName(name string) {
	b.mutex.Lock()
	b.name = name
	b.mutex.Unlock()

	b.reset()
}

func (b *Base) SetFormatter(mf MessageFormatter) error {
	if mf == nil {
		return errors.New("cannot set message formatter to nil")
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.formatter = mf

	return nil
}

func (b *Base) SetErrorHandler(eh ErrorHandler) error {
	if eh == nil {
		return errors.New("error handler must be non-nil")
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.errHandler = eh

	return nil
}

// ErrorHandler calls the error handler, and is a wrapper around the
// embedded ErrorHandler function. It is not part of the Sender interface.
func (b *Base) ErrorHandler(err error, m message.Composer) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	b.errHandler(err, m)
}

func (b *Base) SetLevel(l LevelInfo) error {
	if !l.Valid() {
		return fmt.Errorf("level settings are not valid: %+v", l)
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.level = l

	return nil
}

func (b *Base) Level() LevelInfo {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.level
}
