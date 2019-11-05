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

func (apiPermissions *APIPermissions) BuildFromService() APIPermissions {
	return APIPermissions{
		Permissions: []permission{
			permission{
				Key:    evergreen.PermissionProjectSettings,
				Name:   evergreen.MapPermissionKeyToName(evergreen.PermissionProjectSettings),
				Levels: evergreen.MapPermissionKeyToPermissionLevels(evergreen.PermissionProjectSettings),
			},
		},
	}
}
