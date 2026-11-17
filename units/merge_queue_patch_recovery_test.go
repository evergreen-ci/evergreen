package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/google/go-github/v70/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirstParentSHA(t *testing.T) {
	t.Run("ReturnsFirstParentSHA", func(t *testing.T) {
		commit := &github.RepositoryCommit{Parents: []*github.Commit{
			{SHA: github.Ptr("base-sha")},
			{SHA: github.Ptr("second-parent")},
		}}
		assert.Equal(t, "base-sha", firstParentSHA(commit))
	})
	t.Run("NilCommitReturnsEmpty", func(t *testing.T) {
		assert.Empty(t, firstParentSHA(nil))
	})
	t.Run("NoParentsReturnsEmpty", func(t *testing.T) {
		assert.Empty(t, firstParentSHA(&github.RepositoryCommit{}))
	})
}

func TestNewMergeGroupEvent(t *testing.T) {
	headCommit := &github.RepositoryCommit{Commit: &github.Commit{Message: github.Ptr("merge commit")}}
	event := newMergeGroupEvent("evergreen-ci", "evergreen", "refs/heads/gh-readonly-queue/main/pr-1-abc", "head-sha", "base-sha", headCommit)

	// These are the fields NewGithubMergeIntent reads and validates, so they must
	// all be populated for the reconstructed event to be accepted.
	assert.Equal(t, "evergreen-ci", event.GetOrg().GetLogin())
	assert.Equal(t, "evergreen", event.GetRepo().GetName())
	require.NotNil(t, event.GetMergeGroup())
	assert.Equal(t, "refs/heads/gh-readonly-queue/main/pr-1-abc", event.GetMergeGroup().GetHeadRef())
	assert.Equal(t, "head-sha", event.GetMergeGroup().GetHeadSHA())
	assert.Equal(t, "base-sha", event.GetMergeGroup().GetBaseSHA())
	assert.Equal(t, "merge commit", event.GetMergeGroup().GetHeadCommit().GetMessage())
}

func TestNewMergeGroupEventNilHeadCommit(t *testing.T) {
	event := newMergeGroupEvent("evergreen-ci", "evergreen", "refs/heads/gh-readonly-queue/main/pr-1-abc", "head-sha", "base-sha", nil)
	assert.Nil(t, event.GetMergeGroup().HeadCommit)
	assert.Equal(t, "head-sha", event.GetMergeGroup().GetHeadSHA())
}

func TestMergeGroupStagedAt(t *testing.T) {
	committerTime := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	authorTime := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	t.Run("PrefersCommitterDate", func(t *testing.T) {
		commit := &github.RepositoryCommit{Commit: &github.Commit{
			Committer: &github.CommitAuthor{Date: &github.Timestamp{Time: committerTime}},
			Author:    &github.CommitAuthor{Date: &github.Timestamp{Time: authorTime}},
		}}
		assert.Equal(t, committerTime, mergeGroupStagedAt(commit))
	})
	t.Run("FallsBackToAuthorDateWhenNoCommitterDate", func(t *testing.T) {
		commit := &github.RepositoryCommit{Commit: &github.Commit{
			Author: &github.CommitAuthor{Date: &github.Timestamp{Time: authorTime}},
		}}
		assert.Equal(t, authorTime, mergeGroupStagedAt(commit))
	})
	t.Run("NilCommitReturnsZero", func(t *testing.T) {
		assert.True(t, mergeGroupStagedAt(nil).IsZero())
		assert.True(t, mergeGroupStagedAt(&github.RepositoryCommit{}).IsZero())
	})
}

func TestIsMergeIntentInFlight(t *testing.T) {
	now := time.Now()
	oldIntent := patch.UnprocessedMergeIntent{
		ID:        "intent",
		HeadSHA:   "sha",
		CreatedAt: now.Add(-2 * pendingIntentMaxAge),
	}

	makeQueue := func(t *testing.T) amboy.Queue {
		q := queue.NewLocalLimitedSize(1, 10)
		// Use a no-op runner so queued jobs stay pending.
		runner := pool.NewNoop()
		require.NoError(t, runner.SetQueue(q))
		require.NoError(t, q.SetRunner(runner))
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		require.NoError(t, q.Start(ctx))
		return q
	}
	makeProcessor := func(intentID string) *patchIntentProcessor {
		j := makePatchIntentProcessor()
		j.SetID(patchIntentProcessorJobID(patch.GithubMergeIntentType, intentID))
		return j
	}

	t.Run("RecentIntentWithNoJobIsInFlight", func(t *testing.T) {
		intent := patch.UnprocessedMergeIntent{ID: "intent", HeadSHA: "sha", CreatedAt: now.Add(-time.Minute)}
		assert.True(t, isMergeIntentInFlight(t.Context(), makeQueue(t), intent, now))
	})
	t.Run("OldIntentWithQueuedJobIsInFlight", func(t *testing.T) {
		q := makeQueue(t)
		require.NoError(t, q.Put(t.Context(), makeProcessor(oldIntent.ID)))
		assert.True(t, isMergeIntentInFlight(t.Context(), q, oldIntent, now))
	})
	t.Run("OldIntentWithNoJobIsStale", func(t *testing.T) {
		assert.False(t, isMergeIntentInFlight(t.Context(), makeQueue(t), oldIntent, now))
	})
	t.Run("OldIntentWithCompletedJobIsStale", func(t *testing.T) {
		q := makeQueue(t)
		j := makeProcessor(oldIntent.ID)
		require.NoError(t, q.Put(t.Context(), j))
		j.MarkComplete()
		assert.False(t, isMergeIntentInFlight(t.Context(), q, oldIntent, now))
	})
	t.Run("OldIntentWithJobForDifferentIntentIsStale", func(t *testing.T) {
		q := makeQueue(t)
		require.NoError(t, q.Put(t.Context(), makeProcessor("other_intent")))
		assert.False(t, isMergeIntentInFlight(t.Context(), q, oldIntent, now))
	})
}
