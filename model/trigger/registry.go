package trigger

import (
	"fmt"
	"sync"

	"github.com/evergreen-ci/evergreen/model/event"
)

// prefetch is a function that take an event, and fetches the data needed to
// build a notification, and it's payloads. Exactly one prefetch type must exist
// per EventLogEntry ResourceType.
type prefetch func(*event.EventLogEntry) (interface{}, error)

// trigger is a function that given an event, and the data fetched by
// the prefetch function, produces an initialized notificationGenerator
// (which) can simply have the generate method called to create notifications
//
// In the event of an error, the notificationGenerator should be ignored.
// It is valid for both the generator and error to be nil, in which case the
// trigger does not apply to the given event.
type trigger func(*event.EventLogEntry, interface{}) (*notificationGenerator, error)

var registry = triggerRegistry{
	prefetch: map[string]prefetch{},
	triggers: map[string][]trigger{},
	lock:     sync.RWMutex{},
}

type triggerRegistry struct {
	prefetch map[string]prefetch
	triggers map[string][]trigger
	lock     sync.RWMutex
}

func (r *triggerRegistry) AddPrefetch(resourceType string, f prefetch) {
	r.lock.Lock()
	defer r.lock.Unlock()

	_, ok := r.prefetch[resourceType]
	if ok {
		panic(fmt.Sprintf("prefetch function for '%s' is already set", resourceType))
	}
	r.prefetch[resourceType] = f
}

func (r *triggerRegistry) AddTrigger(resourceType string, t ...trigger) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.triggers[resourceType] = append(r.triggers[resourceType], t...)
}

func (r *triggerRegistry) Triggers(resourceType string) (prefetch, []trigger) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	prefetch, ok := r.prefetch[resourceType]
	if !ok {
		return nil, nil
	}

	triggers, ok := r.triggers[resourceType]
	if !ok {
		return nil, nil
	}

	return prefetch, triggers
}
