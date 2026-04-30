package loaders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func init() {
	testutil.Setup()
}

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
		assert.Equal(t, "user1", result.Id)
		assert.Equal(t, "User One", result.DispName)
		assert.Equal(t, "user1@example.com", result.EmailAddress)
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

func TestGetVersion(t *testing.T) {
	require.NoError(t, db.Clear(model.VersionCollection))

	testVersions := []model.Version{
		{
			Id:                  "version1",
			CreateTime:          time.Now(),
			Revision:            "abc123",
			Author:              "user1",
			Message:             "First commit",
			Status:              "created",
			RevisionOrderNumber: 1,
			Identifier:          "test-project",
		},
		{
			Id:                  "version2",
			CreateTime:          time.Now(),
			Revision:            "def456",
			Author:              "user2",
			Message:             "Second commit",
			Status:              "created",
			RevisionOrderNumber: 2,
			Identifier:          "test-project",
		},
		{
			Id:                  "version3",
			CreateTime:          time.Now(),
			Revision:            "ghi789",
			Author:              "user3",
			Message:             "Third commit",
			Status:              "created",
			RevisionOrderNumber: 3,
			Identifier:          "test-project",
		},
	}
	for _, v := range testVersions {
		require.NoError(t, v.Insert(t.Context()))
	}

	t.Run("SingleVersionLookup", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())

		result, err := GetVersion(ctx, "version1")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "version1", result.Id)
		assert.Equal(t, "abc123", result.Revision)
		assert.Equal(t, "user1", result.Author)
		assert.Equal(t, "First commit", result.Message)
	})

	t.Run("VersionNotFound", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())

		result, err := GetVersion(ctx, "nonexistent")
		require.NoError(t, err)
		assert.Nil(t, result, "should return nil for non-existent version")
	})

	t.Run("BatchedLookups", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())

		// Simulate concurrent requests that should be batched.
		var wg sync.WaitGroup
		type getVersionResult struct {
			versionID string
			found     bool
			err       error
		}
		resultsChan := make(chan getVersionResult, 3)

		versionIDs := []string{"version1", "version2", "version3"}
		for _, id := range versionIDs {
			wg.Add(1)
			go func(versionID string) {
				defer wg.Done()
				result, err := GetVersion(ctx, versionID)
				resultsChan <- getVersionResult{versionID: versionID, found: result != nil, err: err}

			}(id)
		}

		wg.Wait()
		close(resultsChan)

		results := make(map[string]bool)
		for result := range resultsChan {
			require.NoError(t, result.err, "lookup should not error")
			results[result.versionID] = result.found
		}
		assert.Len(t, results, 3)
		for _, id := range versionIDs {
			assert.True(t, results[id], "expected to find version '%s'", id)
		}
	})

	t.Run("MixedExistingAndNonExisting", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())

		var wg sync.WaitGroup
		type getVersionResult struct {
			versionID string
			found     bool
			err       error
		}
		resultsChan := make(chan getVersionResult, 3)

		versionIDs := []string{"version1", "nonexistent", "version3"}
		for _, id := range versionIDs {
			wg.Add(1)
			go func(versionID string) {
				defer wg.Done()
				result, err := GetVersion(ctx, versionID)
				if err != nil {
					resultsChan <- getVersionResult{versionID: versionID, found: false, err: err}
				} else if result == nil {
					resultsChan <- getVersionResult{versionID: versionID, found: false, err: nil}
				} else {
					resultsChan <- getVersionResult{versionID: versionID, found: true, err: nil}
				}
			}(id)
		}

		wg.Wait()
		close(resultsChan)

		results := make(map[string]bool)
		for result := range resultsChan {
			require.NoError(t, result.err, "lookup should not error")
			results[result.versionID] = result.found
		}
		assert.Len(t, results, 3)
		assert.True(t, results["version1"], "version1 should be found")
		assert.False(t, results["nonexistent"], "nonexistent should not be found")
		assert.True(t, results["version3"], "version3 should be found")
	})
}

func TestPreloadVersions(t *testing.T) {
	require.NoError(t, db.Clear(model.VersionCollection))

	testVersions := []model.Version{
		{Id: "preload1", Revision: "rev1", Author: "author1"},
		{Id: "preload2", Revision: "rev2", Author: "author2"},
		{Id: "preload3", Revision: "rev3", Author: "author3"},
	}
	for _, v := range testVersions {
		require.NoError(t, v.Insert(t.Context()))
	}

	t.Run("EmptySliceIsNoOp", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())
		require.NotPanics(t, func() {
			PreloadVersions(ctx, nil)
			PreloadVersions(ctx, []string{})
		})
	})

	t.Run("PrimesCacheSoLaterLoadsSkipDB", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())

		PreloadVersions(ctx, []string{"preload1", "preload2", "preload3"})

		// Once preloaded the values should live in the dataloader's thunk cache.
		// Drop the underlying collection to prove subsequent GetVersion calls
		// don't hit MongoDB.
		require.NoError(t, db.Clear(model.VersionCollection))

		for _, id := range []string{"preload1", "preload2", "preload3"} {
			result, err := GetVersion(ctx, id)
			require.NoError(t, err)
			require.NotNil(t, result, "expected cached version '%s'", id)
			assert.Equal(t, id, result.Id)
		}
	})

	t.Run("DuplicateIDsAreDeduped", func(t *testing.T) {
		require.NoError(t, db.Clear(model.VersionCollection))
		for _, v := range testVersions {
			require.NoError(t, v.Insert(t.Context()))
		}

		ctx := setupLoaderContext(t.Context())
		require.NotPanics(t, func() {
			PreloadVersions(ctx, []string{"preload1", "preload1", "preload2"})
		})

		result, err := GetVersion(ctx, "preload1")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "preload1", result.Id)
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
		l := For(capturedCtx)
		require.NotNil(t, l)
		require.NotNil(t, l.UserLoader)
		require.NotNil(t, l.VersionLoader)
	})
}

func TestNew(t *testing.T) {
	l := New()
	require.NotNil(t, l)
	require.NotNil(t, l.UserLoader)
	require.NotNil(t, l.VersionLoader)
}

func TestIsBatchError(t *testing.T) {
	t.Run("ReturnsTrueForBatchError", func(t *testing.T) {
		err := &batchError{err: assert.AnError}
		assert.True(t, IsBatchError(err))
	})

	t.Run("ReturnsFalseForRegularError", func(t *testing.T) {
		assert.False(t, IsBatchError(assert.AnError))
	})

	t.Run("ReturnsFalseForNil", func(t *testing.T) {
		assert.False(t, IsBatchError(nil))
	})

	// Resolvers wrap the dataloader error in a *gqlerror.Error before returning
	// it to gqlgen. IsBatchError must reach through gqlerror's Unwrap so the
	// GraphQL error presenter skips re-logging every field that shares a
	// failed batch.
	t.Run("ReturnsTrueThroughGqlerrorUnwrap", func(t *testing.T) {
		batchErr := &batchError{err: assert.AnError}
		gqlErr := &gqlerror.Error{
			Message: "fetching version 'v1' for task 't1': some cause",
			Err:     batchErr,
		}
		assert.True(t, IsBatchError(gqlErr))
	})

	t.Run("ReturnsFalseForGqlerrorWithoutCause", func(t *testing.T) {
		gqlErr := &gqlerror.Error{Message: "fetching version 'v1': some cause"}
		assert.False(t, IsBatchError(gqlErr))
	})
}

// setupLoaderContext creates a context with dataloaders injected.
func setupLoaderContext(ctx context.Context) context.Context {
	l := New()
	return context.WithValue(ctx, loadersKey, l)
}
