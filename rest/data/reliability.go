package data

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/reliability"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

// GetTaskReliabilityScores queries the service backend to retrieve the task reliability scores that match the given filter.
func GetTaskReliabilityScores(filter reliability.TaskReliabilityFilter) ([]restModel.APITaskReliability, error) {
	if filter.Project != "" {
		projectID, err := model.GetIdForProject(filter.Project)
		if err != nil {
			return nil, errors.Wrapf(err, "getting project ref ID for identifier '%s'", filter.Project)
		}
		filter.Project = projectID
	}

	serviceStatsResult, err := reliability.GetTaskReliabilityScores(filter)
	if err != nil {
		return nil, errors.Wrap(err, "getting task reliability scores")
	}

	apiStatsResult := make([]restModel.APITaskReliability, len(serviceStatsResult))
	for i, serviceStats := range serviceStatsResult {
		ats := restModel.APITaskReliability{}
		err = ats.BuildFromService(&serviceStats)
		if err != nil {
			return nil, errors.Wrap(err, "converting task reliability stats to API model")
		}
		apiStatsResult[i] = ats
	}
	return apiStatsResult, nil
}
