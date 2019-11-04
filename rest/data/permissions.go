package data

import (
	"github.com/evergreen-ci/evergreen"
)

type permissionLevel struct {
	description string
	value       int
}

type permission struct {
	name             string
	levels permissionLevel[]
}

func GetProjectPermissions() []permission {
	permissionsData := []permission{}

	for _, permissionKey = range evergreen.ProjectPermissions {
		permissions := evergreen.MapPermissionKeyToPermissions(permissionKey)
		name := evergreen.MapPermissionKeyToName(permissionKey)
		levels := []permissionLevel{}
		
		for _, p = range permissions {
			pl = permissionLevel{
				description : p.String(),
				value : p.Value(),
			}
			append(levels, pl)
		}

		p := permission{
			name: name,
			levels: levels
		}
		append(permissionsData, p)
	}

	return permissionsData
}

