package model

import (
	"github.com/evergreen-ci/evergreen"
)

type APIPermissions struct {
	ProjectPermissions []APIPermission `json:"projectPermissions"`
	DistroPermissions  []APIPermission `json:"distroPermissions"`
}

type APIPermission struct {
	Key    string                      `json:"key"`
	Name   string                      `json:"name"`
	Levels []evergreen.PermissionLevel `json:"levels"`
}

// APIPermissionLevel describes a single permission level within a category.
type APIPermissionLevel struct {
	Description       string `json:"description"`
	GrantedToAllUsers bool   `json:"granted_to_all_users"`
}

// APIAvailablePermissions wraps the system-wide permission levels with a note clarifying their scope.
type APIAvailablePermissions struct {
	Note        string                          `json:"note"`
	Permissions map[string][]APIPermissionLevel `json:"permissions"`
}

// APIUserProjectPermissions is the response for GET /users/{user_id}/permission-details.
type APIUserProjectPermissions struct {
	UserID               string                         `json:"user_id"`
	AvailablePermissions *APIAvailablePermissions       `json:"available_permissions,omitempty"`
	Projects             *[]APIProjectPermissionSummary `json:"projects,omitempty"`
	Distros              *[]APIDistroPermissionSummary  `json:"distros,omitempty"`
}

// APIProjectPermissionSummary lists the granted permissions for one project, grouped by category.
// Categories with no access are omitted.
type APIProjectPermissionSummary struct {
	ProjectID         string              `json:"project_id"`
	ProjectIdentifier string              `json:"project_identifier"`
	IsRepo            bool                `json:"is_repo"`
	Permissions       map[string][]string `json:"permissions"`
}

// APIDistroPermissionSummary lists the granted permissions for one distro, grouped by category.
// Categories with no access are omitted.
type APIDistroPermissionSummary struct {
	DistroID    string              `json:"distro_id"`
	Permissions map[string][]string `json:"permissions"`
}
