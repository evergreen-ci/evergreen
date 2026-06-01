package units

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/google/go-github/v70/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeQueueCompletionMetricsFallbackJobSkipsPatchFinishedLessThan5MinAgo(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, db.ClearCollections(patch.Collection)) })

	p := patch.Patch{
		Id:         mgobson.NewObjectId(),
		Project:    "my-project",
		Alias:      evergreen.CommitQueueAlias,
		Status:     evergreen.VersionSucceeded,
		CreateTime: time.Now(),
		FinishTime: time.Now().Add(-2 * time.Minute),
	}
	require.NoError(t, db.Insert(t.Context(), patch.Collection, p))

	j := &mergeQueueCompletionMetricsFallbackJob{}
	j.emitCompletionMetricsForPatch(t.Context(), &p)

	updated, err := patch.FindOneId(t.Context(), p.Id.Hex())
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Empty(t, updated.MergeQueueMetricsEmitStatus)
}

func TestMergeQueueCompletionMetricsFallbackJobSkipsWhenGitHubPRFetchFails(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, db.ClearCollections(patch.Collection)) })

	p := patch.Patch{
		Id:         mgobson.NewObjectId(),
		Project:    "my-project",
		Alias:      evergreen.CommitQueueAlias,
		Status:     evergreen.VersionSucceeded,
		CreateTime: time.Now(),
		FinishTime: time.Now().Add(-10 * time.Minute),
	}
	require.NoError(t, db.Insert(t.Context(), patch.Collection, p))

	j := &mergeQueueCompletionMetricsFallbackJob{}
	j.emitCompletionMetricsForPatch(t.Context(), &p)

	updated, err := patch.FindOneId(t.Context(), p.Id.Hex())
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Empty(t, updated.MergeQueueMetricsEmitStatus)
}

func TestMergeQueueCompletionMetricsFallbackJobPreservesSuccessStatusOnGetPullRequestFailure(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, db.ClearCollections(patch.Collection)) })

	p := patch.Patch{
		Id:                          mgobson.NewObjectId(),
		Project:                     "my-project",
		Alias:                       evergreen.CommitQueueAlias,
		Status:                      evergreen.VersionSucceeded,
		CreateTime:                  time.Now(),
		FinishTime:                  time.Now().Add(-10 * time.Minute),
		MergeQueueMetricsEmitStatus: patch.MergeQueueMetricsEmitStatusSuccess,
	}
	require.NoError(t, db.Insert(t.Context(), patch.Collection, p))

	j := &mergeQueueCompletionMetricsFallbackJob{}
	j.emitCompletionMetricsForPatch(t.Context(), &p)

	updated, err := patch.FindOneId(t.Context(), p.Id.Hex())
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, patch.MergeQueueMetricsEmitStatusSuccess, updated.MergeQueueMetricsEmitStatus)
}

func TestMergeQueueEndTimeFromPRMergedUsesMergedAt(t *testing.T) {
	mergedAt := time.Now().Add(-10 * time.Minute)
	collectiveFinishTime := time.Now().Add(-15 * time.Minute)

	merged := true
	state := "closed"
	ts := github.Timestamp{Time: mergedAt}
	pr := &github.PullRequest{Merged: &merged, State: &state, MergedAt: &ts}

	endTime, source, ok := mergeQueueEndTimeFromPR(pr, collectiveFinishTime)
	require.True(t, ok)
	assert.Equal(t, mergedAt, endTime)
	assert.Equal(t, patch.MergeQueueEndTimeSourceGitHubPolling, source)
}

func TestMergeQueueEndTimeFromPRClosedNotMergedUsesClosedAt(t *testing.T) {
	closedAt := time.Now().Add(-5 * time.Minute)
	collectiveFinishTime := time.Now().Add(-15 * time.Minute)

	merged := false
	state := "closed"
	ts := github.Timestamp{Time: closedAt}
	pr := &github.PullRequest{Merged: &merged, State: &state, ClosedAt: &ts}

	endTime, source, ok := mergeQueueEndTimeFromPR(pr, collectiveFinishTime)
	require.True(t, ok)
	assert.Equal(t, closedAt, endTime)
	assert.Equal(t, patch.MergeQueueEndTimeSourceGitHubPolling, source)
}

func TestMergeQueueEndTimeFromPRClosedNotMergedWithZeroClosedAtUsesCollectiveFinish(t *testing.T) {
	collectiveFinishTime := time.Now().Add(-5 * time.Minute)

	merged := false
	state := "closed"
	pr := &github.PullRequest{Merged: &merged, State: &state}

	endTime, source, ok := mergeQueueEndTimeFromPR(pr, collectiveFinishTime)
	require.True(t, ok)
	assert.Equal(t, collectiveFinishTime, endTime)
	assert.Equal(t, patch.MergeQueueEndTimeSourceCollectiveFinish, source)
}

func TestMergeQueueEndTimeFromPRDraftUsesCollectiveFinish(t *testing.T) {
	collectiveFinishTime := time.Now().Add(-10 * time.Minute)

	merged := false
	state := "open"
	draft := true
	pr := &github.PullRequest{Merged: &merged, State: &state, Draft: &draft}

	endTime, source, ok := mergeQueueEndTimeFromPR(pr, collectiveFinishTime)
	require.True(t, ok)
	assert.Equal(t, collectiveFinishTime, endTime)
	assert.Equal(t, patch.MergeQueueEndTimeSourceCollectiveFinish, source)
}

func TestMergeQueueEndTimeFromPROpenSkips(t *testing.T) {
	collectiveFinishTime := time.Now().Add(-10 * time.Minute)

	merged := false
	state := "open"
	draft := false
	pr := &github.PullRequest{Merged: &merged, State: &state, Draft: &draft}

	_, _, ok := mergeQueueEndTimeFromPR(pr, collectiveFinishTime)
	assert.False(t, ok)
}
