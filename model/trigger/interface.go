package trigger

import (
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/pkg/errors"
)

// `eventHandlerFactory`s create `eventHandler`s capable of validating triggers
type eventHandlerFactory func() eventHandler

// EventHandler
type eventHandler interface {
	// Fetch retrieves the event's underlying document from the
	// EventLogEntry
	Fetch(*event.EventLogEntry) error

	// Selectors creates a slice of selectors suitable for fetching
	// subscriptions for the event. Selectors should not perform
	// any fetch operations.
	Selectors() []event.Selector

	// Process creates a notification for an event from a single
	// Subscription
	Process(*event.EventLogEntry, *event.Subscription) (*notification.Notification, error)

	// Validate returns true if the string refers to a valid trigger
	ValidateTrigger(string) bool
}

type trigger func(*event.EventLogEntry, *event.Subscription) (*notification.Notification, error)

type base struct {
	triggers map[string]trigger
}

func (b *base) Process(entry *event.EventLogEntry, sub *event.Subscription) (*notification.Notification, error) {
	f, ok := b.triggers[sub.Trigger]
	if !ok {
		return nil, errors.Errorf("unknown trigger: '%s'", sub.Trigger)
	}

	return f(entry, sub)
}

func (b *base) ValidateTrigger(t string) bool {
	_, ok := b.triggers[t]
	return ok
}
