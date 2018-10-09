package trigger

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/stretchr/testify/suite"
)

type projectTriggerSuite struct {
	suite.Suite
	processor projectProcessor
}

func mockTriggerVersion(args ProcessorArgs) (*version.Version, error) {
	// we're putting the input params into arbitrary fields of the struct so that the tests can inspect them
	v := version.Version{
		Branch:      args.DownstreamProject.Identifier,
		Config:      args.File,
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
	s.NoError(db.ClearCollections(task.Collection, build.Collection, version.Collection))
	t := task.Task{
		Id:          "task",
		Project:     "toTrigger",
		DisplayName: "taskName",
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
	v := version.Version{
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
