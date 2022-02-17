package data

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/stats"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
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

type MockStatsConnector struct {
	CachedTestStats []restModel.APITestStats
	CachedTaskStats []restModel.APITaskStats
}

// GetTestStats returns the cached test stats, only enforcing the Limit field of the filter.
func (msc *MockStatsConnector) GetTestStats(filter stats.StatsFilter) ([]restModel.APITestStats, error) {
	if filter.Limit > len(msc.CachedTestStats) {
		return msc.CachedTestStats, nil
	} else {
		return msc.CachedTestStats[:filter.Limit], nil
	}
}

// GetTaskStats returns the cached task stats, only enforcing the Limit field of the filter.
func (msc *MockStatsConnector) GetTaskStats(filter stats.StatsFilter) ([]restModel.APITaskStats, error) {
	if filter.Limit > len(msc.CachedTaskStats) {
		return msc.CachedTaskStats, nil
	} else {
		return msc.CachedTaskStats[:filter.Limit], nil
	}
}

// SetTestStats sets the cached test stats by generating 'numStats' stats.
func (msc *MockStatsConnector) SetTestStats(baseTestName string, numStats int) {
	msc.CachedTestStats = make([]restModel.APITestStats, numStats)
	day := utility.GetUTCDay(time.Now()).Format("2006-01-02")
	for i := 0; i < numStats; i++ {
		msc.CachedTestStats[i] = restModel.APITestStats{
			TestFile:     utility.ToStringPtr(fmt.Sprintf("%v%v", baseTestName, i)),
			TaskName:     utility.ToStringPtr("task"),
			BuildVariant: utility.ToStringPtr("variant"),
			Distro:       utility.ToStringPtr("distro"),
			Date:         utility.ToStringPtr(day),
		}
	}
}

// SetTaskStats sets the cached task stats by generating 'numStats' stats.
func (msc *MockStatsConnector) SetTaskStats(baseTaskName string, numStats int) {
	msc.CachedTaskStats = make([]restModel.APITaskStats, numStats)
	day := utility.GetUTCDay(time.Now()).Format("2006-01-02")
	for i := 0; i < numStats; i++ {
		msc.CachedTaskStats[i] = restModel.APITaskStats{
			TaskName:     utility.ToStringPtr(fmt.Sprintf("%v%v", baseTaskName, i)),
			BuildVariant: utility.ToStringPtr("variant"),
			Distro:       utility.ToStringPtr("distro"),
			Date:         utility.ToStringPtr(day),
		}
	}
}
