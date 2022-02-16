package data

import (
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

type TaskReliabilityConnector struct{}

// GetTaskReliabilityScores queries the service backend to retrieve the task reliability scores that match the given filter.
func (sc *TaskReliabilityConnector) GetTaskReliabilityScores(filter reliability.TaskReliabilityFilter) ([]model.APITaskReliability, error) {
	serviceStatsResult, err := reliability.GetTaskReliabilityScores(filter)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get task stats from service API")
	}

	apiStatsResult := make([]model.APITaskReliability, len(serviceStatsResult))
	for i, serviceStats := range serviceStatsResult {
		ats := model.APITaskReliability{}
		err = ats.BuildFromService(&serviceStats)
		if err != nil {
			return nil, errors.Wrap(err, "Model error")
		}
		apiStatsResult[i] = ats
	}
	return apiStatsResult, nil
}
