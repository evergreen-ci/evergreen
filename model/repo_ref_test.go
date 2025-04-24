package model

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoRefUpdateAdminRoles(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection, user.Collection))
	env := evergreen.GetEnvironment()
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	rm := env.RoleManager()
	r := RepoRef{ProjectRef{
		Id: "proj",
	}}
	require.NoError(t, r.Replace(t.Context()))
	adminScope := gimlet.Scope{
		ID:        "repo_scope",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"proj", "other_project", "branch_project"},
	}
	require.NoError(t, rm.AddScope(adminScope))
	adminRole := gimlet.Role{
		ID:          fmt.Sprintf("admin_repo_%s", r.Id),
		Scope:       "repo_scope",
		Permissions: adminPermissions,
	}
	require.NoError(t, rm.UpdateRole(adminRole))
	oldAdmin := user.DBUser{
		Id:          "oldAdmin",
		SystemRoles: []string{adminRole.ID},
	}
	require.NoError(t, oldAdmin.Insert(t.Context()))
	newAdmin := user.DBUser{
		Id: "newAdmin",
	}
	require.NoError(t, newAdmin.Insert(t.Context()))

	assert.NoError(t, r.UpdateAdminRoles(t.Context(), []string{newAdmin.Id}, []string{oldAdmin.Id}))
	oldAdminFromDB, err := user.FindOneByIdContext(t.Context(), oldAdmin.Id)
	assert.NoError(t, err)
	assert.Empty(t, oldAdminFromDB.Roles())
	newAdminFromDB, err := user.FindOneByIdContext(t.Context(), newAdmin.Id)
	assert.NoError(t, err)
	assert.Len(t, newAdminFromDB.Roles(), 1)
}
