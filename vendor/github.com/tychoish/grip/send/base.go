package send

import (
	"errors"
	"fmt"
	"sync"

	"github.com/tychoish/grip/message"
)

type base struct {
	name       string
	level      LevelInfo
	reset      func()
	closer     func() error
	errHandler ErrorHandler
	sync.RWMutex
}

func newBase(n string) *base {
	return &base{
		name:       n,
		reset:      func() {},
		closer:     func() error { return nil },
		errHandler: func(error, message.Composer) {},
	}
}

func (b *base) Close() error { return b.closer() }

func (b *base) Name() string {
	b.RLock()
	defer b.RUnlock()

	return b.name
}

func (b *base) SetName(name string) {
	b.Lock()
	b.name = name
	b.Unlock()

	b.reset()
}

func (b *base) SetErrorHandler(eh ErrorHandler) error {
	if eh == nil {
		return errors.New("error handler must be non-nil")
	}

	b.Lock()
	defer b.Unlock()
	b.errHandler = eh

	return nil
}

func (b *base) SetLevel(l LevelInfo) error {
	if !l.Valid() {
		return fmt.Errorf("level settings are not valid: %+v", l)
	}

	b.Lock()
	defer b.Unlock()

	b.level = l

	return nil
}

func (b *base) Level() LevelInfo {
	b.RLock()
	defer b.RUnlock()

	return b.level
}
