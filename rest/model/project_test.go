package model

import (
	"reflect"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v70/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoBuildFromService(t *testing.T) {
	repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
		Id:                  "project",
		Owner:               "my_owner",
		Repo:                "my_repo",
		GithubChecksEnabled: utility.TruePtr(),
		PRTestingEnabled:    utility.FalsePtr(),
		CommitQueue: model.CommitQueueParams{
			MergeMethod: "Squash", // being partially populated shouldn't prevent enabled from being defaulted
		},
		TestSelection: model.TestSelectionSettings{
			Allowed:        utility.FalsePtr(),
			DefaultEnabled: utility.TruePtr(),
		},
		RunEveryMainlineCommit: utility.TruePtr(),
	}}
	apiRef := &APIProjectRef{}
	assert.NoError(t, apiRef.BuildFromService(t.Context(), repoRef.ProjectRef))
	// not defaulted yet
	require.NotNil(t, apiRef)
	assert.Nil(t, apiRef.GitTagVersionsEnabled)

	apiRef.DefaultUnsetBooleans()
	assert.True(t, *apiRef.GithubChecksEnabled)
	assert.False(t, *apiRef.PRTestingEnabled)
	require.NotNil(t, apiRef.GitTagVersionsEnabled) // should default
	assert.False(t, *apiRef.GitTagVersionsEnabled)

	require.NotNil(t, apiRef.CommitQueue.Enabled)
	assert.False(t, *apiRef.CommitQueue.Enabled)

	require.NotNil(t, apiRef.TestSelection.Allowed)
	assert.False(t, *apiRef.TestSelection.Allowed)
	assert.True(t, utility.FromBoolPtr(apiRef.TestSelection.DefaultEnabled))

	require.NotNil(t, apiRef.RunEveryMainlineCommit)
	assert.True(t, *apiRef.RunEveryMainlineCommit)
}

func TestRecursivelyDefaultBooleans(t *testing.T) {
	type insideStruct struct {
		InsideBool *bool
	}
	type testStruct struct {
		EmptyBool *bool
		TrueBool  *bool
		Strct     insideStruct
		PtrStruct *insideStruct
	}

	myStruct := testStruct{TrueBool: utility.TruePtr()}
	reflected := reflect.ValueOf(&myStruct).Elem()
	recursivelyDefaultBooleans(reflected)

	require.NotNil(t, myStruct.EmptyBool)
	assert.False(t, *myStruct.EmptyBool)
	assert.True(t, *myStruct.TrueBool)
	require.NotNil(t, myStruct.Strct.InsideBool)
	assert.False(t, *myStruct.EmptyBool)
	assert.Nil(t, myStruct.PtrStruct) // shouldn't be affected
}

func TestGitHubDynamicTokenPermissionGroupToService(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	t.Run("Empty permissions should result in no permissions", func(t *testing.T) {
		req := APIGitHubDynamicTokenPermissionGroup{
			Name:        utility.ToStringPtr("some-group"),
			Permissions: map[string]string{},
		}
		model, err := req.ToService()
		require.NoError(err)
		require.NotNil(model)

		assert.Equal("some-group", model.Name)
		assert.False(model.AllPermissions)
		assert.Nil(model.Permissions.Contents)
	})

	t.Run("Invalid permissions should result in an error", func(t *testing.T) {
		req := APIGitHubDynamicTokenPermissionGroup{
			Name:        utility.ToStringPtr("some-group"),
			Permissions: map[string]string{"invalid": "invalid"},
		}
		_, err := req.ToService()
		require.ErrorContains(err, "decoding GitHub permissions")
	})

	t.Run("Valid permissions should be converted", func(t *testing.T) {
		req := APIGitHubDynamicTokenPermissionGroup{
			Name:        utility.ToStringPtr("some-group"),
			Permissions: map[string]string{"contents": "read", "pull_requests": "write"},
		}
		model, err := req.ToService()
		require.NoError(err)
		require.NotNil(model)

		assert.Equal("some-group", model.Name)
		assert.False(model.AllPermissions)
		assert.Equal("read", utility.FromStringPtr(model.Permissions.Contents))
		assert.Equal("write", utility.FromStringPtr(model.Permissions.PullRequests))
	})

	t.Run("All permissions set should be kept", func(t *testing.T) {
		req :=
			APIGitHubDynamicTokenPermissionGroup{
				Name:           utility.ToStringPtr("some-group"),
				AllPermissions: utility.TruePtr(),
			}
		model, err := req.ToService()
		require.NoError(err)
		require.NotNil(model)

		assert.Equal("some-group", model.Name)
		assert.True(model.AllPermissions)
	})

	t.Run("If permissions are set and all permissions is true, an error is returned", func(t *testing.T) {
		req := APIGitHubDynamicTokenPermissionGroup{
			Name:           utility.ToStringPtr("some-group"),
			Permissions:    map[string]string{"contents": "read", "pull_requests": "write"},
			AllPermissions: utility.TruePtr(),
		}
		_, err := req.ToService()
		require.ErrorContains(err, "a group will all permissions must have no permissions set")
	})
}

func TestGitHubDynamicTokenPermissionGroupBuildFromService(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	t.Run("Empty permissions should result in no permissions", func(t *testing.T) {
		pg := APIGitHubDynamicTokenPermissionGroup{}
		err := pg.BuildFromService(model.GitHubDynamicTokenPermissionGroup{})
		require.NoError(err)
		assert.Equal("", utility.FromStringPtr(pg.Name))
		assert.Empty(pg.Permissions)
	})

	t.Run("Valid permissions should be converted", func(t *testing.T) {
		pg := APIGitHubDynamicTokenPermissionGroup{}
		err := pg.BuildFromService(model.GitHubDynamicTokenPermissionGroup{
			Name: "some-group",
			Permissions: github.InstallationPermissions{
				Administration: utility.ToStringPtr("write"),
				Actions:        utility.ToStringPtr("write"),
				Checks:         utility.ToStringPtr("read"),
			},
		})
		require.NoError(err)
		require.Len(pg.Permissions, 3)
		assert.Equal("some-group", utility.FromStringPtr(pg.Name))
		assert.Contains(pg.Permissions, "administration")
		assert.Equal("write", pg.Permissions["administration"])
		assert.Contains(pg.Permissions, "actions")
		assert.Equal("write", pg.Permissions["actions"])
		assert.Contains(pg.Permissions, "checks")
		assert.Equal("read", pg.Permissions["checks"])
	})

	t.Run("All permissions set (default permission group) should be kept", func(t *testing.T) {
		pg := APIGitHubDynamicTokenPermissionGroup{}
		err := pg.BuildFromService(model.GitHubDynamicTokenPermissionGroup{
			Name:           "some-group",
			AllPermissions: true,
		})
		require.NoError(err)
		assert.Equal("some-group", utility.FromStringPtr(pg.Name))
		assert.True(utility.FromBoolPtr(pg.AllPermissions))
	})
}
