package trigger

import (
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// NotificationsFromEvent takes an event, processes all of its triggers, and returns
// a slice of notifications, and an error object representing all errors
// that occurred while processing triggers

// It is possible for this function to return notifications and errors at the
// same time. If the notifications array is not nil, they are valid and should
// be processed as normal.
func NotificationsFromEvent(e *event.EventLogEntry) ([]notification.Notification, error) {
	h := registry.eventHandler(e.ResourceType, e.EventType)
	if h == nil {
		return nil, errors.Errorf("unknown event ResourceType '%s' or EventType '%s'", e.ResourceType, e.EventType)
	}

	if err := h.Fetch(e); err != nil {
		return nil, errors.Wrapf(err, "error fetching data for event: %s (%s, %s)", e.ID, e.ResourceType, e.EventType)
	}

	subscriptions, err := event.FindSubscriptions(e.ResourceType, h.Selectors())
	msg := message.Fields{
		"source":              "events-processing",
		"message":             "processing event",
		"event_id":            e.ID.Hex(),
		"event_type":          e.EventType,
		"event_resource_type": e.ResourceType,
		"event_resource":      e.ResourceId,
		"num_subscriptions":   len(subscriptions),
	}
	if err != nil {
		err = errors.Wrapf(err, "error fetching subscriptions for event: %s (%s, %s)", e.ID, e.ResourceType, e.EventType)
		grip.Error(message.WrapError(err, msg))
		return nil, err
	}
	grip.Info(msg)
	if len(subscriptions) == 0 {
		return nil, nil
	}

	notifications := make([]notification.Notification, 0, len(subscriptions))

	catcher := grip.NewSimpleCatcher()
	for i := range subscriptions {
		n, err := h.Process(&subscriptions[i])
		msg := message.Fields{
			"source":              "events-processing",
			"message":             "processing subscription",
			"event_id":            e.ID.Hex(),
			"event_type":          e.EventType,
			"event_resource_type": e.ResourceType,
			"event_resource":      e.ResourceId,
			"subscription_id":     subscriptions[i].ID,
			"notification_is_nil": n == nil,
		}
		catcher.Add(err)
		if err != nil {
			grip.Error(message.WrapError(err, msg))
		} else {
			grip.Info(msg)
		}
		if n == nil {
			continue
		}

		notifications = append(notifications, *n)
	}

	return notifications, catcher.Resolve()
}
