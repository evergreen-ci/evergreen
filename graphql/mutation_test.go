package graphql

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
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
	setupPermissions(t)

	t.Run("ProjectNotFound", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(model.ProjectRefCollection))
		usr := &user.DBUser{Id: testUser}
		ctx := gimlet.AttachUser(t.Context(), usr)

		resolver := &mutationResolver{}
		_, err := resolver.PromoteVarsToRepo(ctx, PromoteVarsToRepoInput{
			ProjectID: "nonexistent",
			VarNames:  []string{"var1"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("ProjectNotAttachedToRepo", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(model.ProjectRefCollection))
		projectRef := &model.ProjectRef{
			Id: "project_no_repo",
		}
		require.NoError(t, projectRef.Insert(t.Context()))

		usr := &user.DBUser{Id: testUser}
		ctx := gimlet.AttachUser(t.Context(), usr)

		resolver := &mutationResolver{}
		_, err := resolver.PromoteVarsToRepo(ctx, PromoteVarsToRepoInput{
			ProjectID: "project_no_repo",
			VarNames:  []string{"var1"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not attached to a repo")
	})

	t.Run("UserLacksRepoPermission", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.RepoRefCollection, user.Collection))

		repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
			Id: "repo_id",
		}}
		require.NoError(t, repoRef.Replace(t.Context()))

		projectRef := &model.ProjectRef{
			Id:        "project_id",
			RepoRefId: "repo_id",
		}
		require.NoError(t, projectRef.Insert(t.Context()))

		// Create a user without repo-level edit permission.
		usr, err := user.GetOrCreateUser(t.Context(), "unprivileged_user", "Unprivileged User", "unpriv@test.com", "", "", []string{})
		require.NoError(t, err)
		ctx := gimlet.AttachUser(t.Context(), usr)

		resolver := &mutationResolver{}
		_, err = resolver.PromoteVarsToRepo(ctx, PromoteVarsToRepoInput{
			ProjectID: "project_id",
			VarNames:  []string{"var1"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not have permission")
	})

	t.Run("UserWithRepoPermissionProceeds", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.RepoRefCollection, user.Collection))

		repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
			Id: "repo_id",
		}}
		require.NoError(t, repoRef.Replace(t.Context()))

		// Use "project_id" which is already in the AllProjectsScope from setupPermissions.
		projectRef := &model.ProjectRef{
			Id:        "project_id",
			RepoRefId: "repo_id",
		}
		require.NoError(t, projectRef.Insert(t.Context()))

		// Add repo_id to the AllProjectsScope so the superuser role applies to it.
		env := evergreen.GetEnvironment()
		roleManager := env.RoleManager()
		require.NoError(t, roleManager.AddResourceToScope(t.Context(), evergreen.AllProjectsScope, "repo_id"))

		// Create user with superuser project access role (has ProjectSettingsEdit on AllProjectsScope).
		usr, err := user.GetOrCreateUser(t.Context(), testUser, "Test User", "test@test.com", "", "", []string{})
		require.NoError(t, err)
		require.NoError(t, usr.AddRole(t.Context(), evergreen.SuperUserProjectAccessRole))
		ctx := gimlet.AttachUser(t.Context(), usr)

		resolver := &mutationResolver{}
		// This will pass the permission check but may fail in the data layer
		// due to missing project vars or Parameter Store setup. The key assertion
		// is that it does NOT return a permission error.
		_, err = resolver.PromoteVarsToRepo(ctx, PromoteVarsToRepoInput{
			ProjectID: "project_id",
			VarNames:  []string{"var1"},
		})
		if err != nil {
			assert.NotContains(t, err.Error(), "does not have permission")
		}
	})
}
