package triggers

import (
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type trigger func(*event.EventLogEntry) ([]notification.Notification, error)

// ProcessTriggers takes an event, processes all of its triggers, and returns
// a slice of notifications, and an error object representing all errors
// that occurred while processing triggers

// It is possible for this notification to return notifications and errors at the
// same time. If the notifications array is not nil, they are valid and should
// be processed as normal.
func ProcessTriggers(event *event.EventLogEntry) ([]notification.Notification, error) {
	triggers := getTriggers(event.Type())
	if len(triggers) == 0 {
		return nil, errors.Errorf("no triggers for event type: '%s'", event.Type())
	}

	notifications := []notification.Notification{}
	catcher := grip.NewSimpleCatcher()
	for _, f := range triggers {
		notes, err := f(event)
		catcher.Add(err)
		if len(notes) > 0 {
			notifications = append(notifications, notes...)
		}
	}

	return notifications, catcher.Resolve()
}
