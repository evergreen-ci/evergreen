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

// APIUserProjectPermissions is the default response for GET /users/{user_id}/permission-details.
type APIUserProjectPermissions struct {
	UserID   string                        `json:"user_id"`
	Legend   map[string][]string           `json:"legend"`
	Projects []APIProjectPermissionSummary `json:"projects"`
}

// APIProjectPermissionSummary lists the granted permissions for one project or distro, grouped by category.
// Categories with no access are omitted.
type APIProjectPermissionSummary struct {
	ProjectID         string              `json:"project_id"`
	ProjectIdentifier string              `json:"project_identifier"`
	Permissions       map[string][]string `json:"permissions"`
}

// APIUserProjectPermissionsFull is the response for GET /users/{user_id}/permission-details?all=true.
type APIUserProjectPermissionsFull struct {
	UserID   string                            `json:"user_id"`
	Legend   map[string][]string               `json:"legend"`
	Projects []APIProjectPermissionSummaryFull `json:"projects"`
}

// APIProjectPermissionSummaryFull shows all permission categories for one project or distro,
// with true/false for each level indicating whether the user has been granted it.
type APIProjectPermissionSummaryFull struct {
	ProjectID         string                     `json:"project_id"`
	ProjectIdentifier string                     `json:"project_identifier"`
	Permissions       map[string]map[string]bool `json:"permissions"`
}
