package data

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/stats"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

type StatsConnector struct{}

// GetTestStats queries the service backend to retrieve the test stats that match the given filter.
func (sc *StatsConnector) GetTestStats(filter stats.StatsFilter) ([]restModel.APITestStats, error) {
	if filter.Project != "" {
		projectID, err := model.GetIdForProject(filter.Project)
		if err != nil {
			return nil, errors.Wrapf(err, "getting project id for '%s'", filter.Project)
		}
		filter.Project = projectID
	}

	serviceStatsResult, err := stats.GetTestStats(filter)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get test stats from service API")
	}

	apiStatsResult := make([]restModel.APITestStats, len(serviceStatsResult))
	for i, serviceStats := range serviceStatsResult {
		ats := restModel.APITestStats{}
		err = ats.BuildFromService(&serviceStats)
		if err != nil {
			return nil, errors.Wrap(err, "Model error")
		}
		apiStatsResult[i] = ats
	}
	return apiStatsResult, nil
}

// GetTaskStats queries the service backend to retrieve the task stats that match the given filter.
func (sc *StatsConnector) GetTaskStats(filter stats.StatsFilter) ([]restModel.APITaskStats, error) {
	if filter.Project != "" {
		projectID, err := model.GetIdForProject(filter.Project)
		if err != nil {
			return nil, errors.Wrapf(err, "getting project id for '%s'", filter.Project)
		}
		filter.Project = projectID
	}

	serviceStatsResult, err := stats.GetTaskStats(filter)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get task stats from service API")
	}

	apiStatsResult := make([]restModel.APITaskStats, len(serviceStatsResult))
	for i, serviceStats := range serviceStatsResult {
		ats := restModel.APITaskStats{}
		err = ats.BuildFromService(&serviceStats)
		if err != nil {
			return nil, errors.Wrap(err, "Model error")
		}
		apiStatsResult[i] = ats
	}
	return apiStatsResult, nil
}
