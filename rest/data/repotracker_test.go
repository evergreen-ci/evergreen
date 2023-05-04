package data

import (
	"testing"

	"github.com/evergreen-ci/gimlet"
	"github.com/google/go-github/v52/github"
	"github.com/stretchr/testify/assert"
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
