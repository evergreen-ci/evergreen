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
	"github.com/google/go-github/v52/github"
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
	s.NoError(t.Insert())
	b := build.Build{
		Id:        "build",
		Project:   "toTrigger",
		Status:    evergreen.BuildFailed,
		Version:   "v",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	s.NoError(b.Insert())
	v := model.Version{
		Id: "v",
	}
	s.NoError(v.Insert())
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
	s.NoError(simpleTaskFile.Insert())

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
	s.NoError(proj1.Insert())
	proj2 := model.ProjectRef{
		Id:      "proj2",
		Enabled: true,
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, ConfigFile: "configFile"},
		},
	}
	s.NoError(proj2.Insert())
	proj3 := model.ProjectRef{
		Id:      "proj3",
		Enabled: true,
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, ConfigFile: "configFile"},
		},
	}
	s.NoError(proj3.Insert())

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
	s.NoError(proj.Insert())

	e := event.EventLogEntry{
		EventType:  event.TaskFinished,
		ResourceId: "task",
	}
	versions, err := EvalProjectTriggers(ctx, &e, s.processor)
	s.NoError(err)
	s.Len(versions, 0)
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
	s.NoError(simpleTaskFile.Insert())

	e := event.EventLogEntry{
		EventType:  event.TaskStarted,
		ResourceId: "task",
	}
	versions, err := EvalProjectTriggers(ctx, &e, s.processor)
	s.NoError(err)
	s.Len(versions, 0)
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
	s.NoError(proj1.Insert())
	proj2 := model.ProjectRef{
		Id:      "proj2",
		Enabled: true,
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, TaskRegex: "$wontmatch^", ConfigFile: "configFile2"},
		},
	}
	s.NoError(proj2.Insert())

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
	s.NoError(duplicate.Insert())

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
	s.NoError(ref.Insert())

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
	_ = evergreen.GetEnvironment().DB().RunCommand(ctx, map[string]string{"create": model.ParserProjectCollection})

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
	assert.NoError(upstreamTask.Insert())
	upstreamVersion := model.Version{
		Id:         "upstreamVersion",
		Author:     "me",
		CreateTime: time.Now(),
		Revision:   "abc",
		Identifier: "upstream",
	}
	assert.NoError(upstreamVersion.Insert())
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
	assert.NoError(downstreamProjectRef.Insert())
	uptreamProjectRef := model.ProjectRef{
		Id:         mgobson.NewObjectId().Hex(),
		Identifier: "upstream",
		Enabled:    true,
		Owner:      "evergreen-ci",
		Repo:       "sample",
		Branch:     "main",
	}
	assert.NoError(uptreamProjectRef.Insert())
	alias := model.ProjectAlias{
		ID:        mgobson.NewObjectId(),
		ProjectID: downstreamProjectRef.Id,
		Alias:     "a1",
		Variant:   "buildvariant",
		Task:      "task1",
	}
	assert.NoError(alias.Upsert())
	_, err := model.GetNewRevisionOrderNumber(downstreamProjectRef.Id)
	assert.NoError(err)
	downstreamRevision := "9338711cc1acc94ff75889a3b53a936a00e8c385"
	assert.NoError(model.UpdateLastRevision(downstreamProjectRef.Id, downstreamRevision))

	downstreamVersions, err := EvalProjectTriggers(ctx, &e, TriggerDownstreamVersion)
	assert.NoError(err)
	dbVersions, err := model.VersionFind(model.BaseVersionByProjectIdAndRevision(downstreamProjectRef.Id, downstreamRevision))
	assert.NoError(err)
	require.Len(downstreamVersions, 1)
	require.Len(dbVersions, 1)
	versions := []model.Version{downstreamVersions[0], dbVersions[0]}
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
	builds, err := build.Find(build.ByVersion(downstreamVersions[0].Id))
	assert.NoError(err)
	assert.True(len(builds) > 0)
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
	tasks, err := task.Find(task.ByVersion(downstreamVersions[0].Id))
	assert.NoError(err)
	assert.True(len(tasks) > 0)
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
	mani, err := manifest.FindFromVersion(dbVersions[0].Id, downstreamProjectRef.Id, downstreamRevision, evergreen.RepotrackerVersionRequester)
	assert.NoError(err)
	require.NotNil(mani)
	assert.Equal(downstreamProjectRef.Id, mani.ProjectName)
	assert.Equal(uptreamProjectRef.Branch, mani.Branch)

	// verify that triggering this version again does nothing
	upstreamVersionFromDB, err := model.VersionFindOneId(upstreamVersion.Id)
	assert.NoError(err)
	assert.Contains(upstreamVersionFromDB.SatisfiedTriggers, "def1")
	downstreamVersions, err = EvalProjectTriggers(ctx, &e, TriggerDownstreamVersion)
	assert.NoError(err)
	assert.Len(downstreamVersions, 0)
}

func TestProjectTriggerIntegrationForBuild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	assert.NoError(db.ClearCollections(task.Collection, build.Collection, model.VersionCollection, evergreen.ConfigCollection,
		model.ProjectRefCollection, model.RepositoriesCollection, model.ProjectAliasCollection, model.ParserProjectCollection, manifest.Collection))
	_ = evergreen.GetEnvironment().DB().RunCommand(ctx, map[string]string{"create": model.ParserProjectCollection})

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
	assert.NoError(upstreamBuild.Insert())
	upstreamVersion := model.Version{
		Id:         "upstreamVersion",
		Author:     "me",
		CreateTime: time.Now(),
		Revision:   "abc",
		Identifier: "upstream",
	}
	assert.NoError(upstreamVersion.Insert())
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
	assert.NoError(downstreamProjectRef.Insert())
	uptreamProjectRef := model.ProjectRef{
		Id:         mgobson.NewObjectId().Hex(),
		Identifier: "upstream",
		Enabled:    true,
		Owner:      "evergreen-ci",
		Repo:       "sample",
		Branch:     "main",
	}
	assert.NoError(uptreamProjectRef.Insert())
	alias := model.ProjectAlias{
		ID:        mgobson.NewObjectId(),
		ProjectID: downstreamProjectRef.Id,
		Alias:     "a1",
		Variant:   "buildvariant",
		Task:      "task1",
	}
	assert.NoError(alias.Upsert())
	_, err := model.GetNewRevisionOrderNumber(downstreamProjectRef.Id)
	assert.NoError(err)
	downstreamRevision := "9338711cc1acc94ff75889a3b53a936a00e8c385"
	assert.NoError(model.UpdateLastRevision(downstreamProjectRef.Id, downstreamRevision))

	downstreamVersions, err := EvalProjectTriggers(ctx, &e, TriggerDownstreamVersion)
	assert.NoError(err)
	dbVersions, err := model.VersionFind(model.BaseVersionByProjectIdAndRevision(downstreamProjectRef.Id, downstreamRevision))
	assert.NoError(err)
	require.Len(downstreamVersions, 1)
	require.Len(dbVersions, 1)
	versions := []model.Version{downstreamVersions[0], dbVersions[0]}
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
	builds, err := build.Find(build.ByVersion(downstreamVersions[0].Id))
	assert.NoError(err)
	assert.True(len(builds) > 0)
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
	tasks, err := task.Find(task.ByVersion(downstreamVersions[0].Id))
	assert.NoError(err)
	assert.True(len(tasks) > 0)
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
	mani, err := manifest.FindFromVersion(dbVersions[0].Id, downstreamProjectRef.Id, downstreamRevision, evergreen.RepotrackerVersionRequester)
	assert.NoError(err)
	require.NotNil(mani)
	assert.Equal(downstreamProjectRef.Id, mani.ProjectName)
	assert.Equal(uptreamProjectRef.Branch, mani.Branch)

	// verify that triggering this version again does nothing
	upstreamVersionFromDB, err := model.VersionFindOneId(upstreamVersion.Id)
	assert.NoError(err)
	assert.Contains(upstreamVersionFromDB.SatisfiedTriggers, "def1")
	downstreamVersions, err = EvalProjectTriggers(ctx, &e, TriggerDownstreamVersion)
	assert.NoError(err)
	assert.Len(downstreamVersions, 0)
}

func TestProjectTriggerIntegrationForPush(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	assert.NoError(db.ClearCollections(task.Collection, build.Collection, model.VersionCollection, evergreen.ConfigCollection,
		model.ProjectRefCollection, model.RepositoriesCollection, model.ProjectAliasCollection, model.ParserProjectCollection, manifest.Collection))
	_ = evergreen.GetEnvironment().DB().RunCommand(ctx, map[string]string{"create": model.ParserProjectCollection})

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
	assert.NoError(downstreamProjectRef.Insert())
	uptreamProjectRef := model.ProjectRef{
		Id:         mgobson.NewObjectId().Hex(),
		Identifier: "upstream",
		Enabled:    true,
		Owner:      "evergreen-ci",
		Repo:       "sample",
		Branch:     "main",
	}
	assert.NoError(uptreamProjectRef.Insert())
	alias := model.ProjectAlias{
		ID:        mgobson.NewObjectId(),
		ProjectID: downstreamProjectRef.Id,
		Alias:     "a1",
		Variant:   "buildvariant",
		Task:      "task1",
	}
	assert.NoError(alias.Upsert())
	_, err := model.GetNewRevisionOrderNumber(downstreamProjectRef.Id)
	assert.NoError(err)
	downstreamRevision := "cf46076567e4949f9fc68e0634139d4ac495c89b"
	assert.NoError(model.UpdateLastRevision(downstreamProjectRef.Id, downstreamRevision))

	pushEvent := &github.PushEvent{
		HeadCommit: &github.HeadCommit{
			ID:      utility.ToStringPtr("3585388b1591dfca47ac26a5b9a564ec8f138a5e"),
			Message: utility.ToStringPtr("message"),
			Author: &github.CommitAuthor{
				Email: utility.ToStringPtr("hello@example.com"),
				Name:  utility.ToStringPtr("test"),
			},
			Timestamp: &github.Timestamp{Time: time.Now()},
		},
	}
	err = TriggerDownstreamProjectsForPush(ctx, "upstream", pushEvent, TriggerDownstreamVersion)
	assert.NoError(err)
	dbVersions, err := model.VersionFind(model.BaseVersionByProjectIdAndRevision(downstreamProjectRef.Id, downstreamRevision))
	assert.NoError(err)
	require.Len(dbVersions, 1)
	assert.True(utility.FromBoolPtr(dbVersions[0].Activated))
	assert.Equal("downstream_3585388b1591dfca47ac26a5b9a564ec8f138a5e_def1", dbVersions[0].Id)
	assert.Equal(downstreamRevision, dbVersions[0].Revision)
	assert.Equal(evergreen.VersionCreated, dbVersions[0].Status)
	assert.Equal(downstreamProjectRef.Id, dbVersions[0].Identifier)
	assert.Equal(evergreen.TriggerRequester, dbVersions[0].Requester)
	assert.Equal(model.ProjectTriggerLevelPush, dbVersions[0].TriggerType)
	assert.Equal("3585388b1591dfca47ac26a5b9a564ec8f138a5e", dbVersions[0].TriggerSHA)
	assert.Equal("upstream", dbVersions[0].TriggerID)

	builds, err := build.Find(build.ByVersion(dbVersions[0].Id))
	assert.NoError(err)
	assert.True(len(builds) > 0)
	for _, b := range builds {
		assert.True(b.Activated)
		assert.Equal(downstreamProjectRef.Id, b.Project)
		assert.Equal(evergreen.TriggerRequester, b.Requester)
		assert.Equal(evergreen.BuildCreated, b.Status)
		assert.Equal(model.ProjectTriggerLevelPush, b.TriggerType)
		assert.Contains(b.BuildVariant, "buildvariant")
	}
	tasks, err := task.Find(task.ByVersion(dbVersions[0].Id))
	assert.NoError(err)
	assert.True(len(tasks) > 0)
	for _, t := range tasks {
		assert.True(t.Activated)
		assert.Equal(downstreamProjectRef.Id, t.Project)
		assert.Equal(evergreen.TriggerRequester, t.Requester)
		assert.Equal(evergreen.TaskUndispatched, t.Status)
		assert.Equal(model.ProjectTriggerLevelPush, t.TriggerType)
		assert.Contains(t.DisplayName, "task1")
	}
	mani, err := manifest.FindFromVersion(dbVersions[0].Id, downstreamProjectRef.Id, downstreamRevision, evergreen.RepotrackerVersionRequester)
	assert.NoError(err)
	require.NotNil(mani)
	assert.Equal(downstreamProjectRef.Id, mani.ProjectName)
	assert.Equal(uptreamProjectRef.Branch, mani.Branch)
	assert.Len(mani.Modules, 1)
	require.NotNil(mani.Modules["sample"])
	assert.Equal("3585388b1591dfca47ac26a5b9a564ec8f138a5e", mani.Modules["sample"].Revision)
	assert.Equal("main", mani.Modules["sample"].Branch)
	assert.Equal("sample", mani.Modules["sample"].Repo)
	assert.Equal("evergreen-ci", mani.Modules["sample"].Owner)

	// verify that triggering this version again does nothing
	assert.NoError(TriggerDownstreamProjectsForPush(ctx, uptreamProjectRef.Id, pushEvent, TriggerDownstreamVersion))
}
