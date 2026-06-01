package graphql

import (
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
