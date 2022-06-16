package data

import (
	"context"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/stats"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

// GetTestStats queries the service backend to retrieve the test stats that match the given filter.
func GetTestStats(filter stats.StatsFilter) ([]restModel.APITestStats, error) {
	if filter.Project != "" {
		projectID, err := model.GetIdForProject(filter.Project)
		if err != nil {
			return nil, errors.Wrapf(err, "getting project ID for project identifier '%s'", filter.Project)
		}
		filter.Project = projectID
	}

	serviceStatsResult, err := stats.GetTestStats(filter)
	if err != nil {
		return nil, errors.Wrap(err, "getting test stats")
	}

	apiStatsResult := make([]restModel.APITestStats, len(serviceStatsResult))
	for i, serviceStats := range serviceStatsResult {
		ats := restModel.APITestStats{}
		err = ats.BuildFromService(&serviceStats)
		if err != nil {
			return nil, errors.Wrap(err, "converting test stats to API model")
		}
		apiStatsResult[i] = ats
	}
	return apiStatsResult, nil
}

// GetPrestoTestStats queries the Presto cluster to retrieve the test stats
// that match the given filter.
func GetPrestoTestStats(ctx context.Context, filter stats.PrestoTestStatsFilter) ([]restModel.APITestStats, error) {
	if filter.Project != "" {
		projectID, err := model.GetIdForProject(filter.Project)
		if err != nil {
			return nil, errors.Wrapf(err, "getting project ID for project identifier '%s'", filter.Project)
		}
		filter.Project = projectID
	}

	serviceStatsResult, err := stats.GetPrestoTestStats(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "getting Presto test stats")
	}

	apiStatsResult := make([]restModel.APITestStats, len(serviceStatsResult))
	for i, serviceStats := range serviceStatsResult {
		ats := restModel.APITestStats{}
		err = ats.BuildFromService(&serviceStats)
		if err != nil {
			return nil, errors.Wrap(err, "converting test stats to API model")
		}
		apiStatsResult[i] = ats
	}

	return apiStatsResult, nil
}

// GetTaskStats queries the service backend to retrieve the task stats that match the given filter.
func GetTaskStats(filter stats.StatsFilter) ([]restModel.APITaskStats, error) {
	if filter.Project != "" {
		projectID, err := model.GetIdForProject(filter.Project)
		if err != nil {
			return nil, errors.Wrapf(err, "getting project ID for project identifier '%s'", filter.Project)
		}
		filter.Project = projectID
	}

	serviceStatsResult, err := stats.GetTaskStats(filter)
	if err != nil {
		return nil, errors.Wrap(err, "getting task stats")
	}

	apiStatsResult := make([]restModel.APITaskStats, len(serviceStatsResult))
	for i, serviceStats := range serviceStatsResult {
		ats := restModel.APITaskStats{}
		err = ats.BuildFromService(&serviceStats)
		if err != nil {
			return nil, errors.Wrap(err, "converting task stats to API model")
		}
		apiStatsResult[i] = ats
	}
	return apiStatsResult, nil
}
