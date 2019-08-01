package mongorpc

import (
	"sync"

	"github.com/pkg/errors"
	"github.com/tychoish/mongorpc/mongowire"
)

type OperationRegistry struct {
	ops map[mongowire.OpScope]HandlerFunc
	mu  sync.RWMutex
}

func (o *OperationRegistry) Add(op mongowire.OpScope, h HandlerFunc) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if err := op.Validate(); err != nil {
		return errors.Wrap(err, "could not add operation, it failed to validate")
	}

	if h == nil {
		return errors.Errorf("cannot define nil handler function for %+v", op)
	}

	if _, ok := o.ops[op]; ok {
		return errors.Errorf("operation '%+v' is already defined", op)
	}

	o.ops[op] = h

	return nil
}

func (o *OperationRegistry) Get(scope *mongowire.OpScope) (HandlerFunc, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	handler, ok := o.ops[*scope]
	return handler, ok
}
