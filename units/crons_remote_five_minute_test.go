package units

import (
	"context"
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
	require.NoError(t, db.Insert(t.Context(), patch.GitHubStatusResyncCollection, patch.GitHubStatusResync{
		PatchID:      "patch",
		DirtySince:   now,
		LastFailedAt: now,
	}))
	ts := time.Date(2026, 11, 9, 12, 5, 0, 0, time.UTC)
	enabled := evergreen.ServiceFlags{GitHubStatusResyncEnabled: true}
	require.NoError(t, evergreen.SetServiceFlags(t.Context(), enabled))

	t.Run("ReturnsResyncJobForMarkedPatch", func(t *testing.T) {
		jobs, err := githubStatusResyncJobs(t.Context(), ts)
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		job, ok := jobs[0].(*githubStatusRefreshJob)
		require.True(t, ok)
		assert.Equal(t, "patch", job.FetchID)
		assert.True(t, job.IsResync)
	})
	for tName, flags := range map[string]evergreen.ServiceFlags{
		"ResyncNotEnabledReturnsNoJobs":  {},
		"StatusAPIDisabledReturnsNoJobs": {GitHubStatusResyncEnabled: true, GithubStatusAPIDisabled: true},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, evergreen.SetServiceFlags(t.Context(), flags))
			t.Cleanup(func() {
				assert.NoError(t, evergreen.SetServiceFlags(context.Background(), enabled))
			})

			jobs, err := githubStatusResyncJobs(t.Context(), ts)
			require.NoError(t, err)
			assert.Empty(t, jobs)
		})
	}
}
