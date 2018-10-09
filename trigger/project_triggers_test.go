package trigger

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func init() {
	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
}

func TestRevisionFromVersion(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(user.Collection))
	author := user.DBUser{
		Id: "me",
		Settings: user.UserSettings{
			GithubUser: user.GithubUser{
				UID: 123,
			},
		},
	}
	assert.NoError(author.Insert())
	source := version.Version{
		Author:     "name",
		CreateTime: time.Now(),
		Revision:   "abc",
		AuthorID:   "me",
	}

	rev, err := revisionFromVersion(source)
	assert.NoError(err)
	assert.Equal(source.Author, rev.Author)
	assert.Equal(source.CreateTime, rev.CreateTime)
	assert.Equal(source.Revision, rev.Revision)
	assert.Equal(123, rev.AuthorGithubUID)
}

func TestMakeDownstreamConfigFromFile(t *testing.T) {
	assert := assert.New(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestMakeDownstreamConfigFromFile")
	assert.NoError(db.ClearCollections(evergreen.ConfigCollection))
	assert.NoError(testConfig.Set())

	ref := model.ProjectRef{
		Identifier: "myProj",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
	}
	proj, err := makeDownstreamConfigFromFile(ref, "trigger/testdata/downstream_config.yml")
	assert.NoError(err)
	assert.Equal(ref.Identifier, proj.Identifier)
	assert.Len(proj.Tasks, 2)
	assert.Equal("task1", proj.Tasks[0].Name)
	assert.Len(proj.BuildVariants, 1)
	assert.Equal("something", proj.BuildVariants[0].DisplayName)
}
