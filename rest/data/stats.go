package data

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/taskstats"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

// GetTaskStats queries the service backend to retrieve the task stats that match the given filter.
func GetTaskStats(filter taskstats.StatsFilter) ([]restModel.APITaskStats, error) {
	if filter.Project != "" {
		projectID, err := model.GetIdForProject(filter.Project)
		if err != nil {
			return nil, errors.Wrapf(err, "getting project ID for project identifier '%s'", filter.Project)
		}
		filter.Project = projectID
	}

	serviceStatsResult, err := taskstats.GetTaskStats(filter)
	if err != nil {
		return nil, errors.Wrap(err, "getting task stats")
	}

	apiStatsResult := make([]restModel.APITaskStats, len(serviceStatsResult))
	for i, serviceStats := range serviceStatsResult {
		ats := restModel.APITaskStats{}
		ats.BuildFromService(serviceStats)
		apiStatsResult[i] = ats
	}
	return apiStatsResult, nil
}
