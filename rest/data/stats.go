package data

import (
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

type StatsConnector struct{}

// GetTestStats queries the service backend to retrieve the test stats that match the given filter.
func (sc *StatsConnector) GetTestStats(filter stats.StatsFilter) ([]model.APITestStats, error) {
	serviceStatsResult, err := stats.GetTestStats(filter)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get test stats from service API")
	}

	apiStatsResult := make([]model.APITestStats, len(serviceStatsResult))
	for i, serviceStats := range serviceStatsResult {
		ats := model.APITestStats{}
		err = ats.BuildFromService(&serviceStats)
		if err != nil {
			return nil, errors.Wrap(err, "Model error")
		}
		apiStatsResult[i] = ats
	}
	return apiStatsResult, nil
}

// GetTaskStats queries the service backend to retrieve the task stats that match the given filter.
func (sc *StatsConnector) GetTaskStats(filter stats.StatsFilter) ([]model.APITaskStats, error) {
	serviceStatsResult, err := stats.GetTaskStats(filter)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get task stats from service API")
	}

	apiStatsResult := make([]model.APITaskStats, len(serviceStatsResult))
	for i, serviceStats := range serviceStatsResult {
		ats := model.APITaskStats{}
		err = ats.BuildFromService(&serviceStats)
		if err != nil {
			return nil, errors.Wrap(err, "Model error")
		}
		apiStatsResult[i] = ats
	}
	return apiStatsResult, nil
}
