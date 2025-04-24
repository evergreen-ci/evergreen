package data

import (
	"context"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/reliability"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

// GetTaskReliabilityScores queries the service backend to retrieve the task reliability scores that match the given filter.
func GetTaskReliabilityScores(ctx context.Context, filter reliability.TaskReliabilityFilter) ([]restModel.APITaskReliability, error) {
	if filter.Project != "" {
		projectID, err := model.GetIdForProject(ctx, filter.Project)
		if err != nil {
			return nil, errors.Wrapf(err, "getting project ref ID for identifier '%s'", filter.Project)
		}
		filter.Project = projectID
	}

	serviceStatsResult, err := reliability.GetTaskReliabilityScores(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "getting task reliability scores")
	}

	apiStatsResult := make([]restModel.APITaskReliability, len(serviceStatsResult))
	for i, serviceStats := range serviceStatsResult {
		ats := restModel.APITaskReliability{}
		ats.BuildFromService(serviceStats)
		apiStatsResult[i] = ats
	}
	return apiStatsResult, nil
}
