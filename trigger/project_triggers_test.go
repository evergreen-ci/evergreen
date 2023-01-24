package trigger

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestMetadataFromVersion(t *testing.T) {
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
	source := model.Version{
		Author:     "name",
		CreateTime: time.Now(),
		Revision:   "abc",
		AuthorID:   "me",
	}
	ref := model.ProjectRef{
		Id: "project",
	}
	_, err := model.GetNewRevisionOrderNumber(ref.Id)
	assert.NoError(err)
	assert.NoError(model.UpdateLastRevision(ref.Id, "def"))

	args := ProcessorArgs{
		SourceVersion:     &source,
		DownstreamProject: ref,
	}
	metadata, err := metadataFromVersion(args)
	assert.NoError(err)
	assert.Equal(source.Author, metadata.Revision.Author)
	assert.Equal(source.CreateTime, metadata.Revision.CreateTime)
	assert.Equal("def", metadata.Revision.Revision)
	assert.Equal(123, metadata.Revision.AuthorGithubUID)
	assert.True(metadata.Activate)
}

func TestMakeDownstreamConfigFromFile(t *testing.T) {
	assert := assert.New(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestMakeDownstreamConfigFromFile")
	assert.NoError(db.ClearCollections(evergreen.ConfigCollection))
	assert.NoError(testConfig.Set())

	ref := model.ProjectRef{
		Id:    "myProj",
		Owner: "evergreen-ci",
		Repo:  "evergreen",
	}
	projectInfo, err := makeDownstreamProjectFromFile(ref, "trigger/testdata/downstream_config.yml")
	assert.NoError(err)
	assert.NotNil(projectInfo.Project)
	assert.NotNil(projectInfo.IntermediateProject)
	assert.Equal(ref.Id, projectInfo.Project.Identifier)
	assert.Len(projectInfo.Project.Tasks, 2)
	assert.Equal("task1", projectInfo.Project.Tasks[0].Name)
	assert.Len(projectInfo.Project.BuildVariants, 1)
	assert.Equal("something", projectInfo.Project.BuildVariants[0].DisplayName)
}
