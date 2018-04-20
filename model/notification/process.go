package notification

import (
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// NotificationsFromEvent takes an event, processes all of its triggers, and returns
// a slice of notifications, and an error object representing all errors
// that occurred while processing triggers

// It is possible for this function to return notifications and errors at the
// same time. If the notifications array is not nil, they are valid and should
// be processed as normal.
func NotificationsFromEvent(e *event.EventLogEntry) ([]Notification, error) {
	prefetch, triggers := registry.Triggers(e.ResourceType)
	if len(triggers) == 0 {
		return nil, errors.Errorf("no triggers for event type: '%s'", e.ResourceType)
	}

	data, err := prefetch(e)
	if err != nil {
		return nil, errors.Wrapf(err, "prefetch function for '%s' failed", e.ResourceType)
	}

	notifications := []Notification{}
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
