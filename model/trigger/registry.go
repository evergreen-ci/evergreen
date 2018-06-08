package trigger

import (
	"fmt"
	"sync"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

var registry = triggerRegistry{
	handlers: map[string]eventHandlerFactory{},
	lock:     sync.RWMutex{},
}

type triggerRegistry struct {
	handlers map[string]eventHandlerFactory
	lock     sync.RWMutex
}

func (r *triggerRegistry) eventHandler(resourceType string) eventHandler {
	r.lock.RLock()
	defer r.lock.RUnlock()

	f, ok := r.handlers[resourceType]
	if !ok {
		grip.Error(message.Fields{
			"message": "unknown event handler",
			"r_type":  resourceType,
			"cause":   "programmer error",
		})
		return nil
	}

	return f()
}

func (r *triggerRegistry) registerEventHandler(resourceType, eventData string, h eventHandlerFactory) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if _, ok := r.handlers[resourceType]; ok {
		panic(fmt.Sprintf("tried to register an eventHandler with duplicate key '%s'", resourceType))
	}

	r.handlers[resourceType] = h
}

func ValidateTrigger(resourceType, triggerName string) bool {
	h := registry.eventHandler(resourceType)
	if h == nil {
		return false
	}

	return h.ValidateTrigger(triggerName)
}
