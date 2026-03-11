package graphql

import (
	"sync"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUser(t *testing.T) {
	require.NoError(t, db.Clear(user.Collection))

	testUsers := []user.DBUser{
		{
			Id:           "user1",
			DispName:     "User One",
			EmailAddress: "user1@example.com",
		},
		{
			Id:           "user2",
			DispName:     "User Two",
			EmailAddress: "user2@example.com",
		},
		{
			Id:           "user3",
			DispName:     "User Three",
			EmailAddress: "user3@example.com",
		},
	}
	for _, u := range testUsers {
		require.NoError(t, u.Insert(t.Context()))
	}

	t.Run("SingleUserLookup", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())

		result, err := GetUser(ctx, "user1")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "user1", *result.UserID)
		assert.Equal(t, "User One", *result.DisplayName)
		assert.Equal(t, "user1@example.com", *result.EmailAddress)
	})

	t.Run("UserNotFound", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())

		result, err := GetUser(ctx, "nonexistent")
		require.NoError(t, err)
		assert.Nil(t, result, "should return nil for non-existent user")
	})

	t.Run("BatchedLookups", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())

		// Simulate concurrent requests that should be batched.
		var wg sync.WaitGroup
		type getUserResult struct {
			userID string
			found  bool
			err    error
		}
		resultsChan := make(chan getUserResult, 3)

		userIDs := []string{"user1", "user2", "user3"}
		for _, id := range userIDs {
			wg.Add(1)
			go func(userID string) {
				defer wg.Done()
				result, err := GetUser(ctx, userID)
				resultsChan <- getUserResult{userID: userID, found: result != nil, err: err}
			}(id)
		}

		wg.Wait()
		close(resultsChan)

		results := make(map[string]bool)
		for result := range resultsChan {
			require.NoError(t, result.err, "lookup should not error")
			results[result.userID] = result.found
		}
		assert.Len(t, results, 3)
		for _, id := range userIDs {
			assert.True(t, results[id], "expected to find user '%s'", id)
		}
	})

	t.Run("MixedExistingAndNonExisting", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())

		// Simulate concurrent requests that should be batched.
		var wg sync.WaitGroup
		type getUserResult struct {
			userID string
			found  bool
			err    error
		}
		resultsChan := make(chan getUserResult, 3)

		userIDs := []string{"user1", "nonexistent", "user3"}
		for _, id := range userIDs {
			wg.Add(1)
			go func(userID string) {
				defer wg.Done()
				result, err := GetUser(ctx, userID)
				resultsChan <- getUserResult{userID: userID, found: result != nil, err: err}
			}(id)
		}

		wg.Wait()
		close(resultsChan)

		results := make(map[string]bool)
		for result := range resultsChan {
			require.NoError(t, result.err, "lookup should not error")
			results[result.userID] = result.found
		}
		assert.Len(t, results, 3)
		assert.True(t, results["user1"], "user1 should be found")
		assert.False(t, results["nonexistent"], "nonexistent should not be found")
		assert.True(t, results["user3"], "user3 should be found")
	})
}
