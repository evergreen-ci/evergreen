package event

import (
	"time"

	"github.com/pkg/errors"
)

const (
	ResourceTypeUser = "USER"
	ResourceTypeRole = "ROLE"
)

// UserEventType represents types of changes possible to a DB user.
type UserEventType string

const (
	UserEventTypeRolesUpdate UserEventType = "USER_ROLES_UPDATED"
)

func (e UserEventType) validate() error {
	switch e {
	case UserEventTypeRolesUpdate:
		return nil
	default:
		return errors.Errorf("invalid user event type '%s'", e)
	}
}

type userData struct {
	User   string      `bson:"user" json:"user"`
	Before interface{} `bson:"before" json:"before"`
	After  interface{} `bson:"after" json:"after"`
}

// LogUserEvent logs a DB User change to the event log collection.
func LogUserEvent(user string, eventType UserEventType, before, after interface{}) error {
	if err := eventType.validate(); err != nil {
		return errors.Wrapf(err, "failed to log user event for user '%s'", user)
	}

	data := userData{
		User:   user,
		Before: before,
		After:  after,
	}
	event := EventLogEntry{
		Timestamp:    time.Now(),
		EventType:    string(eventType),
		Data:         data,
		ResourceType: ResourceTypeUser,
	}
	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		return errors.Wrapf(err, "failed to log user event for user '%s'", user)
	}

	return nil
}
