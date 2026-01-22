package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetAPIKey(t *testing.T) {
	assert.NoError(t, db.ClearCollections(user.Collection))

	t.Run("SuccessfullyResetsAPIKey", func(t *testing.T) {
		usr := &user.DBUser{
			Id:     "test_user",
			APIKey: "old_api_key",
		}
		require.NoError(t, usr.Insert(t.Context()))

		ctx := gimlet.AttachUser(context.Background(), usr)

		resolver := &mutationResolver{}
		result, err := resolver.ResetAPIKey(ctx)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, "test_user", result.User)
		assert.NotEmpty(t, result.APIKey)
		assert.NotEqual(t, "old_api_key", result.APIKey)

		// Verify the user's API key was updated in the database
		updatedUser, err := user.FindOneByIdContext(t.Context(), "test_user")
		require.NoError(t, err)
		require.NotNil(t, updatedUser)
		assert.Equal(t, result.APIKey, updatedUser.APIKey)
	})
}
