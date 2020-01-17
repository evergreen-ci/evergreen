package data

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/evergreen/util"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

type TaskReliabilityConnector struct{}

// GetTaskReliabilityScores queries the service backend to retrieve the task reliability scores that match the given filter.
func (sc *TaskReliabilityConnector) GetTaskReliabilityScores(filter reliability.TaskReliabilityFilter) ([]model.APITaskReliability, error) {
	serviceStatsResult, err := reliability.GetTaskReliabilityScores(filter)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get task stats from service API")
	}

	apiStatsResult := make([]model.APITaskReliability, len(serviceStatsResult))
	for i, serviceStats := range serviceStatsResult {
		ats := model.APITaskReliability{}
		err = ats.BuildFromService(&serviceStats)
		if err != nil {
			return nil, errors.Wrap(err, "Model error")
		}
		apiStatsResult[i] = ats
	}
	return apiStatsResult, nil
}

type MockTaskReliabilityConnector struct {
	CachedTaskReliability []model.APITaskReliability
}

// GetTaskReliabilityScores returns the cached task stats, only enforcing the Limit field of the filter.
func (msc *MockTaskReliabilityConnector) GetTaskReliabilityScores(filter reliability.TaskReliabilityFilter) ([]model.APITaskReliability, error) {
	if filter.Limit > len(msc.CachedTaskReliability) {
		return msc.CachedTaskReliability, nil
	} else {
		return msc.CachedTaskReliability[:filter.Limit], nil
	}
}

// SetTaskReliabilityScores sets the cached task stats by generating 'numStats' stats.
func (msc *MockTaskReliabilityConnector) SetTaskReliabilityScores(baseTaskName string, numStats int) {
	msc.CachedTaskReliability = make([]model.APITaskReliability, numStats)
	day := util.GetUTCDay(time.Now()).Format("2006-01-02")
	for i := 0; i < numStats; i++ {
		msc.CachedTaskReliability[i] = model.APITaskReliability{
			TaskName:     model.ToStringPtr(fmt.Sprintf("%v%v", baseTaskName, i)),
			BuildVariant: model.ToStringPtr("variant"),
			Distro:       model.ToStringPtr("distro"),
			Date:         model.ToStringPtr(day),
		}
	}
}
