package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v70/github"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestContextForQueuedJobPreservesValuesAfterRequestCancellation(t *testing.T) {
	type contextKey string
	const key contextKey = "key"

	requestCtx, cancel := context.WithCancel(context.WithValue(t.Context(), key, "value"))
	queueCtx := contextForQueuedJob(requestCtx)
	cancel()

	assert.NoError(t, queueCtx.Err())
	assert.Equal(t, "value", queueCtx.Value(key))
}

func TestAddGithubMergeIntentRemovesIntentWhenEnqueueFails(t *testing.T) {
	require.NoError(t, db.Clear(patch.IntentCollection))
	t.Cleanup(func() {
		assert.NoError(t, db.Clear(patch.IntentCollection))
	})

	intent, err := patch.NewGithubMergeIntent(t.Context(), "delivery_id", "auto", &github.MergeGroupEvent{
		MergeGroup: &github.MergeGroup{
			HeadSHA: utility.ToStringPtr("head_sha"),
			HeadRef: utility.ToStringPtr("refs/heads/gh-readonly-queue/main/pr-1-abc"),
			BaseSHA: utility.ToStringPtr("base_sha"),
		},
		Org:  &github.Organization{Login: utility.ToStringPtr("org")},
		Repo: &github.Repository{Name: utility.ToStringPtr("repo")},
	})
	require.NoError(t, err)

	// The queue is never started, so enqueueing the processor job fails.
	q := queue.NewLocalLimitedSize(1, 10)
	assert.Error(t, AddGithubMergeIntent(t.Context(), intent, "project", q))

	count, err := db.Count(t.Context(), patch.IntentCollection, bson.M{})
	require.NoError(t, err)
	assert.Zero(t, count)
}
