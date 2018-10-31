package data

import (
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

type DBProjectTriggersConnector struct{}

func (c *DBProjectTriggersConnector) TriggerDataFromTask(taskID string) (model.UpstreamMetadata, error) {
	metadata := model.UpstreamMetadata{}
	downstreamTask, err := task.FindOneId(taskID)
	if err != nil {
		return metadata, errors.Wrap(err, "error finding task")
	}
	if downstreamTask == nil {
		return metadata, errors.Errorf("unable to find task '%s'", taskID)
	}

	var upstreamTask *task.Task
	var upstreamBuild *build.Build
	if downstreamTask.TriggerType == dbModel.ProjectTriggerLevelTask {
		upstreamTask, err = task.FindOneId(downstreamTask.TriggerID)
		if err != nil {
			return metadata, errors.Wrap(err, "error finding task")
		}
		upstreamBuild, err = build.FindOneId(upstreamTask.BuildId)
		if err != nil {
			return metadata, errors.Wrap(err, "error finding build")
		}
		metadata.Task = &model.APITask{}
		err = metadata.Task.BuildFromService(upstreamTask)
		if err != nil {
			return metadata, err
		}
	} else if downstreamTask.TriggerType == dbModel.ProjectTriggerLevelBuild {
		upstreamBuild, err = build.FindOneId(downstreamTask.TriggerID)
		if err != nil {
			return metadata, errors.Wrap(err, "error finding build")
		}
	}
	if upstreamBuild == nil {
		return metadata, errors.New("unable to find upstream build")
	}
	projectRef, err := dbModel.FindOneProjectRef(upstreamBuild.Project)
	if err != nil {
		return metadata, errors.Wrap(err, "error finding project ref")
	}
	if projectRef == nil {
		return metadata, errors.New("project ref not found")
	}
	err = metadata.Build.BuildFromService(*upstreamBuild)
	if err != nil {
		return metadata, err
	}
	err = metadata.Project.BuildFromService(*projectRef)
	if err != nil {
		return metadata, err
	}
	metadata.EventID = model.ToAPIString(downstreamTask.TriggerEvent)

	return metadata, nil
}

type MockProjectTriggersConnector struct{}

func (c *MockProjectTriggersConnector) TriggerDataFromTask(taskID string) (model.UpstreamMetadata, error) {
	return model.UpstreamMetadata{}, nil
}
