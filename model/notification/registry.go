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

func init() {
	registryAdd(event.ResourceTypeTest, testTrigger)
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

func testTrigger(e *event.EventLogEntry) ([]Notification, error) {
	subs, err := event.FindSubscribers("test", "test", []event.Selector{
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

	n := make([]Notification, 0, len(subs))

	catcher := grip.NewSimpleCatcher()
	for i := range subs {
		if subs[i].Type != event.EmailSubscriberType {
			catcher.Add(errors.New("trigger only supports email subscriptions"))
			continue
		}

		payload := &EmailPayload{
			Subject: "Hi",
			Body:    fmt.Sprintf("event says '%s'", e.Data.(*event.TestEvent).Message),
		}

		for j := range subs[i].Subscribers {
			sub := &subs[i].Subscribers[j].Subscriber
			n = append(n, Notification{
				ID:         makeNotificationID(e, "test", sub),
				Subscriber: *sub,
				Payload:    payload,
			})
		}
	}

	return n, nil
}
