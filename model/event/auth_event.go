package event

import (
	"time"

	"github.com/pkg/errors"
)

const (
	ResourceTypeUser = "USER"
)

// UserEventType represents types of changes possible to a DB user.
type UserEventType string

const (
	UserEventTypeRolesUpdate            UserEventType = "USER_ROLES_UPDATED"
	UserEventTypeFavoriteProjectsUpdate UserEventType = "USER_FAVORITE_PROJECTS_UPDATED"
)

func (e UserEventType) validate() error {
	switch e {
	case UserEventTypeRolesUpdate:
		return nil
	case UserEventTypeFavoriteProjectsUpdate:
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
		return errors.Wrapf(err, "invalid user event for user '%s'", user)
	}

	data := userData{
		User:   user,
		Before: before,
		After:  after,
	}
	event := EventLogEntry{
		Timestamp:    time.Now(),
		EventType:    string(eventType),
		ResourceId:   user,
		Data:         data,
		ResourceType: ResourceTypeUser,
	}
	if err := event.Log(); err != nil {
		return errors.Wrapf(err, "logging user event for user '%s'", user)
	}

	return nil
}
