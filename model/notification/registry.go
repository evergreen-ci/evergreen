package notification

import (
	"fmt"
	"sync"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/util"
)

type prefetch func(*event.EventLogEntry) (interface{}, error)
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

// TODO delete this and test event after the first real event is implemented
func init() {
	registry.AddTrigger(event.ResourceTypeTest, testTrigger)
	registry.AddPrefetch(event.ResourceTypeTest, func(_ *event.EventLogEntry) (interface{}, error) {
		return nil, nil
	})
}

func testTrigger(e *event.EventLogEntry, _ interface{}) (*notificationGenerator, error) {
	data := e.Data.(*event.TestEvent)
	selectors := []event.Selector{
		{
			Type: "test",
			Data: "awesomeness",
		},
		{
			Type: "test2",
			Data: "notawesomeness",
		},
	}

	return &notificationGenerator{
		triggerName: "test",
		selectors:   selectors,
		evergreenWebhook: &util.EvergreenWebhook{
			Body: []byte(fmt.Sprintf("event says '%s'", data.Message)),
		},
	}, nil
}
