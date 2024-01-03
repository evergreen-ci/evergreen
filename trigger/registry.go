package trigger

import (
	"fmt"
	"sync"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

var registry = triggerRegistry{
	handlers:               map[registryKey]eventHandlerFactory{},
	handlersByResourceType: map[string][]eventHandlerFactory{},
	lock:                   sync.RWMutex{},
}

func init() {
	registry.registerEventHandler(event.ResourceTypePatch, event.PatchStateChange, makePatchTriggers)
	registry.registerEventHandler(event.ResourceTypePatch, event.PatchChildrenCompletion, makePatchTriggers)
}

type registryKey struct {
	resourceType  string
	eventDataType string
}

type triggerRegistry struct {
	handlers               map[registryKey]eventHandlerFactory
	handlersByResourceType map[string][]eventHandlerFactory
	lock                   sync.RWMutex
}

func (r *triggerRegistry) eventHandler(resourceType, eventDataType string) eventHandler {
	r.lock.RLock()
	defer r.lock.RUnlock()

	f, ok := r.handlers[registryKey{resourceType: resourceType, eventDataType: eventDataType}]
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

	key := registryKey{resourceType: resourceType, eventDataType: eventData}
	if _, ok := r.handlers[key]; ok {
		panic(fmt.Sprintf("tried to register an eventHandler with duplicate key '%s'", resourceType))
	}

	r.handlersByResourceType[resourceType] = append(r.handlersByResourceType[resourceType], h)
	r.handlers[key] = h
}

func ValidateTrigger(resourceType, triggerName string) bool {
	handlers := registry.handlersByResourceType[resourceType]
	for i := range handlers {
		if handlers[i]().ValidateTrigger(triggerName) {
			return true
		}
	}

	return false
}

// ConvertToFamilyTrigger takes a trigger and returns its "family-" equivalent, if it exists.
func ConvertToFamilyTrigger(trigger string) string {
	switch trigger {
	case event.TriggerOutcome:
		return event.TriggerFamilyOutcome
	case event.TriggerFailure:
		return event.TriggerFamilyFailure
	case event.TriggerSuccess:
		return event.TriggerFamilySuccess
	default:
		return trigger
	}
}
