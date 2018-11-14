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
func (sc *StatsConnector) GetTestStats(filter *stats.StatsFilter) ([]model.APITestStats, error) {
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

type MockStatsConnector struct {
	CachedTestStats []model.APITestStats
}

// GetTestStats returns the cached test stats, only enforcing the Limit field of the filter.
func (msc *MockStatsConnector) GetTestStats(filter *stats.StatsFilter) ([]model.APITestStats, error) {
	if filter.Limit > len(msc.CachedTestStats) {
		return msc.CachedTestStats, nil
	} else {
		return msc.CachedTestStats[:filter.Limit], nil
	}
}

// SetTestStats sets the cached test stats by generating 'numStats' stats.
func (msc *MockStatsConnector) SetTestStats(baseTestName string, numStats int) {
	msc.CachedTestStats = make([]model.APITestStats, numStats)
	day := util.GetUTCDay(time.Now()).Format("2006-01-02")
	for i := 0; i < numStats; i++ {
		msc.CachedTestStats[i] = model.APITestStats{
			TestFile:     fmt.Sprintf("%v%v", baseTestName, i),
			TaskName:     "task",
			BuildVariant: "variant",
			Distro:       "distro",
			Date:         day,
		}
	}
}
