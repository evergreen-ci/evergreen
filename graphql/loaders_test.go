package graphql

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	testutil.Setup()
}

func TestGetUser(t *testing.T) {
	require.NoError(t, db.Clear(user.Collection))

	// Insert test users
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

		// Simulate concurrent requests that should be batched
		var wg sync.WaitGroup
		results := make([]*string, 3)
		errors := make([]error, 3)

		userIDs := []string{"user1", "user2", "user3"}
		for i, id := range userIDs {
			wg.Add(1)
			go func(idx int, userID string) {
				defer wg.Done()
				result, err := GetUser(ctx, userID)
				errors[idx] = err
				if result != nil {
					results[idx] = result.UserID
				}
			}(i, id)
		}
		wg.Wait()

		// All lookups should succeed
		for i, err := range errors {
			require.NoError(t, err, "lookup %d should not error", i)
		}
		for i, result := range results {
			require.NotNil(t, result, "result %d should not be nil", i)
			assert.Equal(t, userIDs[i], *result)
		}
	})

	t.Run("MixedExistingAndNonExisting", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())

		var wg sync.WaitGroup
		type lookupResult struct {
			user *string
			err  error
		}
		results := make([]lookupResult, 3)

		userIDs := []string{"user1", "nonexistent", "user3"}
		for i, id := range userIDs {
			wg.Add(1)
			go func(idx int, userID string) {
				defer wg.Done()
				result, err := GetUser(ctx, userID)
				results[idx].err = err
				if result != nil {
					results[idx].user = result.UserID
				}
			}(i, id)
		}
		wg.Wait()

		// All lookups should succeed (no errors)
		for i, r := range results {
			require.NoError(t, r.err, "lookup %d should not error", i)
		}

		// user1 and user3 should be found
		require.NotNil(t, results[0].user)
		assert.Equal(t, "user1", *results[0].user)

		// nonexistent should be nil
		assert.Nil(t, results[1].user, "nonexistent user should return nil")

		// user3 should be found
		require.NotNil(t, results[2].user)
		assert.Equal(t, "user3", *results[2].user)
	})
}

func TestMiddleware(t *testing.T) {
	t.Run("InjectsLoadersIntoContext", func(t *testing.T) {
		var capturedCtx context.Context

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})

		wrappedHandler := Middleware(handler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify loaders were injected
		loaders := For(capturedCtx)
		assert.NotNil(t, loaders)
		assert.NotNil(t, loaders.UserLoader)
	})
}

func TestNewLoaders(t *testing.T) {
	loaders := NewLoaders()
	assert.NotNil(t, loaders)
	assert.NotNil(t, loaders.UserLoader)
}

// setupLoaderContext creates a context with dataloaders injected.
func setupLoaderContext(ctx context.Context) context.Context {
	loaders := NewLoaders()
	return context.WithValue(ctx, loadersKey, loaders)
}
