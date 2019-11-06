package model

import (
	"github.com/evergreen-ci/evergreen"
)

type APIPermissions struct {
	ProjectPermissions []APIPermission `json: "projectPermissions"`
	DistroPermissions  []APIPermission `json: "distroPermissions"`
}

type APIPermission struct {
	Key    string                      `json:"key"`
	Name   string                      `json:"name"`
	Levels []evergreen.PermissionLevel `json:"levels"`
}
