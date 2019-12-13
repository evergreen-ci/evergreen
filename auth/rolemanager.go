package auth

import (
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type eventLoggingRoleManager struct {
	gimlet.RoleManager
}

// NewEventLoggingRoleManager returns a layered gimlet.RoleManager
// implementation that supports Evergreen event logging.
func NewEventLoggingRoleManager(rm gimlet.RoleManager) gimlet.RoleManager {
	return &eventLoggingRoleManager{rm}
}

func (l *eventLoggingRoleManager) DeleteRole(role string) error {
	roles, err := l.GetRoles([]string{role})
	if err != nil {
		return errors.Wrapf(err, "problem getting original role '%s' for event logging", role)
	}
	if len(roles) == 0 {
		return nil
	}

	if err := l.RoleManager.DeleteRole(role); err != nil {
		return err
	}

	return event.LogRoleEvent(&roles[0], nil, event.RemoveRole)
}

func (l *eventLoggingRoleManager) UpdateRole(role gimlet.Role) error {
	roles, err := l.GetRoles([]string{role.ID})
	if err != nil {
		return errors.Wrapf(err, "problem getting original role '%s' for event logging", role.ID)
	}

	if err := l.RoleManager.UpdateRole(role); err != nil {
		return err
	}

	op := event.AddRole
	var before *gimlet.Role
	if len(roles) > 0 {
		before = &roles[0]
		op = event.UpdateRole
	}
	return event.LogRoleEvent(before, &role, op)
}

func (l *eventLoggingRoleManager) Clear() error {
	roles, err := l.GetAllRoles()
	if err != nil {
		return errors.Wrapf(err, "problem getting all roles for event logging")
	}
	if len(roles) == 0 {
		return nil
	}

	if err := l.RoleManager.Clear(); err != nil {
		return err
	}

	catcher := grip.NewBasicCatcher()
	for _, role := range roles {
		catcher.Add(event.LogRoleEvent(&role, nil, event.RemoveRole))
	}
	return catcher.Resolve()
}
