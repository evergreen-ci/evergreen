package model

import (
	"github.com/evergreen-ci/evergreen"
)

type APIPermissions struct {
	ProjectPermissions []APIPermission `json: "projects"`
	DistroPermissions  []APIPermission `json: "distros"`
}

type APIPermission struct {
	Key    string                      `json:"key"`
	Name   string                      `json:"name"`
	Levels []evergreen.PermissionLevel `json:"levels"`
}
