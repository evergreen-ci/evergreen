package data

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/reliability"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

type TaskReliabilityConnector struct{}

// GetTaskReliabilityScores queries the service backend to retrieve the task reliability scores that match the given filter.
func (sc *TaskReliabilityConnector) GetTaskReliabilityScores(filter reliability.TaskReliabilityFilter) ([]restModel.APITaskReliability, error) {
	if filter.Project != "" {
		projectID, err := model.GetIdForProject(filter.Project)
		if err != nil {
			return nil, errors.Wrapf(err, "getting project id for '%s'", filter.Project)
		}
		filter.Project = projectID
	}

	serviceStatsResult, err := reliability.GetTaskReliabilityScores(filter)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get task stats from service API")
	}

	apiStatsResult := make([]restModel.APITaskReliability, len(serviceStatsResult))
	for i, serviceStats := range serviceStatsResult {
		ats := restModel.APITaskReliability{}
		err = ats.BuildFromService(&serviceStats)
		if err != nil {
			return nil, errors.Wrap(err, "Model error")
		}
		apiStatsResult[i] = ats
	}
	return apiStatsResult, nil
}
