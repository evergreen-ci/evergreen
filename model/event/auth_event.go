package event

import (
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

const (
	ResourceTypeUser     = "USER"
	ResourceTypeRole     = "ROLE"
	EventTypeRoleChanged = "ROLE_CHANGED"
)

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

type RoleEventType string

const (
	RoleEventTypeAdd    RoleEventType = "ROLE_ADDED"
	RoleEventTypeUpdate RoleEventType = "ROLE_UPDATED"
	RoleEventTypeRemove RoleEventType = "ROLE_REMOVED"
)

func (e RoleEventType) validate() error {
	switch e {
	case RoleEventTypeAdd, RoleEventTypeUpdate, RoleEventTypeRemove:
		return nil
	default:
		return errors.Errorf("invalid role event type '%s'", e)
	}
}

type roleData struct {
	Before *gimlet.Role `bson:"before" json:"before"`
	After  *gimlet.Role `bson:"after" json:"after"`
}

func LogRoleEvent(eventType RoleEventType, before, after *gimlet.Role) error {
	if err := eventType.validate(); err != nil {
		return errors.Wrapf(err, "failed to log role event for role '%s'", before.ID)
	}

	data := roleData{
		Before: before,
		After:  after,
	}
	event := EventLogEntry{
		Timestamp:    time.Now(),
		EventType:    string(eventType),
		Data:         data,
		ResourceType: ResourceTypeRole,
	}
	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		return errors.Wrapf(err, "failed to log role event for role '%s'", before.ID)
	}

	return nil
}
