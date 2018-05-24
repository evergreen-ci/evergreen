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

		groupedSubs, err := event.FindSubscriptions(e.ResourceType, gen.triggerName, gen.selectors)
		if err != nil {
			catcher.Add(errors.Wrap(err, "failed to fetch subscribers"))
			continue
		}
		num := 0
		for _, v := range groupedSubs {
			num += len(v)
		}
		if num == 0 {
			return nil, nil
		}

		for subType, subs := range groupedSubs {
			for _, sub := range subs {
				notification, err := gen.generate(e, subType, sub)
				if err != nil {
					catcher.Add(errors.Wrap(err, "failed to generate a notification"))
					continue
				}
				if notification != nil {
					notifications = append(notifications, *notification)
				}
			}
		}
	}

	return notifications, catcher.Resolve()
}
