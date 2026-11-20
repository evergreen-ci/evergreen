package patch

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkGitHubStatusResync(t *testing.T) {
	require.NoError(t, db.ClearCollections(GitHubStatusResyncCollection))
	dirtySince := time.Now().Add(-time.Hour).Truncate(time.Millisecond)
	require.NoError(t, db.Insert(t.Context(), GitHubStatusResyncCollection, GitHubStatusResync{
		PatchID:      "existing",
		DirtySince:   dirtySince,
		LastFailedAt: dirtySince,
	}))

	require.NoError(t, MarkGitHubStatusResync(t.Context(), "existing", true))
	require.NoError(t, MarkGitHubStatusResync(t.Context(), "new", false))

	existing, err := FindGitHubStatusResync(t.Context(), "existing")
	require.NoError(t, err)
	require.NotNil(t, existing)
	assert.True(t, existing.IsMergeQueue)
	assert.True(t, dirtySince.Equal(existing.DirtySince))
	assert.WithinDuration(t, time.Now(), existing.LastFailedAt, time.Minute)

	created, err := FindGitHubStatusResync(t.Context(), "new")
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.False(t, created.IsMergeQueue)
	assert.WithinDuration(t, time.Now(), created.DirtySince, time.Minute)
	assert.True(t, created.DirtySince.Equal(created.LastFailedAt))
}

func TestFindDueGitHubStatusResyncs(t *testing.T) {
	require.NoError(t, db.ClearCollections(GitHubStatusResyncCollection))
	now := time.Now()
	for _, resync := range []GitHubStatusResync{
		{PatchID: "old-pr", DirtySince: now.Add(-3 * time.Hour), LastFailedAt: now.Add(-time.Minute)},
		{PatchID: "new-pr", DirtySince: now.Add(-time.Hour), LastFailedAt: now.Add(-time.Minute)},
		{PatchID: "merge-queue", IsMergeQueue: true, DirtySince: now.Add(-time.Minute), LastFailedAt: now.Add(-time.Minute)},
		{PatchID: "expired", DirtySince: now.Add(-8 * time.Hour), LastFailedAt: now.Add(-7 * time.Hour)},
	} {
		require.NoError(t, db.Insert(t.Context(), GitHubStatusResyncCollection, resync))
	}

	resyncs, err := FindDueGitHubStatusResyncs(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, resyncs, 3)
	assert.Equal(t, "merge-queue", resyncs[0].PatchID)
	assert.Equal(t, "old-pr", resyncs[1].PatchID)
	assert.Equal(t, "new-pr", resyncs[2].PatchID)

	resyncs, err = FindDueGitHubStatusResyncs(t.Context(), 2)
	require.NoError(t, err)
	require.Len(t, resyncs, 2)
	assert.Equal(t, "merge-queue", resyncs[0].PatchID)
	assert.Equal(t, "old-pr", resyncs[1].PatchID)
}

func TestClearGitHubStatusResync(t *testing.T) {
	require.NoError(t, db.ClearCollections(GitHubStatusResyncCollection))
	jobStart := time.Now().Truncate(time.Millisecond)
	require.NoError(t, db.Insert(t.Context(), GitHubStatusResyncCollection, GitHubStatusResync{
		PatchID:      "failed-before",
		LastFailedAt: jobStart.Add(-time.Minute),
	}))
	require.NoError(t, db.Insert(t.Context(), GitHubStatusResyncCollection, GitHubStatusResync{
		PatchID:      "failed-during",
		LastFailedAt: jobStart,
	}))

	require.NoError(t, ClearGitHubStatusResync(t.Context(), "failed-before", jobStart))
	require.NoError(t, ClearGitHubStatusResync(t.Context(), "failed-during", jobStart))
	require.NoError(t, ClearGitHubStatusResync(t.Context(), "not-marked", jobStart))

	failedBefore, err := FindGitHubStatusResync(t.Context(), "failed-before")
	require.NoError(t, err)
	assert.Nil(t, failedBefore)
	failedDuring, err := FindGitHubStatusResync(t.Context(), "failed-during")
	require.NoError(t, err)
	assert.NotNil(t, failedDuring)
}
