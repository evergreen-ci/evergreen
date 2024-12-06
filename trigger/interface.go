package trigger

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
)

// `eventHandlerFactory`s create `eventHandler`s capable of validating triggers
type eventHandlerFactory func() eventHandler

// EventHandler
type eventHandler interface {
	// Fetch retrieves the event's underlying document from the
	// EventLogEntry
	Fetch(context.Context, *event.EventLogEntry) error

	// Attributes returns Attributes for this event
	// suitable for matching with subscription filters.
	// Attributes should not perform any fetch operations.
	Attributes() event.Attributes

	// Process creates a notification for an event from a single
	// Subscription
	Process(context.Context, *event.Subscription) (*notification.Notification, error)

	// Validate returns true if the string refers to a valid trigger
	ValidateTrigger(string) bool
}

type trigger func(context.Context, *event.Subscription) (*notification.Notification, error)

type base struct {
	triggers map[string]trigger
}

func (b *base) Process(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	f, found := b.triggers[sub.Trigger]
	if !found {
		return nil, nil
	}

	return f(ctx, sub)
}

func (b *base) ValidateTrigger(t string) bool {
	_, ok := b.triggers[t]
	return ok
}
