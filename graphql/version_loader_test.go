package graphql

import (
	"sync"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		assert.Equal(t, "version1", utility.FromStringPtr(result.Id))
		assert.Equal(t, "abc123", utility.FromStringPtr(result.Revision))
		assert.Equal(t, "user1", utility.FromStringPtr(result.Author))
		assert.Equal(t, "First commit", utility.FromStringPtr(result.Message))
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
		assert.True(t, results["version1"], "version1 should be found")
		assert.False(t, results["nonexistent"], "nonexistent should not be found")
		assert.True(t, results["version3"], "version3 should be found")
	})
}
