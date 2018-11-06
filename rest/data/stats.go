package data

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/evergreen/util"
)

type StatsConnector struct{}

// GetTestStats queries the service backend to retrieve the test stats that match the given filter.
func (sc *StatsConnector) GetTestStats(filter *stats.StatsFilter) ([]stats.TestStats, error) {
	return stats.GetTestStats(filter)
}

type MockStatsConnector struct {
	CachedTestStats []stats.TestStats
}

// GetTestStats returns the cached test stats, only enforcing the Limit field of the filter.
func (msc *MockStatsConnector) GetTestStats(filter *stats.StatsFilter) ([]stats.TestStats, error) {
	if filter.Limit > len(msc.CachedTestStats) {
		return msc.CachedTestStats, nil
	} else {
		return msc.CachedTestStats[:filter.Limit], nil
	}
}

// SetTestStats sets the cached test stats by generating 'numStats' stats.
func (msc *MockStatsConnector) SetTestStats(baseTestName string, numStats int) {
	msc.CachedTestStats = make([]stats.TestStats, numStats)
	day := util.GetUTCDay(time.Now())
	for i := 0; i < numStats; i++ {
		msc.CachedTestStats[i] = stats.TestStats{
			TestFile:     fmt.Sprintf("%v%v", baseTestName, i),
			TaskName:     "task",
			BuildVariant: "variant",
			Distro:       "distro",
			Date:         day,
		}
	}
}
