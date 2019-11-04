package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func GetMockPermissionsData() []Permission {
	return []Permission{
		Permission{
			name: "Settings",
			levels: []PermissionLevel{
				PermissionLevel{
					description: "Edit project settings",
					value:       20,
				},
				PermissionLevel{
					description: "View project settings",
					value:       10,
				},
				PermissionLevel{
					description: "No project settings permissions",
					value:       0,
				},
			},
		},
		Permission{
			name: "Tasks",
			levels: []PermissionLevel{
				PermissionLevel{
					description: "Full tasks permissions",
					value:       30,
				},
				PermissionLevel{
					description: "Basic modifications to tasks",
					value:       20,
				},
				PermissionLevel{
					description: "View tasks",
					value:       10,
				},
				PermissionLevel{
					description: "Not able to view or edit tasks",
					value:       0,
				},
			},
		},
		Permission{
			name: "Patches",
			levels: []PermissionLevel{
				PermissionLevel{
					description: "Submit and edit patches",
					value:       10,
				},
				PermissionLevel{
					description: "Not able to view or submit patches",
					value:       0,
				},
			},
		},
		Permission{
			name: "Logs",
			levels: []PermissionLevel{
				PermissionLevel{
					description: "View logs",
					value:       10,
				},
				PermissionLevel{
					description: "Not able to view logs",
					value:       0,
				},
			},
		},
	}
}

func TestGetProjectPermissions(t *testing.T) {
	result := GetProjectPermissions()

	assert.Equal(t, result, GetMockPermissionsData())
}
