package notification

import (
	"fmt"
	"sync"

	"github.com/evergreen-ci/evergreen/model/event"
)

var triggerRegistry map[string][]trigger = map[string][]trigger{}
var triggerRegistryM = sync.RWMutex{}

func registryAdd(eventResourceType string, t ...trigger) {
	triggerRegistryM.Lock()
	defer triggerRegistryM.Unlock()

	triggers, _ := triggerRegistry[eventResourceType]
	triggerRegistry[eventResourceType] = append(triggers, t...)
}

func getTriggers(resourceType string) []trigger {
	triggerRegistryM.RLock()
	defer triggerRegistryM.RUnlock()

	triggers, ok := triggerRegistry[resourceType]
	if !ok {
		return nil
	}

	return triggers
}

// TODO delete this and test event after the first real event is implemented
func init() {
	registryAdd(event.ResourceTypeTest, testTrigger)
}

func testTrigger(e *event.EventLogEntry) ([]Notification, error) {
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
	p := payloads{
		email: &EmailPayload{
			Subject: "Hi",
			Body:    fmt.Sprintf("event says '%s'", data.Message),
		},
	}

	return p.generateNotifications(e, "test", selectors)
}
