package trigger

import (
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// NotificationsFromEvent takes an event, processes all of its triggers, and returns
// a slice of notifications, and an error object representing all errors
// that occurred while processing triggers

// It is possible for this function to return notifications and errors at the
// same time. If the notifications array is not nil, they are valid and should
// be processed as normal.
func NotificationsFromEvent(e *event.EventLogEntry) ([]notification.Notification, error) {
	h := registry.eventHandler(e.ResourceType)
	if h == nil {
		return nil, errors.Errorf("unknown event ResourceType: '%s'", e.ResourceType)
	}

	if err := h.Fetch(e); err != nil {
		return nil, errors.Wrapf(err, "error fetching data for event: %s (%s, %s)", e.ID, e.ResourceType, e.EventType)
	}

	subscriptions, err := event.FindSubscriptions(e.ResourceType, h.Selectors())
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching subscriptions for event: %s (%s, %s)", e.ID, e.ResourceType, e.EventType)
	}
	if len(subscriptions) == 0 {
		return nil, nil
	}

	notifications := make([]notification.Notification, 0, len(subscriptions))

	catcher := grip.NewSimpleCatcher()
	for i := range subscriptions {
		n, err := h.Process(&subscriptions[i])
		catcher.Add(err)
		if n == nil {
			continue
		}

		notifications = append(notifications, *n)
	}

	return notifications, catcher.Resolve()
}
