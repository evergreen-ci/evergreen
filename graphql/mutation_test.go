package graphql

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
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
