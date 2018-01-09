package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/rest"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
)

func TestValidatePushEvent(t *testing.T) {
	assert := assert.New(t) //nolint

	err := validatePushEvent(nil)
	assert.Error(err)
	assert.IsType(new(rest.APIError), err)

	event := github.PushEvent{}
	err = validatePushEvent(&event)
	assert.Error(err)
	assert.IsType(new(rest.APIError), err)

	event.Ref = github.String("ref/heads/changes")
	event.Repo = &github.PushEventRepository{}
	event.Repo.Name = github.String("public-repo")
	event.Repo.Owner = &github.PushEventRepoOwner{}
	event.Repo.Owner.Name = github.String("baxterthehacker")
	event.Repo.FullName = github.String("baxterthehacker/public-repo")

	err = validatePushEvent(&event)
	assert.Nil(err)
}
