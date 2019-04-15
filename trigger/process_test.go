package trigger

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	mgobson "gopkg.in/mgo.v2/bson"
)

type projectTriggerSuite struct {
	suite.Suite
	processor projectProcessor
}

func mockTriggerVersion(args ProcessorArgs) (*model.Version, error) {
	// we're putting the input params into arbitrary fields of the struct so that the tests can inspect them
	v := model.Version{
		Branch:      args.DownstreamProject.Identifier,
		Config:      args.ConfigFile,
		Message:     args.Command,
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
	simpleTaskFile := model.ProjectRef{
		Identifier: "simpleTaskFile",
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
	versions, err := EvalProjectTriggers(&e, s.processor)
	s.NoError(err)
	s.Require().Len(versions, 1)
	s.Equal("simpleTaskFile", versions[0].Branch)
	s.Equal("configFile", versions[0].Config)
}

func (s *projectTriggerSuite) TestSimpleTaskCommand() {
	simpleTaskCommand := model.ProjectRef{
		Identifier: "simpleTaskCommand",
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, Command: "command"},
			{Project: "notTrigger", Level: model.ProjectTriggerLevelTask, Command: "command"},
			{Project: "somethingElse", Level: model.ProjectTriggerLevelTask, Command: "command"},
		},
	}
	s.NoError(simpleTaskCommand.Insert())

	e := event.EventLogEntry{
		EventType:  event.TaskFinished,
		ResourceId: "task",
	}
	versions, err := EvalProjectTriggers(&e, s.processor)
	s.NoError(err)
	s.Require().Len(versions, 1)
	s.Equal("simpleTaskCommand", versions[0].Branch)
	s.Equal("command", versions[0].Message)
}

func (s *projectTriggerSuite) TestMultipleProjects() {
	proj1 := model.ProjectRef{
		Identifier: "proj1",
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, ConfigFile: "configFile"},
		},
	}
	s.NoError(proj1.Insert())
	proj2 := model.ProjectRef{
		Identifier: "proj2",
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, Command: "command"},
		},
	}
	s.NoError(proj2.Insert())
	proj3 := model.ProjectRef{
		Identifier: "proj3",
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, Command: "command"},
		},
	}
	s.NoError(proj3.Insert())

	e := event.EventLogEntry{
		EventType:  event.TaskFinished,
		ResourceId: "task",
	}
	versions, err := EvalProjectTriggers(&e, s.processor)
	s.NoError(err)
	s.Len(versions, 3)
}

func (s *projectTriggerSuite) TestDateCutoff() {
	date := 1
	proj := model.ProjectRef{
		Identifier: "proj",
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, ConfigFile: "configFile", DateCutoff: &date},
		},
	}
	s.NoError(proj.Insert())

	e := event.EventLogEntry{
		EventType:  event.TaskFinished,
		ResourceId: "task",
	}
	versions, err := EvalProjectTriggers(&e, s.processor)
	s.NoError(err)
	s.Len(versions, 0)
}

func (s *projectTriggerSuite) TestWrongEvent() {
	simpleTaskCommand := model.ProjectRef{
		Identifier: "simpleTaskCommand",
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, Command: "command"},
		},
	}
	s.NoError(simpleTaskCommand.Insert())

	e := event.EventLogEntry{
		EventType:  event.TaskStarted,
		ResourceId: "task",
	}
	versions, err := EvalProjectTriggers(&e, s.processor)
	s.NoError(err)
	s.Len(versions, 0)
}

func (s *projectTriggerSuite) TestTaskRegex() {
	proj1 := model.ProjectRef{
		Identifier: "proj1",
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, TaskRegex: "task*", ConfigFile: "configFile1"},
		},
	}
	s.NoError(proj1.Insert())
	proj2 := model.ProjectRef{
		Identifier: "proj2",
		Triggers: []model.TriggerDefinition{
			{Project: "toTrigger", Level: model.ProjectTriggerLevelTask, TaskRegex: "$wontmatch^", ConfigFile: "configFile2"},
		},
	}
	s.NoError(proj2.Insert())

	e := event.EventLogEntry{
		EventType:  event.TaskFinished,
		ResourceId: "task",
	}
	versions, err := EvalProjectTriggers(&e, s.processor)
	s.NoError(err)
	s.Require().Len(versions, 1)
	s.Equal("proj1", versions[0].Branch)
	s.Equal("configFile1", versions[0].Config)
}

func (s *projectTriggerSuite) TestMultipleTriggers() {
	duplicate := model.ProjectRef{
		Identifier: "duplicate",
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
	versions, err := EvalProjectTriggers(&e, s.processor)
	s.NoError(err)
	s.Len(versions, 1)
}

func (s *projectTriggerSuite) TestBuildFinish() {
	ref := model.ProjectRef{
		Identifier: "ref",
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
	versions, err := EvalProjectTriggers(&e, s.processor)
	s.NoError(err)
	s.Require().Len(versions, 1)
	s.Equal("ref", versions[0].Branch)
	s.Equal("configFile", versions[0].Config)
}

func TestProjectTriggerIntegration(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	assert.NoError(db.ClearCollections(task.Collection, build.Collection, model.VersionCollection, evergreen.ConfigCollection,
		model.ProjectRefCollection, model.RepositoriesCollection, model.ProjectAliasCollection))
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, "TestProjectTriggerIntegration")
	assert.NoError(config.Set())
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
		Identifier: "downstream",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		RemotePath: "self-tests.yml",
		RepoKind:   "github",
		Triggers: []model.TriggerDefinition{
			{Project: "upstream", Level: "task", DefinitionID: "def1", TaskRegex: "upstream*", Status: evergreen.TaskSucceeded, ConfigFile: "self-tests.yml", Alias: "a1"},
		},
	}
	assert.NoError(downstreamProjectRef.Insert())
	alias := model.ProjectAlias{
		ID:        mgobson.NewObjectId(),
		ProjectID: downstreamProjectRef.Identifier,
		Alias:     "a1",
		Variant:   "ubuntu1604",
		Task:      "test",
	}
	assert.NoError(alias.Upsert())
	_, err := model.GetNewRevisionOrderNumber(downstreamProjectRef.Identifier)
	assert.NoError(err)
	downstreamRevision := "abc123"
	assert.NoError(model.UpdateLastRevision(downstreamProjectRef.Identifier, downstreamRevision))

	downstreamVersions, err := EvalProjectTriggers(&e, TriggerDownstreamVersion)
	assert.NoError(err)
	dbVersions, err := model.VersionFind(model.VersionByProjectIdAndRevision(downstreamProjectRef.Identifier, downstreamRevision))
	assert.NoError(err)
	require.Len(downstreamVersions, 1)
	require.Len(dbVersions, 1)
	versions := []model.Version{downstreamVersions[0], dbVersions[0]}
	for _, v := range versions {
		assert.Equal("downstream_abc_def1", v.Id)
		assert.Equal(downstreamRevision, v.Revision)
		assert.Equal(evergreen.VersionCreated, v.Status)
		assert.Equal(downstreamProjectRef.Identifier, v.Identifier)
		assert.Equal(evergreen.TriggerRequester, v.Requester)
		assert.Equal(upstreamTask.Id, v.TriggerID)
		assert.Equal("task", v.TriggerType)
		assert.Equal(e.ID, v.TriggerEvent)
		assert.NotEmpty(v.Config)
	}
	builds, err := build.Find(build.ByVersion(downstreamVersions[0].Id))
	assert.NoError(err)
	assert.True(len(builds) > 0)
	for _, b := range builds {
		assert.Equal(downstreamProjectRef.Identifier, b.Project)
		assert.Equal(evergreen.TriggerRequester, b.Requester)
		assert.Equal(evergreen.BuildCreated, b.Status)
		assert.Equal(upstreamTask.Id, b.TriggerID)
		assert.Equal("task", b.TriggerType)
		assert.Equal(e.ID, b.TriggerEvent)
		assert.Contains(b.BuildVariant, "ubuntu1604")
	}
	tasks, err := task.Find(task.ByVersion(downstreamVersions[0].Id))
	assert.NoError(err)
	assert.True(len(tasks) > 0)
	for _, t := range tasks {
		assert.Equal(downstreamProjectRef.Identifier, t.Project)
		assert.Equal(evergreen.TriggerRequester, t.Requester)
		assert.Equal(evergreen.TaskUndispatched, t.Status)
		assert.Equal(upstreamTask.Id, t.TriggerID)
		assert.Equal("task", t.TriggerType)
		assert.Equal(e.ID, t.TriggerEvent)
		assert.Contains(t.DisplayName, "test")
	}

	// verify that triggering this version again does nothing
	upstreamVersionFromDB, err := model.VersionFindOneId(upstreamVersion.Id)
	assert.NoError(err)
	assert.Contains(upstreamVersionFromDB.SatisfiedTriggers, "def1")
	downstreamVersions, err = EvalProjectTriggers(&e, TriggerDownstreamVersion)
	assert.NoError(err)
	assert.Len(downstreamVersions, 0)
}
