package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
)

func init() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func TestValidatePushEvent(t *testing.T) {
	assert := assert.New(t) //nolint

	branch, err := validatePushEvent(nil)
	assert.Error(err)
	assert.IsType(new(rest.APIError), err)
	assert.Empty(branch)

	event := github.PushEvent{}
	branch, err = validatePushEvent(&event)
	assert.Error(err)
	assert.IsType(new(rest.APIError), err)
	assert.Empty(branch)

	event.Ref = github.String("refs/heads/changes")
	event.Repo = &github.PushEventRepository{}
	event.Repo.Name = github.String("public-repo")
	event.Repo.Owner = &github.PushEventRepoOwner{}
	event.Repo.Owner.Name = github.String("baxterthehacker")
	event.Repo.FullName = github.String("baxterthehacker/public-repo")

	branch, err = validatePushEvent(&event)
	assert.NoError(err)
	assert.Equal("changes", branch)

	event.Ref = github.String("refs/tags/v9001")
	branch, err = validatePushEvent(&event)
	assert.NoError(err)
	assert.Empty(branch)
}

func TestValidateProjectRef(t *testing.T) {
	assert := assert.New(t) //nolint

	assert.NoError(db.Clear(model.ProjectRefCollection))

	doc := &model.ProjectRef{
		Identifier: "hi",
		Owner:      "baxterthehacker",
		Repo:       "public-repo",
		Branch:     "changes",
		Enabled:    true,
	}

	ref, err := validateProjectRef("baxterthehacker", "public-repo", "changes")
	assert.Error(err)
	assert.Equal("can't find project ref", err.Error())
	assert.Nil(ref)
	assert.NoError(doc.Insert())

	ref, err = validateProjectRef("baxterthehacker", "public-repo", "changes")
	assert.NoError(err)
	assert.NotNil(ref)
}
