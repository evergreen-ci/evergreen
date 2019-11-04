package data

import (
	"github.com/evergreen-ci/evergreen"
)

type PermissionLevel struct {
	description string
	value       int
}

type Permission struct {
	name   string
	levels []PermissionLevel
}

func GetProjectPermissions() []Permission {
	permissionsData := []Permission{}

	for _, permissionKey := range evergreen.ProjectPermissions {
		permissions := evergreen.MapPermissionKeyToPermissions(permissionKey)
		name := evergreen.MapPermissionKeyToName(permissionKey)
		levels := []PermissionLevel{}

		for _, p := range permissions {
			pl := PermissionLevel{
				description: p.String(),
				value:       p.Value(),
			}
			levels = append(levels, pl)
		}

		perm := Permission{
			name:   name,
			levels: levels,
		}
		permissionsData = append(permissionsData, perm)
	}

	return permissionsData
}
