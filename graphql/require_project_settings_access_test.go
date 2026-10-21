package graphql

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestRequireProjectSettingsAccess(t *testing.T) {
	setupPermissions(t)
	ctx := context.Background()

	pRef := model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
		RepoRefId:  "repo_project_id",
	}
	assert.NoError(t, pRef.Insert(t.Context()))

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (any, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	usr, err := setupUser(t)
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx = gimlet.AttachUser(ctx, usr)
	require.NotNil(t, ctx)

	apiProjectSettings := &restModel.APIProjectSettings{
		ProjectRef: restModel.APIProjectRef{
			Identifier: utility.ToStringPtr("project_identifier"),
			Admins:     utility.ToStringPtrSlice([]string{"admin_1", "admin_2", "admin_3"}),
		},
	}

	fieldCtx := &graphql.FieldContext{
		Field: graphql.CollectedField{
			Field: &ast.Field{
				Alias: "admins",
			},
		},
	}
	ctx = graphql.WithFieldContext(ctx, fieldCtx)

	res, err := RequireProjectSettingsAccess(ctx, any(nil), next)
	assert.EqualError(t, err, "input: project not valid")
	assert.Nil(t, res)
	assert.Equal(t, 0, callCount)

	res, err = RequireProjectSettingsAccess(ctx, apiProjectSettings, next)
	assert.EqualError(t, err, "input: project not specified")
	assert.Nil(t, res)
	assert.Equal(t, 0, callCount)

	validApiProjectSettings := &restModel.APIProjectSettings{
		ProjectRef: restModel.APIProjectRef{
			Id:         utility.ToStringPtr("project_id"),
			RepoRefId:  utility.ToStringPtr("repo_project_id"),
			Identifier: utility.ToStringPtr("project_identifier"),
			Admins:     utility.ToStringPtrSlice([]string{"admin_1", "admin_2", "admin_3"}),
		},
	}
	validRepoProjectSettings := &restModel.APIProjectSettings{
		ProjectRef: restModel.APIProjectRef{
			Id: utility.ToStringPtr("repo_project_id"),
		},
	}
	res, err = RequireProjectSettingsAccess(ctx, validApiProjectSettings, next)
	assert.EqualError(t, err, "input: user does not have permission to access the field 'admins' for project with ID 'project_id'")
	assert.Nil(t, res)
	assert.Equal(t, 0, callCount)

	// Verify that the user also doesn't have permission to view the repo project
	res, err = RequireProjectSettingsAccess(ctx, validRepoProjectSettings, next)
	assert.EqualError(t, err, "input: user does not have permission to access the field 'admins' for project with ID 'repo_project_id'")
	assert.Nil(t, res)
	assert.Equal(t, 0, callCount)

	err = usr.AddRole(t.Context(), "view_project")
	require.NoError(t, err)

	res, err = RequireProjectSettingsAccess(ctx, validApiProjectSettings, next)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 1, callCount)

	// Verify that the user also has permission to view the repo project
	res, err = RequireProjectSettingsAccess(ctx, validRepoProjectSettings, next)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 2, callCount)
}
