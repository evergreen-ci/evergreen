package apm

import (
	"github.com/evergreen-ci/birch"
	"github.com/mongodb/grip/message"
	"go.mongodb.org/mongo-driver/event"
)

// Monitor provides a high level command monitoring total.
type Monitor interface {
	DriverAPM() *event.CommandMonitor
	Rotate() Event
}

// Event describes a single "event" produced by rotating the Client's
// cached storage. These events aren't single events from the
// perspective of the driver, but rather a window of events.
type Event interface {
	Message() message.Composer
	Document() *birch.Document
}
