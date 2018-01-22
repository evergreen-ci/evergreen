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

func TestValidateProjectRef(t *testing.T) {
	assert := assert.New(t) //nolint

	assert.NoError(db.Clear(model.ProjectRefCollection))

	doc := &model.ProjectRef{
		Identifier: "hi",
		Owner:      "baxterthehacker",
		Repo:       "public-repo",
		Branch:     "changes",
	}

	ref, err := validateProjectRef("baxterthehacker", "public-repo")
	assert.Error(err)
	assert.Equal("can't find project ref", err.Error())
	assert.Nil(ref)
	assert.NoError(doc.Insert())

	ref, err = validateProjectRef("baxterthehacker", "public-repo")
	assert.NoError(err)
	assert.NotNil(ref)
}
