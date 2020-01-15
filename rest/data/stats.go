package data

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
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

type MockStatsConnector struct {
	CachedTestStats []model.APITestStats
	CachedTaskStats []model.APITaskStats
}

// GetTestStats returns the cached test stats, only enforcing the Limit field of the filter.
func (msc *MockStatsConnector) GetTestStats(filter stats.StatsFilter) ([]model.APITestStats, error) {
	if filter.Limit > len(msc.CachedTestStats) {
		return msc.CachedTestStats, nil
	} else {
		return msc.CachedTestStats[:filter.Limit], nil
	}
}

// GetTaskStats returns the cached task stats, only enforcing the Limit field of the filter.
func (msc *MockStatsConnector) GetTaskStats(filter stats.StatsFilter) ([]model.APITaskStats, error) {
	if filter.Limit > len(msc.CachedTaskStats) {
		return msc.CachedTaskStats, nil
	} else {
		return msc.CachedTaskStats[:filter.Limit], nil
	}
}

// SetTestStats sets the cached test stats by generating 'numStats' stats.
func (msc *MockStatsConnector) SetTestStats(baseTestName string, numStats int) {
	msc.CachedTestStats = make([]model.APITestStats, numStats)
	day := util.GetUTCDay(time.Now()).Format("2006-01-02")
	for i := 0; i < numStats; i++ {
		msc.CachedTestStats[i] = model.APITestStats{
			TestFile:     model.ToStringPtr(fmt.Sprintf("%v%v", baseTestName, i)),
			TaskName:     model.ToStringPtr("task"),
			BuildVariant: model.ToStringPtr("variant"),
			Distro:       model.ToStringPtr("distro"),
			Date:         model.ToStringPtr(day),
		}
	}
}

// SetTaskStats sets the cached task stats by generating 'numStats' stats.
func (msc *MockStatsConnector) SetTaskStats(baseTaskName string, numStats int) {
	msc.CachedTaskStats = make([]model.APITaskStats, numStats)
	day := util.GetUTCDay(time.Now()).Format("2006-01-02")
	for i := 0; i < numStats; i++ {
		msc.CachedTaskStats[i] = model.APITaskStats{
			TaskName:     model.ToStringPtr(fmt.Sprintf("%v%v", baseTaskName, i)),
			BuildVariant: model.ToStringPtr("variant"),
			Distro:       model.ToStringPtr("distro"),
			Date:         model.ToStringPtr(day),
		}
	}
}
