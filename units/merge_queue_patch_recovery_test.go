package units

import (
	"testing"
	"time"

	"github.com/google/go-github/v70/github"
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
