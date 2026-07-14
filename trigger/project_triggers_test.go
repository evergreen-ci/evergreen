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
	"github.com/google/go-github/v70/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMetadataFromArgs(t *testing.T) {
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig)
	require.NoError(t, testConfig.Set(t.Context()))

	t.Run("WithSourceVersion", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, db.ClearCollections(user.Collection, model.RepositoryRevisionsHistoryCollection))
		t.Cleanup(func() {
			require.NoError(t, db.ClearCollections(user.Collection, model.RepositoryRevisionsHistoryCollection))
		})
		ingestTime := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
		wantSHA := "downstream-at-source-ingest-time"
		downstream := model.ProjectRef{Id: "project-triggers-md-src", Owner: "evergreen-ci", Repo: "sample", Branch: "main"}
		require.NoError(t, model.UpsertRepositoryRevision(ctx, downstream.Owner, downstream.Repo, downstream.Branch, wantSHA, ingestTime))
		require.NoError(t, model.UpsertRepositoryRevision(ctx, downstream.Owner, downstream.Repo, downstream.Branch, "downstream-after-source-ingest-time", ingestTime.Add(time.Minute)))
		author := user.DBUser{Id: "md-src-author", Settings: user.UserSettings{GithubUser: user.GithubUser{UID: 123}}}
		require.NoError(t, author.Insert(ctx))
		source := model.Version{Author: "a", CreateTime: time.Date(2015, time.January, 1, 0, 0, 0, 0, time.UTC), IngestTime: ingestTime, AuthorID: author.Id, Message: "m"}
		meta, err := getMetadataFromArgs(ctx, ProcessorArgs{
			SourceVersion:     &source,
			DownstreamProject: downstream,
			TriggerType:       model.ProjectTriggerLevelTask,
		})
		require.NoError(t, err)
		assert.Equal(t, wantSHA, meta.Revision.Revision)
		assert.Equal(t, ingestTime, meta.IngestTime)
		assert.Equal(t, 123, meta.Revision.AuthorGithubUID)
		assert.Empty(t, meta.SourceCommit)
	})

	t.Run("WithPushRevision", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, db.ClearCollections(model.RepositoryRevisionsHistoryCollection))
		t.Cleanup(func() {
			require.NoError(t, db.ClearCollections(model.RepositoryRevisionsHistoryCollection))
		})
		ingestTime := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
		wantSHA := "downstream-at-push-ingest-time"
		downstream := model.ProjectRef{Id: "project-triggers-md-push", Owner: "evergreen-ci", Repo: "sample", Branch: "main"}
		require.NoError(t, model.UpsertRepositoryRevision(ctx, downstream.Owner, downstream.Repo, downstream.Branch, wantSHA, ingestTime))
		require.NoError(t, model.UpsertRepositoryRevision(ctx, downstream.Owner, downstream.Repo, downstream.Branch, "downstream-after-push-ingest-time", ingestTime.Add(time.Minute)))
		push := model.Revision{Revision: "upstream", CreateTime: time.Date(2015, time.January, 1, 0, 0, 0, 0, time.UTC), Author: "p"}
		meta, err := getMetadataFromArgs(ctx, ProcessorArgs{
			TriggerType:       model.ProjectTriggerLevelPush,
			DownstreamProject: downstream,
			PushRevision:      push,
			PushIngestTime:    ingestTime,
		})
		require.NoError(t, err)
		assert.Equal(t, wantSHA, meta.Revision.Revision)
		assert.Equal(t, ingestTime, meta.IngestTime)
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

func TestTriggerDownstreamProjectsForPush(t *testing.T) {
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig)
	require.NoError(t, testConfig.Set(t.Context()))
	require.NoError(t, db.ClearCollections(model.ProjectRefCollection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(model.ProjectRefCollection))
	})

	const (
		upstreamProjectID   = "push-trigger-upstream"
		downstreamProjectID = "push-trigger-downstream"
	)
	downstream := model.ProjectRef{
		Id:      downstreamProjectID,
		Enabled: true,
		Triggers: []model.TriggerDefinition{
			{
				Project:      upstreamProjectID,
				Level:        model.ProjectTriggerLevelPush,
				DefinitionID: "push-trigger-definition",
				ConfigFile:   "trigger/testdata/downstream_config.yml",
			},
		},
	}
	require.NoError(t, downstream.Insert(t.Context()))

	ingestTime := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	pushCommitTime := ingestTime.Add(-time.Minute)
	pushEvent := &github.PushEvent{
		Commits: []*github.HeadCommit{
			{
				ID:        github.String("upstream-push-sha"),
				Timestamp: &github.Timestamp{Time: pushCommitTime},
				Author:    &github.CommitAuthor{},
			},
		},
	}
	var processorArgs []ProcessorArgs
	processor := func(_ context.Context, args ProcessorArgs) (*model.Version, error) {
		processorArgs = append(processorArgs, args)
		return &model.Version{Id: args.PushRevision.Revision}, nil
	}

	require.NoError(t, TriggerDownstreamProjectsForPush(t.Context(), upstreamProjectID, pushEvent, ingestTime, processor))
	require.Len(t, processorArgs, 1)
	assert.Equal(t, ingestTime, processorArgs[0].PushIngestTime)
	assert.Equal(t, "upstream-push-sha", processorArgs[0].PushRevision.Revision)
	assert.Equal(t, pushCommitTime, processorArgs[0].PushRevision.CreateTime)
	assert.Equal(t, downstreamProjectID, processorArgs[0].DownstreamProject.Id)
	assert.Equal(t, model.ProjectTriggerLevelPush, processorArgs[0].TriggerType)
	assert.Equal(t, upstreamProjectID, processorArgs[0].TriggerID)
	assert.Equal(t, "push-trigger-definition", processorArgs[0].DefinitionID)
}
