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
	h := registry.eventHandler(e)
	if h != nil {
		if err := h.Fetch(e); err != nil {
			return nil, errors.Wrapf(err, "error fetching data for event: %s (%s, %s)", e.ID, e.ResourceType, e.EventType)
		}

		subscriptions, err := event.FindSubscriptions2(e.ResourceType, h.Selectors())
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
			if err != nil || n == nil {
				continue
			}

			notifications = append(notifications, *n)
		}

		return notifications, catcher.Resolve()
	}

	// TODO delete me

	prefetch, triggers := registry.Triggers(e.ResourceType)
	if len(triggers) == 0 {
		return nil, errors.Errorf("no triggers for event type: '%s'", e.ResourceType)
	}

	data, err := prefetch(e)
	if err != nil {
		return nil, errors.Wrapf(err, "prefetch function for '%s' failed", e.ResourceType)
	}

	notifications := []notification.Notification{}
	catcher := grip.NewSimpleCatcher()
	for _, f := range triggers {
		gen, err := f(e, data)
		if err != nil {
			catcher.Add(err)
			continue
		}
		if gen == nil {
			continue
		}

		notes, err := gen.generate(e)
		if err != nil {
			catcher.Add(err)
			continue
		}
		if len(notes) > 0 {
			notifications = append(notifications, notes...)
		}

	}

	return notifications, catcher.Resolve()
}
