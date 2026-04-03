package trigger

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v70/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type projectTriggerSuite struct {
	suite.Suite
	processor projectProcessor
}

func mockTriggerVersion(_ context.Context, args ProcessorArgs) (*model.Version, error) {
	// we're putting the input params into arbitrary fields of the struct so that the tests can inspect them
	v := model.Version{
		Branch:      args.DownstreamProject.Id,
		Message:     args.Alias,
		TriggerID:   args.TriggerID,
		TriggerType: args.TriggerType,
	}
	return &v, nil
}

func TestProjectTriggers(t *testing.T) {
	suite.Run(t, &projectTriggerSuite{})
}

func (s *projectTriggerSuite) SetupSuite() {
	s.processor = mockTriggerVersion
	s.NoError(db.ClearCollections(task.Collection, build.Collection, model.VersionCollection))
	t := task.Task{
		Id:          "task",
		Project:     "toTrigger",
		DisplayName: "taskName",
		IngestTime:  time.Now().Add(-48 * time.Hour),
		Version:     "v",
		Requester:   evergreen.RepotrackerVersionRequester,
	}
	s.NoError(t.Insert(s.T().Context()))
	b := build.Build{
		Id:        "build",
		Project:   "toTrigger",
		Status:    evergreen.BuildFailed,
		Version:   "v",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	s.NoError(b.Insert(s.T().Context()))
	v := model.Version{
		Id: "v",
	}
	s.NoError(v.Insert(s.T().Context()))
}

func (s *projectTriggerSuite) SetupTest() {
	s.NoError(db.ClearCollections(model.ProjectRefCollection))
}

func (s *projectTriggerSuite) TestSimpleTaskFile() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	simpleTaskFile := model.ProjectRef{
		Id:      "simpleTaskFile",
		Enabled: true,
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, ConfigFile: "configFile"},
			{Project: "notTrigger", Level: model.ProjectTriggerLevelTask, ConfigFile: "configFile"},
			{Project: "somethingElse", Level: model.ProjectTriggerLevelTask, ConfigFile: "configFile"},
		},
	}
	s.NoError(simpleTaskFile.Insert(s.T().Context()))

	e := event.EventLogEntry{
		EventType:  event.TaskFinished,
		ResourceId: "task",
	}
	versions, err := EvalProjectTriggers(ctx, &e, s.processor)
	s.NoError(err)
	s.Require().Len(versions, 1)
	s.Equal("simpleTaskFile", versions[0].Branch)
}

func (s *projectTriggerSuite) TestMultipleProjects() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proj1 := model.ProjectRef{
		Id:      "proj1",
		Enabled: true,
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, ConfigFile: "configFile"},
		},
	}
	s.NoError(proj1.Insert(s.T().Context()))
	proj2 := model.ProjectRef{
		Id:      "proj2",
		Enabled: true,
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, ConfigFile: "configFile"},
		},
	}
	s.NoError(proj2.Insert(s.T().Context()))
	proj3 := model.ProjectRef{
		Id:      "proj3",
		Enabled: true,
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, ConfigFile: "configFile"},
		},
	}
	s.NoError(proj3.Insert(s.T().Context()))

	e := event.EventLogEntry{
		EventType:  event.TaskFinished,
		ResourceId: "task",
	}
	versions, err := EvalProjectTriggers(ctx, &e, s.processor)
	s.NoError(err)
	s.Len(versions, 3)
}

func (s *projectTriggerSuite) TestDateCutoff() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	date := 1
	proj := model.ProjectRef{
		Id:      "proj",
		Enabled: true,
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, ConfigFile: "configFile", DateCutoff: &date},
		},
	}
	s.NoError(proj.Insert(s.T().Context()))

	e := event.EventLogEntry{
		EventType:  event.TaskFinished,
		ResourceId: "task",
	}
	versions, err := EvalProjectTriggers(ctx, &e, s.processor)
	s.NoError(err)
	s.Empty(versions)
}

func (s *projectTriggerSuite) TestWrongEvent() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	simpleTaskFile := model.ProjectRef{
		Id:      "simpleTaskFile",
		Enabled: true,
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, ConfigFile: "configFile"},
		},
	}
	s.NoError(simpleTaskFile.Insert(s.T().Context()))

	e := event.EventLogEntry{
		EventType:  event.TaskStarted,
		ResourceId: "task",
	}
	versions, err := EvalProjectTriggers(ctx, &e, s.processor)
	s.NoError(err)
	s.Empty(versions)
}

func (s *projectTriggerSuite) TestTaskRegex() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proj1 := model.ProjectRef{
		Id:      "proj1",
		Enabled: true,
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, TaskRegex: "task*", ConfigFile: "configFile1"},
		},
	}
	s.NoError(proj1.Insert(s.T().Context()))
	proj2 := model.ProjectRef{
		Id:      "proj2",
		Enabled: true,
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, TaskRegex: "$wontmatch^", ConfigFile: "configFile2"},
		},
	}
	s.NoError(proj2.Insert(s.T().Context()))

	e := event.EventLogEntry{
		EventType:  event.TaskFinished,
		ResourceId: "task",
	}
	versions, err := EvalProjectTriggers(ctx, &e, s.processor)
	s.NoError(err)
	s.Require().Len(versions, 1)
	s.Equal("proj1", versions[0].Branch)
}

func (s *projectTriggerSuite) TestMultipleTriggers() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	duplicate := model.ProjectRef{
		Id:      "duplicate",
		Enabled: true,
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, ConfigFile: "configFile1"},
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, ConfigFile: "configFile2"},
		},
	}
	s.NoError(duplicate.Insert(s.T().Context()))

	e := event.EventLogEntry{
		EventType:  event.TaskFinished,
		ResourceId: "task",
	}
	versions, err := EvalProjectTriggers(ctx, &e, s.processor)
	s.NoError(err)
	s.Len(versions, 1)
}

func (s *projectTriggerSuite) TestBuildFinish() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ref := model.ProjectRef{
		Id:      "ref",
		Enabled: true,
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelBuild, ConfigFile: "configFile"},
			{Project: "notTrigger", Level: model.ProjectTriggerLevelBuild, ConfigFile: "configFile"},
			{Project: "somethingElse", Level: model.ProjectTriggerLevelBuild, ConfigFile: "configFile"},
		},
	}
	s.NoError(ref.Insert(s.T().Context()))

	e := event.EventLogEntry{
		EventType:  event.BuildStateChange,
		ResourceId: "build",
		Data: &event.BuildEventData{
			Status: evergreen.BuildFailed,
		},
		ResourceType: event.ResourceTypeBuild,
	}
	versions, err := EvalProjectTriggers(ctx, &e, s.processor)
	s.NoError(err)
	s.Require().Len(versions, 1)
	s.Equal("ref", versions[0].Branch)
}

func TestProjectTriggerIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	assert.NoError(db.ClearCollections(task.Collection, build.Collection, model.VersionCollection, evergreen.ConfigCollection,
		model.ProjectRefCollection, model.RepositoriesCollection, model.ProjectAliasCollection, model.ParserProjectCollection, manifest.Collection))
	require.NoError(db.CreateCollections(model.ParserProjectCollection))

	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config)
	assert.NoError(config.Set(ctx))
	e := event.EventLogEntry{
		ID:           "event1",
		ResourceId:   "upstreamTask",
		ResourceType: event.ResourceTypeTask,
		EventType:    event.TaskFinished,
	}
	upstreamTask := task.Task{
		Id:          "upstreamTask",
		Status:      evergreen.TaskSucceeded,
		Requester:   evergreen.RepotrackerVersionRequester,
		DisplayName: "upstreamTask",
		Version:     "upstreamVersion",
		Project:     "upstream",
	}
	assert.NoError(upstreamTask.Insert(t.Context()))
	upstreamVersion := model.Version{
		Id:         "upstreamVersion",
		Author:     "me",
		CreateTime: time.Date(2023, 12, 13, 18, 13, 31, 0, time.UTC),
		Revision:   "abc",
		Identifier: "upstream",
	}
	assert.NoError(upstreamVersion.Insert(t.Context()))
	downstreamProjectRef := model.ProjectRef{
		Id:         mgobson.NewObjectId().Hex(),
		Identifier: "downstream",
		Enabled:    true,
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		RemotePath: "self-tests.yml",
		Branch:     "main",
		Triggers: []model.TriggerDefinition{
			{Project: "upstream", Level: "task", DefinitionID: "def1", TaskRegex: "upstream*", Status: evergreen.TaskSucceeded, UnscheduleDownstreamVersions: true, ConfigFile: "trigger/testdata/downstream_config.yml", Alias: "a1"},
		},
	}
	assert.NoError(downstreamProjectRef.Insert(t.Context()))
	uptreamProjectRef := model.ProjectRef{
		Id:         mgobson.NewObjectId().Hex(),
		Identifier: "upstream",
		Enabled:    true,
		Owner:      "evergreen-ci",
		Repo:       "sample",
		Branch:     "main",
	}
	assert.NoError(uptreamProjectRef.Insert(t.Context()))
	alias := model.ProjectAlias{
		ID:        mgobson.NewObjectId(),
		ProjectID: downstreamProjectRef.Id,
		Alias:     "a1",
		Variant:   "buildvariant",
		Task:      "task1",
	}
	assert.NoError(alias.Upsert(t.Context()))
	_, err := model.GetNewRevisionOrderNumber(t.Context(), downstreamProjectRef.Id)
	assert.NoError(err)
	downstreamRevision := "c37179fcad01b12ef752a65af3156fb8dc7e452c"
	assert.NoError(model.UpdateLastRevision(t.Context(), downstreamProjectRef.Id, downstreamRevision))

	downstreamVersions, err := EvalProjectTriggers(ctx, &e, TriggerDownstreamVersion)
	assert.NoError(err)
	require.Len(downstreamVersions, 1)
	dbVersion, err := model.VersionFindOneId(t.Context(), downstreamVersions[0].Id)
	assert.NoError(err)
	require.NotNil(dbVersion)
	versions := []model.Version{downstreamVersions[0], *dbVersion}
	for _, v := range versions {
		assert.False(utility.FromBoolPtr(v.Activated))
		assert.Equal("downstream_abc_def1", v.Id)
		assert.Equal(downstreamRevision, v.Revision)
		assert.Equal(evergreen.VersionCreated, v.Status)
		assert.Equal(downstreamProjectRef.Id, v.Identifier)
		assert.Equal(evergreen.TriggerRequester, v.Requester)
		assert.Equal(upstreamTask.Id, v.TriggerID)
		assert.Equal("task", v.TriggerType)
		assert.Equal(e.ID, v.TriggerEvent)
	}
	builds, err := build.Find(t.Context(), build.ByVersion(downstreamVersions[0].Id))
	assert.NoError(err)
	assert.NotEmpty(builds)
	for _, b := range builds {
		assert.False(b.Activated)
		assert.Equal(downstreamProjectRef.Id, b.Project)
		assert.Equal(evergreen.TriggerRequester, b.Requester)
		assert.Equal(evergreen.BuildCreated, b.Status)
		assert.Equal(upstreamTask.Id, b.TriggerID)
		assert.Equal("task", b.TriggerType)
		assert.Equal(e.ID, b.TriggerEvent)
		assert.Contains(b.BuildVariant, "buildvariant")
	}
	tasks, err := task.Find(ctx, task.ByVersion(downstreamVersions[0].Id))
	assert.NoError(err)
	assert.NotEmpty(tasks)
	for _, t := range tasks {
		assert.False(t.Activated)
		assert.Equal(downstreamProjectRef.Id, t.Project)
		assert.Equal(evergreen.TriggerRequester, t.Requester)
		assert.Equal(evergreen.TaskUndispatched, t.Status)
		assert.Equal(upstreamTask.Id, t.TriggerID)
		assert.Equal("task", t.TriggerType)
		assert.Equal(e.ID, t.TriggerEvent)
		assert.Contains(t.DisplayName, "task1")
	}
	mani, err := manifest.FindFromVersion(ctx, dbVersion.Id, downstreamProjectRef.Id, downstreamRevision, evergreen.RepotrackerVersionRequester)
	assert.NoError(err)
	require.NotNil(mani)
	assert.Equal(downstreamProjectRef.Id, mani.ProjectName)
	assert.Equal(uptreamProjectRef.Branch, mani.Branch)

	// verify that triggering this version again does nothing
	upstreamVersionFromDB, err := model.VersionFindOneId(t.Context(), upstreamVersion.Id)
	assert.NoError(err)
	assert.Contains(upstreamVersionFromDB.SatisfiedTriggers, "def1")
	downstreamVersions, err = EvalProjectTriggers(ctx, &e, TriggerDownstreamVersion)
	assert.NoError(err)
	assert.Empty(downstreamVersions)
}

func TestProjectTriggerIntegrationForBuild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	assert.NoError(db.ClearCollections(task.Collection, build.Collection, model.VersionCollection, evergreen.ConfigCollection,
		model.ProjectRefCollection, model.RepositoriesCollection, model.ProjectAliasCollection, model.ParserProjectCollection, manifest.Collection))
	require.NoError(db.CreateCollections(model.ParserProjectCollection))

	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config)
	assert.NoError(config.Set(ctx))
	e := event.EventLogEntry{
		ID:           "event1",
		ResourceId:   "upstreamBuild",
		ResourceType: event.ResourceTypeBuild,
		EventType:    event.BuildStateChange,
		Data: &event.BuildEventData{
			Status: evergreen.BuildSucceeded,
		},
	}
	upstreamBuild := build.Build{
		Id:          "upstreamBuild",
		Status:      evergreen.BuildSucceeded,
		Requester:   evergreen.RepotrackerVersionRequester,
		DisplayName: "upstreamBuild",
		Version:     "upstreamVersion",
		Project:     "upstream",
	}
	assert.NoError(upstreamBuild.Insert(t.Context()))
	upstreamVersion := model.Version{
		Id:         "upstreamVersion",
		Author:     "me",
		CreateTime: time.Date(2023, 12, 13, 18, 13, 31, 0, time.UTC),
		Revision:   "abc",
		Identifier: "upstream",
	}
	assert.NoError(upstreamVersion.Insert(t.Context()))
	downstreamProjectRef := model.ProjectRef{
		Id:         mgobson.NewObjectId().Hex(),
		Identifier: "downstream",
		Enabled:    true,
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		RemotePath: "self-tests.yml",
		Branch:     "main",
		Triggers: []model.TriggerDefinition{
			{Project: "upstream", Level: "build", DefinitionID: "def1", TaskRegex: "upstream*", Status: evergreen.BuildSucceeded, ConfigFile: "trigger/testdata/downstream_config.yml", Alias: "a1"},
		},
	}
	assert.NoError(downstreamProjectRef.Insert(t.Context()))
	uptreamProjectRef := model.ProjectRef{
		Id:         mgobson.NewObjectId().Hex(),
		Identifier: "upstream",
		Enabled:    true,
		Owner:      "evergreen-ci",
		Repo:       "sample",
		Branch:     "main",
	}
	assert.NoError(uptreamProjectRef.Insert(t.Context()))
	alias := model.ProjectAlias{
		ID:        mgobson.NewObjectId(),
		ProjectID: downstreamProjectRef.Id,
		Alias:     "a1",
		Variant:   "buildvariant",
		Task:      "task1",
	}
	assert.NoError(alias.Upsert(t.Context()))
	_, err := model.GetNewRevisionOrderNumber(t.Context(), downstreamProjectRef.Id)
	assert.NoError(err)
	downstreamRevision := "c37179fcad01b12ef752a65af3156fb8dc7e452c"
	assert.NoError(model.UpdateLastRevision(t.Context(), downstreamProjectRef.Id, downstreamRevision))

	downstreamVersions, err := EvalProjectTriggers(ctx, &e, TriggerDownstreamVersion)
	assert.NoError(err)
	require.Len(downstreamVersions, 1)
	dbVersion, err := model.VersionFindOneId(t.Context(), downstreamVersions[0].Id)
	assert.NoError(err)
	require.NotNil(dbVersion)
	versions := []model.Version{downstreamVersions[0], *dbVersion}
	for _, v := range versions {
		assert.True(utility.FromBoolPtr(v.Activated))
		assert.Equal("downstream_abc_def1", v.Id)
		assert.Equal(downstreamRevision, v.Revision)
		assert.Equal(evergreen.VersionCreated, v.Status)
		assert.Equal(downstreamProjectRef.Id, v.Identifier)
		assert.Equal(evergreen.TriggerRequester, v.Requester)
		assert.Equal(upstreamBuild.Id, v.TriggerID)
		assert.Equal("build", v.TriggerType)
		assert.Equal(e.ID, v.TriggerEvent)
	}
	builds, err := build.Find(t.Context(), build.ByVersion(downstreamVersions[0].Id))
	assert.NoError(err)
	assert.NotEmpty(builds)
	for _, b := range builds {
		assert.True(b.Activated)
		assert.Equal(downstreamProjectRef.Id, b.Project)
		assert.Equal(evergreen.TriggerRequester, b.Requester)
		assert.Equal(evergreen.BuildCreated, b.Status)
		assert.Equal(upstreamBuild.Id, b.TriggerID)
		assert.Equal("build", b.TriggerType)
		assert.Equal(e.ID, b.TriggerEvent)
		assert.Contains(b.BuildVariant, "buildvariant")
	}
	tasks, err := task.Find(ctx, task.ByVersion(downstreamVersions[0].Id))
	assert.NoError(err)
	assert.NotEmpty(tasks)
	for _, t := range tasks {
		assert.True(t.Activated)
		assert.Equal(downstreamProjectRef.Id, t.Project)
		assert.Equal(evergreen.TriggerRequester, t.Requester)
		assert.Equal(evergreen.TaskUndispatched, t.Status)
		assert.Equal(upstreamBuild.Id, t.TriggerID)
		assert.Equal("build", t.TriggerType)
		assert.Equal(e.ID, t.TriggerEvent)
		assert.Contains(t.DisplayName, "task1")
	}
	mani, err := manifest.FindFromVersion(ctx, dbVersion.Id, downstreamProjectRef.Id, downstreamRevision, evergreen.RepotrackerVersionRequester)
	assert.NoError(err)
	require.NotNil(mani)
	assert.Equal(downstreamProjectRef.Id, mani.ProjectName)
	assert.Equal(uptreamProjectRef.Branch, mani.Branch)

	// verify that triggering this version again does nothing
	upstreamVersionFromDB, err := model.VersionFindOneId(t.Context(), upstreamVersion.Id)
	assert.NoError(err)
	assert.Contains(upstreamVersionFromDB.SatisfiedTriggers, "def1")
	downstreamVersions, err = EvalProjectTriggers(ctx, &e, TriggerDownstreamVersion)
	assert.NoError(err)
	assert.Empty(downstreamVersions)
}

func TestProjectTriggerIntegrationForPush(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)

	assert.NoError(db.ClearCollections(task.Collection, build.Collection, model.VersionCollection, evergreen.ConfigCollection,
		model.ProjectRefCollection, model.RepositoriesCollection, model.ProjectAliasCollection, model.ParserProjectCollection, manifest.Collection))
	require.NoError(db.CreateCollections(model.ParserProjectCollection))

	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config)
	assert.NoError(config.Set(ctx))

	downstreamProjectRef := model.ProjectRef{
		Id:         mgobson.NewObjectId().Hex(),
		Identifier: "downstream",
		Enabled:    true,
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		RemotePath: "self-tests.yml",
		Branch:     "main",
		Triggers: []model.TriggerDefinition{
			{Project: "upstream", Level: model.ProjectTriggerLevelPush, DefinitionID: "def1", TaskRegex: "upstream*", Status: evergreen.BuildSucceeded, ConfigFile: "trigger/testdata/downstream_config.yml", Alias: "a1"},
		},
	}
	assert.NoError(downstreamProjectRef.Insert(t.Context()))
	uptreamProjectRef := model.ProjectRef{
		Id:         mgobson.NewObjectId().Hex(),
		Identifier: "upstream",
		Enabled:    true,
		Owner:      "evergreen-ci",
		Repo:       "sample",
		Branch:     "main",
	}
	assert.NoError(uptreamProjectRef.Insert(t.Context()))
	alias := model.ProjectAlias{
		ID:        mgobson.NewObjectId(),
		ProjectID: downstreamProjectRef.Id,
		Alias:     "a1",
		Variant:   "buildvariant",
		Task:      "task1",
	}
	assert.NoError(alias.Upsert(t.Context()))
	_, err := model.GetNewRevisionOrderNumber(t.Context(), downstreamProjectRef.Id)
	assert.NoError(err)
	downstreamRevisionPrior := "3c10c5e814535dc8e6d06553cce69a60e4f58c91"
	assert.NoError(model.UpdateLastRevision(t.Context(), downstreamProjectRef.Id, downstreamRevisionPrior))

	sampleOlderSHA := "9bdedd0990e83e328e42f7bb8c2771cab6ae0145"
	sampleNewerSHA := "7e05633b9bc529e19eba18b1fc88f78d346855b2"
	downstreamRevisionOlder := "178959df398ed0767113492cbd90ad9afbc67bfe"
	downstreamRevisionNewer := "c37179fcad01b12ef752a65af3156fb8dc7e452c"

	pushEvent := &github.PushEvent{
		Commits: []*github.HeadCommit{
			{
				ID:      utility.ToStringPtr(sampleOlderSHA),
				Message: utility.ToStringPtr("Update random.txt"),
				Author: &github.CommitAuthor{
					Email: utility.ToStringPtr("chaya.malik@10gen.com"),
					Name:  utility.ToStringPtr("Chaya Malik"),
				},
				Timestamp: &github.Timestamp{Time: time.Date(2023, 12, 12, 17, 59, 6, 0, time.UTC)},
			},
			{
				ID:      utility.ToStringPtr(sampleNewerSHA),
				Message: utility.ToStringPtr("Update README.md"),
				Author: &github.CommitAuthor{
					Email: utility.ToStringPtr("chaya.malik@10gen.com"),
					Name:  utility.ToStringPtr("Chaya Malik"),
				},
				Timestamp: &github.Timestamp{Time: time.Date(2023, 12, 13, 18, 13, 31, 0, time.UTC)},
			},
		},
	}
	err = TriggerDownstreamProjectsForPush(ctx, "upstream", pushEvent, TriggerDownstreamVersion)
	assert.NoError(err)
	dbVersions, err := model.VersionFind(t.Context(), model.VersionByProjectIdAndCreateTime(downstreamProjectRef.Id, time.Now()))
	assert.NoError(err)
	require.Len(dbVersions, 2)

	assert.True(utility.FromBoolPtr(dbVersions[0].Activated))
	assert.Equal("downstream_"+sampleNewerSHA+"_def1", dbVersions[0].Id)
	assert.Equal(downstreamRevisionNewer, dbVersions[0].Revision)
	assert.Equal(evergreen.VersionCreated, dbVersions[0].Status)
	assert.Equal(downstreamProjectRef.Id, dbVersions[0].Identifier)
	assert.Equal(evergreen.TriggerRequester, dbVersions[0].Requester)
	assert.Equal(model.ProjectTriggerLevelPush, dbVersions[0].TriggerType)
	assert.Equal(sampleNewerSHA, dbVersions[0].TriggerSHA)
	assert.Equal("upstream", dbVersions[0].TriggerID)
	assert.True(utility.FromBoolPtr(dbVersions[0].Activated))

	assert.Equal("downstream_"+sampleOlderSHA+"_def1", dbVersions[1].Id)
	assert.Equal(downstreamRevisionOlder, dbVersions[1].Revision)
	assert.Equal(evergreen.VersionCreated, dbVersions[1].Status)
	assert.Equal(downstreamProjectRef.Id, dbVersions[1].Identifier)
	assert.Equal(evergreen.TriggerRequester, dbVersions[1].Requester)
	assert.Equal(model.ProjectTriggerLevelPush, dbVersions[1].TriggerType)
	assert.Equal(sampleOlderSHA, dbVersions[1].TriggerSHA)
	assert.Equal("upstream", dbVersions[1].TriggerID)

	builds, err := build.Find(t.Context(), build.ByVersion(dbVersions[0].Id))
	assert.NoError(err)
	assert.NotEmpty(builds)
	for _, b := range builds {
		assert.True(b.Activated)
		assert.Equal(downstreamProjectRef.Id, b.Project)
		assert.Equal(evergreen.TriggerRequester, b.Requester)
		assert.Equal(evergreen.BuildCreated, b.Status)
		assert.Equal(model.ProjectTriggerLevelPush, b.TriggerType)
		assert.Contains(b.BuildVariant, "buildvariant")
	}
	tasks, err := task.Find(ctx, task.ByVersion(dbVersions[0].Id))
	assert.NoError(err)
	assert.NotEmpty(tasks)
	for _, t := range tasks {
		assert.True(t.Activated)
		assert.Equal(downstreamProjectRef.Id, t.Project)
		assert.Equal(evergreen.TriggerRequester, t.Requester)
		assert.Equal(evergreen.TaskUndispatched, t.Status)
		assert.Equal(model.ProjectTriggerLevelPush, t.TriggerType)
		assert.Contains(t.DisplayName, "task1")
	}
	mani, err := manifest.FindFromVersion(ctx, dbVersions[1].Id, downstreamProjectRef.Id, downstreamRevisionOlder, evergreen.RepotrackerVersionRequester)
	assert.NoError(err)
	require.NotNil(mani)
	assert.Equal(downstreamProjectRef.Id, mani.ProjectName)
	assert.Equal(uptreamProjectRef.Branch, mani.Branch)
	assert.Len(mani.Modules, 1)
	require.NotNil(mani.Modules["sample"])
	assert.Equal(sampleOlderSHA, mani.Modules["sample"].Revision)
	assert.Equal("main", mani.Modules["sample"].Branch)
	assert.Equal("sample", mani.Modules["sample"].Repo)
	assert.Equal("evergreen-ci", mani.Modules["sample"].Owner)

	// verify that triggering this version again does nothing
	assert.NoError(TriggerDownstreamProjectsForPush(ctx, uptreamProjectRef.Id, pushEvent, TriggerDownstreamVersion))
}
