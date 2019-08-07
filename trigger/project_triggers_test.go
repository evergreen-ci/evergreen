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
		Identifier: "project",
	}
	_, err := model.GetNewRevisionOrderNumber(ref.Identifier)
	assert.NoError(err)
	assert.NoError(model.UpdateLastRevision(ref.Identifier, "def"))

	metadata, err := metadataFromVersion(source, ref)
	assert.NoError(err)
	assert.Equal(source.Author, metadata.Revision.Author)
	assert.Equal(source.CreateTime, metadata.Revision.CreateTime)
	assert.Equal("def", metadata.Revision.Revision)
	assert.Equal(123, metadata.Revision.AuthorGithubUID)
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

func TestMakeDownstreamConfigFromCommand(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(evergreen.ConfigCollection))
	config := testutil.MockConfig()
	assert.NoError(evergreen.UpdateConfig(config))
	ref := model.ProjectRef{
		Identifier: "project",
	}
	cmd := "echo hi"

	project, err := makeDownstreamConfigFromCommand(ref, cmd, "generate.json")
	assert.NoError(err)
	assert.Equal(ref.Identifier, project.Identifier)
	foundCommand := false
	foundFile := false
	for _, t := range project.Tasks {
		for _, c := range t.Commands {
			if c.Command == "subprocess.exec" {
				foundCommand = true
			} else if c.Command == "generate.tasks" {
				for _, value := range c.Params {
					assert.Contains(value, "generate.json")
					foundFile = true
				}
			}
		}
	}
	assert.True(foundCommand)
	assert.True(foundFile)
}
