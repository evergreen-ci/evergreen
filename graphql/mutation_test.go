package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetAPIKey(t *testing.T) {
	assert.NoError(t, db.ClearCollections(user.Collection, evergreen.RoleCollection, evergreen.ScopeCollection))

	t.Run("SuccessfullyResetsAPIKey", func(t *testing.T) {
		// Create a test user
		usr := &user.DBUser{
			Id:     "test_user",
			APIKey: "old_api_key",
		}
		require.NoError(t, usr.Insert(t.Context()))

		// Attach user to context
		ctx := gimlet.AttachUser(context.Background(), usr)

		// Create resolver
		resolver := &mutationResolver{}

		// Call ResetAPIKey
		result, err := resolver.ResetAPIKey(ctx)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify the result
		assert.Equal(t, "test_user", result.User)
		assert.NotEmpty(t, result.APIKey)
		assert.NotEqual(t, "old_api_key", result.APIKey)

		// Verify the user's API key was updated in the database
		updatedUser, err := user.FindOneByIdContext(t.Context(), "test_user")
		require.NoError(t, err)
		require.NotNil(t, updatedUser)
		assert.Equal(t, result.APIKey, updatedUser.APIKey)
	})

	t.Run("ErrorWhenNoUserInContext", func(t *testing.T) {
		// Create resolver without a user in context
		resolver := &mutationResolver{}
		ctx := context.Background()

		// Call ResetAPIKey - mustHaveUser returns empty user, which causes UpdateAPIKey to fail
		result, err := resolver.ResetAPIKey(ctx)
		// Should get an error because the empty user doesn't exist in the database
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "updating user API key")
	})
}

