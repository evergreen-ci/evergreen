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
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestGetAPIVersion(t *testing.T) {
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

		result, err := GetAPIVersion(ctx, "version1")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "version1", utility.FromStringPtr(result.Id))
		assert.Equal(t, "abc123", utility.FromStringPtr(result.Revision))
		assert.Equal(t, "user1", utility.FromStringPtr(result.Author))
		assert.Equal(t, "First commit", utility.FromStringPtr(result.Message))
	})

	t.Run("VersionNotFound", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())

		result, err := GetAPIVersion(ctx, "nonexistent")
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
				result, err := GetAPIVersion(ctx, versionID)
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
				result, err := GetAPIVersion(ctx, versionID)
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
		require.NotNil(t, l.APIVersionLoader)
	})
}

func TestNew(t *testing.T) {
	l := New()
	require.NotNil(t, l)
	require.NotNil(t, l.UserLoader)
	require.NotNil(t, l.APIVersionLoader)
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
}

// setupLoaderContext creates a context with dataloaders injected.
func setupLoaderContext(ctx context.Context) context.Context {
	l := New()
	return context.WithValue(ctx, loadersKey, l)
}
