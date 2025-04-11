package event

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

func init() {
	registry.AddType(ResourceTypeUser, func() any { return &userData{} })
}

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
	User   string `bson:"user" json:"user"`
	Before any    `bson:"before" json:"before"`
	After  any    `bson:"after" json:"after"`
}

// LogUserEvent logs a DB User change to the event log collection.
func LogUserEvent(ctx context.Context, user string, eventType UserEventType, before, after any) error {
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
	if err := event.Log(ctx); err != nil {
		return errors.Wrapf(err, "logging user event for user '%s'", user)
	}

	return nil
}
