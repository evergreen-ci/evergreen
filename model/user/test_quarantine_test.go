package user

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasTestQuarantinePermissionForAdminRoles(t *testing.T) {
	ctx := t.Context()
	collections := []string{evergreen.RoleCollection, evergreen.ScopeCollection}
	require.NoError(t, db.ClearCollections(collections...))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	t.Cleanup(func() { assert.NoError(t, db.ClearCollections(collections...)) })
	rm := testutil.NewEnvironment(ctx, t).RoleManager()

	for _, scope := range []gimlet.Scope{
		{ID: "project_scope", Type: evergreen.ProjectResourceType, Resources: []string{"project"}},
		{ID: "repo_scope", Type: evergreen.ProjectResourceType, Resources: []string{"repo", "linked_branch"}},
		{ID: "superuser_scope", Type: evergreen.SuperUserResourceType, Resources: []string{evergreen.SuperUserPermissionsID}},
	} {
		require.NoError(t, rm.AddScope(ctx, scope))
	}
	for _, role := range []gimlet.Role{
		{
			ID:          "legacy_project_admin",
			Scope:       "project_scope",
			Permissions: gimlet.Permissions{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value},
		},
		{
			ID:          "legacy_repo_admin",
			Scope:       "repo_scope",
			Permissions: gimlet.Permissions{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value},
		},
		{
			ID:    "project_admin_with_no_quarantine_grant",
			Scope: "project_scope",
			Permissions: gimlet.Permissions{
				evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
				evergreen.PermissionTestQuarantine:  evergreen.TestQuarantineNone.Value,
			},
		},
		{
			ID:          evergreen.SuperUserRole,
			Scope:       "superuser_scope",
			Permissions: gimlet.Permissions{evergreen.PermissionAdminSettings: evergreen.AdminSettingsEdit.Value},
		},
		{
			ID:          "admin_settings_outside_superuser_scope",
			Scope:       "project_scope",
			Permissions: gimlet.Permissions{evergreen.PermissionAdminSettings: evergreen.AdminSettingsEdit.Value},
		},
	} {
		require.NoError(t, rm.UpdateRole(ctx, role))
	}

	for name, tc := range map[string]struct {
		roleID   string
		project  string
		expected bool
	}{
		"LegacyProjectAdminShouldQuarantine":             {"legacy_project_admin", "project", true},
		"RepoAdminShouldQuarantineLinkedBranch":          {"legacy_repo_admin", "linked_branch", true},
		"ProjectAdminShouldNotQuarantineOtherProject":    {"legacy_project_admin", "other_project", false},
		"ProjectAdminWithNoQuarantineGrantShouldSucceed": {"project_admin_with_no_quarantine_grant", "project", true},
		"SuperuserWithoutProjectRoleShouldQuarantine":    {evergreen.SuperUserRole, "project", true},
		"AdminSettingsOutsideSuperuserScopeShouldError":  {"admin_settings_outside_superuser_scope", "other_project", false},
	} {
		t.Run(name, func(t *testing.T) {
			usr := DBUser{SystemRoles: []string{tc.roleID}}
			allowed, err := usr.HasPermissionErr(t.Context(), gimlet.PermissionOpts{
				Resource:      tc.project,
				ResourceType:  evergreen.ProjectResourceType,
				Permission:    evergreen.PermissionTestQuarantine,
				RequiredLevel: evergreen.TestQuarantineEdit.Value,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.expected, allowed)
		})
	}
}
