package data

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v70/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePushEvent(t *testing.T) {
	assert := assert.New(t)

	branch, err := validatePushEvent(nil)
	assert.Error(err)
	assert.IsType(gimlet.ErrorResponse{}, err)
	assert.Empty(branch)

	event := github.PushEvent{}
	branch, err = validatePushEvent(&event)
	assert.Error(err)
	assert.IsType(gimlet.ErrorResponse{}, err)
	assert.Empty(branch)

	event.Ref = github.String("refs/heads/changes")
	event.Repo = &github.PushEventRepository{}
	event.Repo.Name = github.String("public-repo")
	event.Repo.Owner = &github.User{}
	event.Repo.Owner.Name = github.String("baxterthehacker")
	event.Repo.FullName = github.String("baxterthehacker/public-repo")

	branch, err = validatePushEvent(&event)
	assert.NoError(err)
	assert.Equal("changes", branch)

	event.Ref = github.String("refs/tags/v9001")
	branch, err = validatePushEvent(&event)
	assert.NoError(err)
	assert.Empty(branch)

	event = github.PushEvent{}
	branch, err = validatePushEvent(&event)
	assert.Error(err)
	assert.IsType(gimlet.ErrorResponse{}, err)
	assert.Empty(branch)

	event.Ref = github.String("refs/heads/support/3.x")
	event.Repo = &github.PushEventRepository{}
	event.Repo.Name = github.String("public-repo")
	event.Repo.Owner = &github.User{}
	event.Repo.Owner.Name = github.String("baxterthehacker")
	event.Repo.FullName = github.String("baxterthehacker/public-repo")

	branch, err = validatePushEvent(&event)
	assert.NoError(err)
	assert.Equal("support/3.x", branch)

	event.Ref = github.String("refs/tags/v9001")
	branch, err = validatePushEvent(&event)
	assert.NoError(err)
	assert.Empty(branch)
}

func TestUpsertRepositoryRevisionsFromPushEvent(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.RepositoryRevisionsHistoryCollection))
	ingestTime := time.Now()
	event := &github.PushEvent{
		Commits: []*github.HeadCommit{
			{ID: utility.ToStringPtr("revision-1")},
			{ID: utility.ToStringPtr("revision-2")},
			{},
		},
	}

	require.NoError(t, upsertRepositoryRevisionsFromPushEvent(t.Context(), "project", event, ingestTime))
	revision, err := model.FindLatestRepositoryRevisionByIngestTime(t.Context(), "project", ingestTime)
	require.NoError(t, err)
	require.NotNil(t, revision)
	assert.Equal(t, "revision-2", revision.Revision)
	assert.WithinDuration(t, ingestTime, revision.IngestTime, time.Millisecond)
	assert.Equal(t, 1, revision.Order)
}
