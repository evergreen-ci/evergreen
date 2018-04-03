package notification

import (
	"fmt"
	"sync"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
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
	groupedSubs, err := event.FindSubscribers("test", "test", []event.Selector{
		{
			Type: "test",
			Data: "awesomeness",
		},
		{
			Type: "test2",
			Data: "notawesomeness",
		},
	})
	if err != nil {
		return nil, err
	}

	n := []Notification{}

	catcher := grip.NewSimpleCatcher()
	for subGroup, _ := range groupedSubs {
		if subGroup != event.EmailSubscriberType {
			catcher.Add(errors.New("trigger only supports email subscriptions"))
			continue
		}

		payload := &EmailPayload{
			Subject: "Hi",
			Body:    fmt.Sprintf("event says '%s'", e.Data.(*event.TestEvent).Message),
		}

		for j := range groupedSubs[subGroup] {
			sub := &groupedSubs[subGroup][j]
			notification, err := New(e, "test", sub, payload)
			if err != nil {
				catcher.Add(err)
				continue
			}
			n = append(n, *notification)
		}
	}

	return n, nil
}
