package model

import (
	"github.com/evergreen-ci/evergreen"
)

type APIPermissions struct {
	Permissions []APIPermission `json: "permissions"`
}

type APIPermission struct {
	Key    string                      `json:"key"`
	Name   string                      `json:"name"`
	Levels []evergreen.PermissionLevel `json:"levels"`
}
