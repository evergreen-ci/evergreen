package notification

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

var triggerRegistry map[string][]trigger = map[string][]trigger{}

func init() {
	triggerRegistry[event.ResourceTypeTest] = []trigger{testTrigger}
}

func getTriggers(resourceType string) []trigger {
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
			n = append(n, Notification{
				ID:         bson.NewObjectId(),
				Subscriber: subs[i].Subscribers[j].Subscriber,
				Payload:    payload,
			})
		}
	}

	return n, nil
}
