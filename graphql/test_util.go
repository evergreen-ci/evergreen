package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/require"
)

func setupPermissions(t *testing.T) {
	env := evergreen.GetEnvironment()
	ctx := context.Background()
	require.NoError(t, env.DB().Drop(ctx))

	roleManager := env.RoleManager()

	roles, err := roleManager.GetAllRoles(t.Context())
	require.NoError(t, err)
	require.Empty(t, roles)

	superUserRole := gimlet.Role{
		ID:    "superuser",
		Name:  "superuser",
		Scope: "superuser_scope",
		Permissions: map[string]int{
			evergreen.PermissionProjectCreate: evergreen.ProjectCreate.Value,
			evergreen.PermissionDistroCreate:  evergreen.DistroCreate.Value,
			evergreen.PermissionRoleModify:    evergreen.RoleModify.Value,
			evergreen.PermissionAdminSettings: evergreen.AdminSettingsEdit.Value,
		},
	}
	err = roleManager.UpdateRole(t.Context(), superUserRole)
	require.NoError(t, err)

	superUserScope := gimlet.Scope{
		ID:        "superuser_scope",
		Name:      "superuser scope",
		Type:      evergreen.SuperUserResourceType,
		Resources: []string{"super_user"},
	}
	err = roleManager.AddScope(t.Context(), superUserScope)
	require.NoError(t, err)

	superUserProjectRole := gimlet.Role{
		ID:    evergreen.SuperUserProjectAccessRole,
		Name:  "admin access",
		Scope: evergreen.AllProjectsScope,
		Permissions: map[string]int{
			evergreen.PermissionLogs:            evergreen.LogsView.Value,
			evergreen.PermissionPatches:         evergreen.PatchSubmitAdmin.Value,
			evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
			evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
			evergreen.PermissionAnnotations:     evergreen.AnnotationsModify.Value,
		},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), superUserProjectRole))

	superUserProjectScope := gimlet.Scope{
		ID:        evergreen.AllProjectsScope,
		Name:      "all projects",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"project_id", "project_id_2"},
	}
	require.NoError(t, roleManager.AddScope(t.Context(), superUserProjectScope))

	superUserDistroRole := gimlet.Role{
		ID:    evergreen.SuperUserDistroAccessRole,
		Name:  "admin access",
		Scope: evergreen.AllDistrosScope,
		Permissions: map[string]int{
			evergreen.PermissionDistroSettings: evergreen.DistroSettingsAdmin.Value,
			evergreen.PermissionHosts:          evergreen.HostsEdit.Value,
		},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), superUserDistroRole))

	superUserDistroScope := gimlet.Scope{
		ID:        evergreen.AllDistrosScope,
		Name:      "all distros",
		Type:      evergreen.DistroResourceType,
		Resources: []string{"distro-id"},
	}
	require.NoError(t, roleManager.AddScope(t.Context(), superUserDistroScope))

	projectScope := gimlet.Scope{
		ID:        "project_scope",
		Name:      "project scope",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"project_id"},
	}
	err = roleManager.AddScope(t.Context(), projectScope)
	require.NoError(t, err)

	projectAdminRole := gimlet.Role{
		ID:    "admin_project",
		Scope: projectScope.ID,
		Permissions: map[string]int{
			evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
			evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
			evergreen.PermissionPatches:         evergreen.PatchSubmitAdmin.Value,
			evergreen.PermissionLogs:            evergreen.LogsView.Value,
		},
	}
	err = roleManager.UpdateRole(t.Context(), projectAdminRole)
	require.NoError(t, err)

	projectViewRole := gimlet.Role{
		ID:    "view_project",
		Scope: projectScope.ID,
		Permissions: map[string]int{
			evergreen.PermissionProjectSettings: evergreen.ProjectSettingsView.Value,
			evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
			evergreen.PermissionPatches:         evergreen.PatchSubmit.Value,
			evergreen.PermissionLogs:            evergreen.LogsView.Value,
		},
	}
	err = roleManager.UpdateRole(t.Context(), projectViewRole)
	require.NoError(t, err)

	distroScope := gimlet.Scope{
		ID:        "distro_distro-id",
		Name:      "distro-id",
		Type:      evergreen.DistroResourceType,
		Resources: []string{"distro-id"},
	}
	require.NoError(t, roleManager.AddScope(t.Context(), distroScope))

	distroAdminRole := gimlet.Role{
		ID:          "admin_distro-id",
		Scope:       distroScope.ID,
		Permissions: map[string]int{evergreen.PermissionDistroSettings: evergreen.DistroSettingsAdmin.Value},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), distroAdminRole))

	distroEditRole := gimlet.Role{
		ID:          "edit_distro-id",
		Scope:       distroScope.ID,
		Permissions: map[string]int{evergreen.PermissionDistroSettings: evergreen.DistroSettingsEdit.Value},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), distroEditRole))

	distroViewRole := gimlet.Role{
		ID:          "view_distro-id",
		Scope:       distroScope.ID,
		Permissions: map[string]int{evergreen.PermissionDistroSettings: evergreen.DistroSettingsView.Value},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), distroViewRole))

	hostEditRole := gimlet.Role{
		ID:          "edit_host-id",
		Scope:       distroScope.ID,
		Permissions: map[string]int{evergreen.PermissionHosts: evergreen.HostsEdit.Value},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), hostEditRole))

	hostViewRole := gimlet.Role{
		ID:          "view_host-id",
		Scope:       distroScope.ID,
		Permissions: map[string]int{evergreen.PermissionHosts: evergreen.HostsView.Value},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), hostViewRole))

	taskAdminRole := gimlet.Role{
		ID:          "admin_task",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionTasks: evergreen.TasksAdmin.Value},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), taskAdminRole))

	taskEditRole := gimlet.Role{
		ID:          "edit_task",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionTasks: evergreen.TasksBasic.Value},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), taskEditRole))

	taskViewRole := gimlet.Role{
		ID:          "view_task",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionTasks: evergreen.TasksView.Value},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), taskViewRole))

	annotationEditRole := gimlet.Role{
		ID:          "edit_annotation",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionAnnotations: evergreen.AnnotationsModify.Value},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), annotationEditRole))

	annotationViewRole := gimlet.Role{
		ID:          "view_annotation",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionAnnotations: evergreen.AnnotationsView.Value},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), annotationViewRole))

	patchAdminRole := gimlet.Role{
		ID:          "admin_patch",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionPatches: evergreen.PatchSubmitAdmin.Value},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), patchAdminRole))

	patchEditRole := gimlet.Role{
		ID:          "edit_patch",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionPatches: evergreen.PatchSubmit.Value},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), patchEditRole))

	logViewRole := gimlet.Role{
		ID:          "view_logs",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionLogs: evergreen.LogsView.Value},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), logViewRole))
}

func setupUser(t *testing.T) (*user.DBUser, error) {
	require.NoError(t, db.Clear(user.Collection),
		"unable to clear user collection")
	dbUser := &user.DBUser{
		Id: testUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert(t.Context()))
	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	return user.GetOrCreateUser(t.Context(), testUser, "Evergreen User", email, accessToken, refreshToken, []string{})
}
