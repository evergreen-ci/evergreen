package trigger

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestMetadataFromArgsWithVersion(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(user.Collection, model.RepositoriesCollection))
	defer func() {
		assert.NoError(db.ClearCollections(user.Collection, model.RepositoriesCollection))
	}()
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

	args := ProcessorArgs{
		SourceVersion:     &source,
		DownstreamProject: ref,
		TriggerType:       model.ProjectTriggerLevelPush,
	}
	// Without updating the repositories collection, this errors.
	_, err := getMetadataFromArgs(args)
	assert.Error(err)

	_, err = model.GetNewRevisionOrderNumber(ref.Id)
	assert.NoError(err)
	assert.NoError(model.UpdateLastRevision(ref.Id, "def"))

	metadata, err := getMetadataFromArgs(args)
	assert.NoError(err)
	assert.Equal(source.Author, metadata.Revision.Author)
	assert.Equal(source.CreateTime, metadata.Revision.CreateTime)
	assert.Equal("def", metadata.Revision.Revision)
	assert.Equal(123, metadata.Revision.AuthorGithubUID)
	assert.Empty(metadata.SourceCommit)
	assert.True(metadata.Activate)
}

func TestMetadataFromArgsWithoutVersion(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(model.RepositoriesCollection))
	ref := model.ProjectRef{
		Id: "project",
	}
	args := ProcessorArgs{
		TriggerType:       model.ProjectTriggerLevelPush,
		DownstreamProject: ref,
		PushRevision: model.Revision{
			Revision: "1234",
			Author:   "me",
		},
	}

	// Without updating the repositories collection, this errors.
	_, err := getMetadataFromArgs(args)
	assert.Error(err)

	_, err = model.GetNewRevisionOrderNumber(ref.Id)
	assert.NoError(err)
	assert.NoError(model.UpdateLastRevision(ref.Id, "def"))
	metadata, err := getMetadataFromArgs(args)
	assert.NoError(err)
	assert.True(metadata.Activate)
	assert.Equal(args.TriggerType, metadata.TriggerType)
	// Should equal the downstream project's ref rather than the push revision.
	assert.Equal("def", metadata.Revision.Revision)
	assert.Equal(args.PushRevision.Revision, metadata.SourceCommit)
}

func TestMakeDownstreamConfigFromFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestMakeDownstreamConfigFromFile")
	assert.NoError(db.ClearCollections(evergreen.ConfigCollection))
	assert.NoError(testConfig.Set(ctx))

	ref := model.ProjectRef{
		Id:    "myProj",
		Owner: "evergreen-ci",
		Repo:  "evergreen",
	}
	projectInfo, err := makeDownstreamProjectFromFile(ctx, ref, "trigger/testdata/downstream_config.yml")
	assert.NoError(err)
	assert.NotNil(projectInfo.Project)
	assert.NotNil(projectInfo.IntermediateProject)
	assert.Equal(ref.Id, projectInfo.Project.Identifier)
	assert.Len(projectInfo.Project.Tasks, 2)
	assert.Equal("task1", projectInfo.Project.Tasks[0].Name)
	assert.Len(projectInfo.Project.BuildVariants, 1)
	assert.Equal("something", projectInfo.Project.BuildVariants[0].DisplayName)
}
