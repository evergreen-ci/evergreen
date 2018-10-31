package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/assert"
)

func TestTriggerDataFromTask(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, build.Collection, model.ProjectRefCollection))
	downstreamTask := task.Task{
		Id:           "d",
		TriggerID:    "u",
		TriggerType:  model.ProjectTriggerLevelTask,
		TriggerEvent: "e",
	}
	assert.NoError(downstreamTask.Insert())
	upstreamTask := task.Task{
		Id:      "u",
		BuildId: "ub",
		Project: "proj",
	}
	assert.NoError(upstreamTask.Insert())
	upstreamBuild := build.Build{
		Id:      "ub",
		Project: "proj",
	}
	assert.NoError(upstreamBuild.Insert())
	proj := model.ProjectRef{
		Identifier: "proj",
	}
	assert.NoError(proj.Insert())
	dc := DBProjectTriggersConnector{}

	metadata, err := dc.TriggerDataFromTask(downstreamTask.Id)
	assert.NoError(err)
	assert.Equal(downstreamTask.TriggerEvent, restModel.FromAPIString(metadata.EventID))
	assert.Equal(upstreamTask.Id, restModel.FromAPIString(metadata.Task.Id))
	assert.Equal(upstreamBuild.Id, restModel.FromAPIString(metadata.Build.Id))
	assert.Equal(proj.Identifier, restModel.FromAPIString(metadata.Project.Identifier))
}
