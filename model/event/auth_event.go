package event

import (
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

const (
	ResourceTypeUserRoles = "USER_ROLES"
	ResourceTypeRole      = "ROLE"
	EventTypeUserRoles    = "USER_ROLES_CHANGED"
	EventTypeRole         = "ROLE_CHANGED"
)

type RoleChangeOperation string

const (
	AddRole    RoleChangeOperation = "add"
	UpdateRole RoleChangeOperation = "update"
	RemoveRole RoleChangeOperation = "remove"
)

func (op RoleChangeOperation) validate() error {
	switch op {
	case AddRole, UpdateRole, RemoveRole:
		return nil
	default:
		return errors.Errorf("invalid role change operation '%s'", op)
	}
}

type userRolesData struct {
	User      string              `bson:"user" json:"user"`
	RoleID    string              `bson:"role_id" json:"role_id"`
	Operation RoleChangeOperation `bson:"operation" json:"operation"`
}

func LogUserRolesEvent(user, roleID string, op RoleChangeOperation) error {
	if err := op.validate(); err != nil {
		return errors.Wrapf(err, "failed to log user role event for user '%s' and role '%s'", user, roleID)
	}

	data := userRolesData{
		User:      user,
		RoleID:    roleID,
		Operation: op,
	}
	event := EventLogEntry{
		Timestamp:    time.Now(),
		EventType:    EventTypeUserRoles,
		Data:         data,
		ResourceType: ResourceTypeUserRoles,
	}
	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		return errors.Wrapf(err, "failed to log user role event for user '%s' and role '%s'", user, roleID)
	}

	return nil
}

type roleData struct {
	Before    *gimlet.Role        `bson:"before" json:"before"`
	After     *gimlet.Role        `bson:"after" json:"after"`
	Operation RoleChangeOperation `bson:"operation" json:"operation"`
}

func LogRoleEvent(before, after *gimlet.Role, op RoleChangeOperation) error {
	if err := op.validate(); err != nil {
		return errors.Wrapf(err, "failed to log role event for  role '%s'", before.ID)
	}

	data := roleData{
		Before:    before,
		After:     after,
		Operation: op,
	}
	event := EventLogEntry{
		Timestamp:    time.Now(),
		EventType:    EventTypeRole,
		Data:         data,
		ResourceType: ResourceTypeRole,
	}
	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		return errors.Wrapf(err, "failed to log user role event for role '%s'", before.ID)
	}

	return nil
}
