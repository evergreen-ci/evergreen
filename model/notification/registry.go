package notification

import (
	"fmt"
	"sync"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/util"
)

var registry = triggerRegistry{
	triggers: map[string][]trigger{},
	lock:     sync.RWMutex{},
}

type triggerRegistry struct {
	triggers map[string][]trigger
	lock     sync.RWMutex
}

func (r *triggerRegistry) AddTrigger(resourceType string, t ...trigger) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.triggers[resourceType] = append(r.triggers[resourceType], t...)
}

func (r *triggerRegistry) Triggers(resourceType string) []trigger {
	r.lock.RLock()
	defer r.lock.RUnlock()

	triggers, ok := r.triggers[resourceType]
	if !ok {
		return nil
	}

	return triggers
}

// TODO delete this and test event after the first real event is implemented
func init() {
	registry.AddTrigger(event.ResourceTypeTest, testTrigger)
}

func testTrigger(e *event.EventLogEntry) (*notificationGenerator, error) {
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
