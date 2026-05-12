package graphql

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetAPIKey(t *testing.T) {
	t.Run("SuccessfullyResetsAPIKey", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(user.Collection, evergreen.ConfigCollection))
		usr := &user.DBUser{
			Id:     "test_user",
			APIKey: "old_api_key",
		}
		require.NoError(t, usr.Insert(t.Context()))

		ctx := gimlet.AttachUser(t.Context(), usr)

		resolver := &mutationResolver{}
		result, err := resolver.ResetAPIKey(ctx)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, "test_user", result.User)
		assert.NotEmpty(t, result.APIKey)
		assert.NotEqual(t, "old_api_key", result.APIKey)

		// Verify the user's API key was updated in the database
		updatedUser, err := user.FindOneById(t.Context(), "test_user")
		require.NoError(t, err)
		require.NotNil(t, updatedUser)
		assert.Equal(t, result.APIKey, updatedUser.APIKey)
	})

	t.Run("StaticAPIKeysDisabled", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(user.Collection, evergreen.ConfigCollection))
		usr := &user.DBUser{
			Id:     "test_user",
			APIKey: "old_api_key",
		}
		require.NoError(t, usr.Insert(t.Context()))

		settings, err := evergreen.GetConfig(t.Context())
		require.NoError(t, err)
		settings.ServiceFlags.StaticAPIKeysDisabled = true
		require.NoError(t, settings.ServiceFlags.Set(t.Context()))

		ctx := gimlet.AttachUser(t.Context(), usr)

		resolver := &mutationResolver{}
		_, err = resolver.ResetAPIKey(ctx)
		require.ErrorContains(t, err, "static API keys are disabled")
	})

	t.Run("StaticAPIKeysDisabledDoesNotAffectServiceUser", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(user.Collection, evergreen.ConfigCollection))
		usr := &user.DBUser{
			Id:      "test_user",
			APIKey:  "old_api_key",
			OnlyAPI: true,
		}
		require.NoError(t, usr.Insert(t.Context()))

		settings, err := evergreen.GetConfig(t.Context())
		require.NoError(t, err)
		settings.ServiceFlags.StaticAPIKeysDisabled = true
		require.NoError(t, settings.ServiceFlags.Set(t.Context()))

		ctx := gimlet.AttachUser(t.Context(), usr)

		resolver := &mutationResolver{}
		result, err := resolver.ResetAPIKey(ctx)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, "test_user", result.User)
		assert.NotEmpty(t, result.APIKey)
		assert.NotEqual(t, "old_api_key", result.APIKey)

		// Verify the user's API key was updated in the database
		updatedUser, err := user.FindOneById(t.Context(), "test_user")
		require.NoError(t, err)
		require.NotNil(t, updatedUser)
		assert.Equal(t, result.APIKey, updatedUser.APIKey)
	})
}

func TestSetCursorAPIKey(t *testing.T) {
	require.NoError(t, db.ClearCollections(user.Collection, evergreen.ConfigCollection))
	usr := &user.DBUser{Id: "test_user"}
	require.NoError(t, usr.Insert(t.Context()))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pr-bot/user/cursor-key" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success": true, "keyLastFour": "1234"}`))
		}
	}))
	defer server.Close()

	sageConfig := &evergreen.SageConfig{BaseURL: server.URL}
	require.NoError(t, sageConfig.Set(t.Context()))

	ctx := gimlet.AttachUser(t.Context(), usr)
	resolver := &mutationResolver{}

	result, err := resolver.SetCursorAPIKey(ctx, "test-api-key-ending-in-1234")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	require.NotNil(t, result.KeyLastFour)
	assert.Equal(t, "1234", *result.KeyLastFour)
}

func TestDeleteCursorAPIKey(t *testing.T) {
	require.NoError(t, db.ClearCollections(user.Collection, evergreen.ConfigCollection))
	usr := &user.DBUser{Id: "test_user"}
	require.NoError(t, usr.Insert(t.Context()))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pr-bot/user/cursor-key" && r.Method == http.MethodDelete {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success": true}`))
		}
	}))
	defer server.Close()

	sageConfig := &evergreen.SageConfig{BaseURL: server.URL}
	require.NoError(t, sageConfig.Set(t.Context()))

	ctx := gimlet.AttachUser(t.Context(), usr)
	resolver := &mutationResolver{}

	result, err := resolver.DeleteCursorAPIKey(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
}

func TestPromoteVarsToRepo(t *testing.T) {
	const (
		projectID    = "p_id"
		repoID       = "r_id"
		repoScopeID  = "r_id_scope"
		repoEditRole = "edit_r_id"
	)

	// setupRepoEditRole creates a scope and role that grant
	// PermissionProjectSettings/EDIT on the repo resource. It mirrors how
	// setupPermissions wires admin_project for project resources, but no
	// shared helper exposes that for repos.
	setupRepoEditRole := func(t *testing.T) {
		rm := evergreen.GetEnvironment().RoleManager()
		require.NoError(t, rm.AddScope(t.Context(), gimlet.Scope{
			ID:        repoScopeID,
			Type:      evergreen.ProjectResourceType,
			Resources: []string{repoID},
		}))
		require.NoError(t, rm.UpdateRole(t.Context(), gimlet.Role{
			ID:    repoEditRole,
			Scope: repoScopeID,
			Permissions: map[string]int{
				evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
			},
		}))
	}

	setupFixtures := func(t *testing.T) {
		require.NoError(t, db.ClearCollections(
			model.ProjectRefCollection,
			model.RepoRefCollection,
			model.ProjectVarsCollection,
			fakeparameter.Collection,
			event.EventCollection,
		))
		pRef := model.ProjectRef{
			Id:         projectID,
			Identifier: "p_identifier",
			Owner:      "evergreen-ci",
			Repo:       "evergreen",
			Branch:     "main",
			Restricted: utility.FalsePtr(),
			RepoRefId:  repoID,
		}
		require.NoError(t, pRef.Insert(t.Context()))

		repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
			Id:         repoID,
			Owner:      "evergreen-ci",
			Repo:       "evergreen",
			Restricted: utility.FalsePtr(),
		}}
		require.NoError(t, repoRef.Replace(t.Context()))

		pVars := model.ProjectVars{
			Id:   projectID,
			Vars: map[string]string{"shared": "from-branch", "branch-only": "x"},
		}
		require.NoError(t, pVars.Insert(t.Context()))

		rVars := model.ProjectVars{
			Id:   repoID,
			Vars: map[string]string{"shared": "from-repo"},
		}
		require.NoError(t, rVars.Insert(t.Context()))
	}

	t.Run("ProjectNotFound", func(t *testing.T) {
		setupPermissions(t)
		setupFixtures(t)
		usr, err := setupUser(t)
		require.NoError(t, err)
		ctx := gimlet.AttachUser(t.Context(), usr)

		resolver := &mutationResolver{}
		ok, err := resolver.PromoteVarsToRepo(ctx, PromoteVarsToRepoInput{
			ProjectID: "does-not-exist",
			VarNames:  []string{"shared"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.False(t, ok)
	})

	t.Run("ProjectNotAttachedToRepo", func(t *testing.T) {
		setupPermissions(t)
		require.NoError(t, db.ClearCollections(model.ProjectRefCollection))
		unattached := model.ProjectRef{
			Id:         "unattached",
			Owner:      "evergreen-ci",
			Repo:       "evergreen",
			Restricted: utility.FalsePtr(),
		}
		require.NoError(t, unattached.Insert(t.Context()))

		usr, err := setupUser(t)
		require.NoError(t, err)
		ctx := gimlet.AttachUser(t.Context(), usr)

		resolver := &mutationResolver{}
		ok, err := resolver.PromoteVarsToRepo(ctx, PromoteVarsToRepoInput{
			ProjectID: "unattached",
			VarNames:  []string{"shared"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not attached to a repo")
		assert.False(t, ok)
	})

	t.Run("UserLacksRepoEditPermissionReturnsForbidden", func(t *testing.T) {
		setupPermissions(t)
		setupFixtures(t)
		setupRepoEditRole(t)

		usr, err := setupUser(t)
		require.NoError(t, err)
		// Branch admin only — explicitly not granted the repo edit role.
		require.NoError(t, usr.AddRole(t.Context(), "admin_project"))
		ctx := gimlet.AttachUser(t.Context(), usr)

		resolver := &mutationResolver{}
		ok, err := resolver.PromoteVarsToRepo(ctx, PromoteVarsToRepoInput{
			ProjectID: projectID,
			VarNames:  []string{"shared"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not have permission to edit settings for repo")
		assert.False(t, ok)

		// Repo vars must be untouched.
		repoVars, err := model.FindOneProjectVars(t.Context(), repoID)
		require.NoError(t, err)
		assert.Equal(t, "from-repo", repoVars.Vars["shared"])

		// Branch vars must also be untouched (no deletion).
		projVars, err := model.FindOneProjectVars(t.Context(), projectID)
		require.NoError(t, err)
		assert.Equal(t, "from-branch", projVars.Vars["shared"])
	})

	t.Run("UserWithRepoEditPermissionSucceeds", func(t *testing.T) {
		setupPermissions(t)
		setupFixtures(t)
		setupRepoEditRole(t)

		usr, err := setupUser(t)
		require.NoError(t, err)
		require.NoError(t, usr.AddRole(t.Context(), repoEditRole))
		ctx := gimlet.AttachUser(t.Context(), usr)

		resolver := &mutationResolver{}
		ok, err := resolver.PromoteVarsToRepo(ctx, PromoteVarsToRepoInput{
			ProjectID: projectID,
			VarNames:  []string{"shared"},
		})
		require.NoError(t, err)
		assert.True(t, ok)

		repoVars, err := model.FindOneProjectVars(t.Context(), repoID)
		require.NoError(t, err)
		assert.Equal(t, "from-branch", repoVars.Vars["shared"])

		projVars, err := model.FindOneProjectVars(t.Context(), projectID)
		require.NoError(t, err)
		assert.NotContains(t, projVars.Vars, "shared")
		assert.Equal(t, "x", projVars.Vars["branch-only"])
	})
}
