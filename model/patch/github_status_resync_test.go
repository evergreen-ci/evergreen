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
		Contexts:     []string{"evergreen/ubuntu"},
	}))

	require.NoError(t, MarkGitHubStatusResync(t.Context(), "existing", true, "evergreen"))
	require.NoError(t, MarkGitHubStatusResync(t.Context(), "existing", true, "evergreen"))
	require.NoError(t, MarkGitHubStatusResync(t.Context(), "new", false, "evergreen/ubuntu"))

	existing, err := FindGitHubStatusResync(t.Context(), "existing")
	require.NoError(t, err)
	require.NotNil(t, existing)
	assert.True(t, existing.IsMergeQueue)
	assert.True(t, dirtySince.Equal(existing.DirtySince))
	assert.WithinDuration(t, time.Now(), existing.LastFailedAt, time.Minute)
	assert.ElementsMatch(t, []string{"evergreen/ubuntu", "evergreen"}, existing.Contexts)

	created, err := FindGitHubStatusResync(t.Context(), "new")
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.False(t, created.IsMergeQueue)
	assert.WithinDuration(t, time.Now(), created.DirtySince, time.Minute)
	assert.True(t, created.DirtySince.Equal(created.LastFailedAt))
	assert.Equal(t, []string{"evergreen/ubuntu"}, created.Contexts)
}

func TestFindDueGitHubStatusResyncs(t *testing.T) {
	require.NoError(t, db.ClearCollections(GitHubStatusResyncCollection))
	now := time.Now()
	for _, resync := range []GitHubStatusResync{
		{PatchID: "attempted-pr", DirtySince: now.Add(-4 * time.Hour), LastFailedAt: now.Add(-time.Minute)},
		{PatchID: "old-pr", DirtySince: now.Add(-3 * time.Hour), LastFailedAt: now.Add(-time.Minute)},
		{PatchID: "new-pr", DirtySince: now.Add(-time.Hour), LastFailedAt: now.Add(-time.Minute)},
		{PatchID: "merge-queue", IsMergeQueue: true, DirtySince: now.Add(-time.Minute), LastFailedAt: now.Add(-time.Minute)},
		{PatchID: "expired", DirtySince: now.Add(-8 * time.Hour), LastFailedAt: now.Add(-7 * time.Hour)},
	} {
		require.NoError(t, db.Insert(t.Context(), GitHubStatusResyncCollection, resync))
	}
	require.NoError(t, MarkGitHubStatusResyncAttempted(t.Context(), "attempted-pr"))

	resyncs, err := FindDueGitHubStatusResyncs(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, resyncs, 4)
	assert.Equal(t, "merge-queue", resyncs[0].PatchID)
	assert.Equal(t, "old-pr", resyncs[1].PatchID)
	assert.Equal(t, "new-pr", resyncs[2].PatchID)
	assert.Equal(t, "attempted-pr", resyncs[3].PatchID)

	resyncs, err = FindDueGitHubStatusResyncs(t.Context(), 2)
	require.NoError(t, err)
	require.Len(t, resyncs, 2)
	assert.Equal(t, "merge-queue", resyncs[0].PatchID)
	assert.Equal(t, "old-pr", resyncs[1].PatchID)
}

func TestClearGitHubStatusResync(t *testing.T) {
	require.NoError(t, db.ClearCollections(GitHubStatusResyncCollection))
	lastFailedAt := time.Now().Truncate(time.Millisecond)
	for _, resync := range []GitHubStatusResync{
		{PatchID: "failed-before", LastFailedAt: lastFailedAt.Add(-time.Minute)},
		{PatchID: "failed-at", LastFailedAt: lastFailedAt},
		{PatchID: "failed-after", LastFailedAt: lastFailedAt.Add(time.Millisecond)},
	} {
		require.NoError(t, db.Insert(t.Context(), GitHubStatusResyncCollection, resync))
		require.NoError(t, ClearGitHubStatusResync(t.Context(), resync.PatchID, lastFailedAt))
	}
	require.NoError(t, ClearGitHubStatusResync(t.Context(), "not-marked", lastFailedAt))

	for patchID, shouldClear := range map[string]bool{"failed-before": true, "failed-at": true, "failed-after": false} {
		resync, err := FindGitHubStatusResync(t.Context(), patchID)
		require.NoError(t, err)
		assert.Equal(t, shouldClear, resync == nil, patchID)
	}
}
