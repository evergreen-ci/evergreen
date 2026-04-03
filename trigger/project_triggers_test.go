package trigger

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/google/go-github/v70/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMetadataFromArgs(t *testing.T) {
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig)
	require.NoError(t, testConfig.Set(t.Context()))

	until := time.Date(2015, time.January, 1, 0, 0, 0, 0, time.UTC)
	commits, _, err := thirdparty.GetGithubCommits(t.Context(), "evergreen-ci", "sample", &github.CommitsListOptions{
		SHA:   "",
		Until: until,
		ListOptions: github.ListOptions{
			Page: 0, PerPage: 1,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, commits)
	wantSHA := commits[0].GetSHA()

	t.Run("WithSourceVersion", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, db.ClearCollections(user.Collection, model.RepositoriesCollection))
		t.Cleanup(func() {
			require.NoError(t, db.ClearCollections(user.Collection, model.RepositoriesCollection))
		})
		downstreamID := "project-triggers-md-src"
		_, err := model.GetNewRevisionOrderNumber(ctx, downstreamID)
		require.NoError(t, err)
		require.NoError(t, model.UpdateLastRevision(ctx, downstreamID, "bootstrap"))
		author := user.DBUser{Id: "md-src-author", Settings: user.UserSettings{GithubUser: user.GithubUser{UID: 123}}}
		require.NoError(t, author.Insert(ctx))
		source := model.Version{Author: "a", CreateTime: until, AuthorID: author.Id, Message: "m"}
		meta, err := getMetadataFromArgs(ctx, ProcessorArgs{
			SourceVersion:     &source,
			DownstreamProject: model.ProjectRef{Id: downstreamID, Owner: "evergreen-ci", Repo: "sample"},
			TriggerType:       model.ProjectTriggerLevelTask,
		})
		require.NoError(t, err)
		assert.Equal(t, wantSHA, meta.Revision.Revision)
		assert.Equal(t, 123, meta.Revision.AuthorGithubUID)
		assert.Empty(t, meta.SourceCommit)
	})

	t.Run("WithPushRevision", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, db.ClearCollections(user.Collection, model.RepositoriesCollection))
		t.Cleanup(func() {
			require.NoError(t, db.ClearCollections(user.Collection, model.RepositoriesCollection))
		})
		downstreamID := "project-triggers-md-push"
		_, err := model.GetNewRevisionOrderNumber(ctx, downstreamID)
		require.NoError(t, err)
		require.NoError(t, model.UpdateLastRevision(ctx, downstreamID, "bootstrap"))
		push := model.Revision{Revision: "upstream", CreateTime: until, Author: "p"}
		meta, err := getMetadataFromArgs(ctx, ProcessorArgs{
			TriggerType:       model.ProjectTriggerLevelPush,
			DownstreamProject: model.ProjectRef{Id: downstreamID, Owner: "evergreen-ci", Repo: "sample"},
			PushRevision:      push,
		})
		require.NoError(t, err)
		assert.Equal(t, wantSHA, meta.Revision.Revision)
		assert.Equal(t, push.Revision, meta.SourceCommit)
	})
}

func TestMakeDownstreamProjectFromFile(t *testing.T) {
	require.NoError(t, db.ClearCollections(evergreen.ConfigCollection))

	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig)
	require.NoError(t, testConfig.Set(t.Context()))

	ref := model.ProjectRef{
		Id:     "myProj",
		Owner:  "evergreen-ci",
		Repo:   "evergreen",
		Branch: "main",
	}
	projectInfo, err := makeDownstreamProjectFromFile(t.Context(), ref, "trigger/testdata/downstream_config.yml")
	require.NoError(t, err)
	require.NotNil(t, projectInfo.Project)
	require.NotNil(t, projectInfo.IntermediateProject)
	assert.Equal(t, ref.Id, projectInfo.Project.Identifier)
	assert.Len(t, projectInfo.Project.Tasks, 2)
	assert.Equal(t, "task1", projectInfo.Project.Tasks[0].Name)
	assert.Len(t, projectInfo.Project.BuildVariants, 1)
	assert.Equal(t, "something", projectInfo.Project.BuildVariants[0].DisplayName)
}
