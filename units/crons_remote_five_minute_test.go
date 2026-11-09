package units

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubStatusResyncJobs(t *testing.T) {
	require.NoError(t, db.ClearCollections(patch.GitHubStatusResyncCollection, evergreen.ConfigCollection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(patch.GitHubStatusResyncCollection, evergreen.ConfigCollection))
	})
	now := time.Now()
	for i := range githubStatusResyncBatchSize {
		require.NoError(t, db.Insert(t.Context(), patch.GitHubStatusResyncCollection, patch.GitHubStatusResync{
			PatchID:      fmt.Sprintf("pr-%d", i),
			DirtySince:   now.Add(-time.Hour),
			LastFailedAt: now,
		}))
	}
	require.NoError(t, db.Insert(t.Context(), patch.GitHubStatusResyncCollection, patch.GitHubStatusResync{
		PatchID:      "merge-queue",
		IsMergeQueue: true,
		DirtySince:   now,
		LastFailedAt: now,
	}))
	ts := time.Date(2026, 11, 9, 12, 5, 0, 0, time.UTC)

	t.Run("ReturnsDuePatchesUpToCapWithMergeQueueFirst", func(t *testing.T) {
		jobs, err := githubStatusResyncJobs(t.Context(), ts)
		require.NoError(t, err)
		require.Len(t, jobs, githubStatusResyncBatchSize)
		job, ok := jobs[0].(*githubStatusRefreshJob)
		require.True(t, ok)
		assert.Equal(t, "merge-queue", job.FetchID)
	})
	t.Run("JobIDsAreStableWithinARun", func(t *testing.T) {
		jobs, err := githubStatusResyncJobs(t.Context(), ts)
		require.NoError(t, err)
		require.NotEmpty(t, jobs)
		sameRunJobs, err := githubStatusResyncJobs(t.Context(), ts)
		require.NoError(t, err)
		require.NotEmpty(t, sameRunJobs)
		nextRunJobs, err := githubStatusResyncJobs(t.Context(), ts.Add(5*time.Minute))
		require.NoError(t, err)
		require.NotEmpty(t, nextRunJobs)

		assert.Equal(t, jobs[0].ID(), sameRunJobs[0].ID())
		assert.NotEqual(t, jobs[0].ID(), nextRunJobs[0].ID())
	})
	for tName, flags := range map[string]evergreen.ServiceFlags{
		"ResyncDisabledReturnsNoJobs":    {GithubStatusResyncDisabled: true},
		"StatusAPIDisabledReturnsNoJobs": {GithubStatusAPIDisabled: true},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, evergreen.SetServiceFlags(t.Context(), flags))
			t.Cleanup(func() {
				assert.NoError(t, evergreen.SetServiceFlags(context.Background(), evergreen.ServiceFlags{}))
			})

			jobs, err := githubStatusResyncJobs(t.Context(), ts)
			require.NoError(t, err)
			assert.Empty(t, jobs)
		})
	}
}
