package data

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/reliability"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

type TaskReliabilityConnector struct{}

// GetTaskReliabilityScores queries the service backend to retrieve the task reliability scores that match the given filter.
func (sc *TaskReliabilityConnector) GetTaskReliabilityScores(filter reliability.TaskReliabilityFilter) ([]restModel.APITaskReliability, error) {
	if filter.Project != "" {
		projectID, err := model.GetIdForProject(filter.Project)
		if err != nil {
			return nil, errors.Wrapf(err, "getting project id for '%s'", filter.Project)
		}
		filter.Project = projectID
	}

	serviceStatsResult, err := reliability.GetTaskReliabilityScores(filter)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get task stats from service API")
	}

	apiStatsResult := make([]restModel.APITaskReliability, len(serviceStatsResult))
	for i, serviceStats := range serviceStatsResult {
		ats := restModel.APITaskReliability{}
		err = ats.BuildFromService(&serviceStats)
		if err != nil {
			return nil, errors.Wrap(err, "Model error")
		}
		apiStatsResult[i] = ats
	}
	return apiStatsResult, nil
}

type MockTaskReliabilityConnector struct {
	CachedTaskReliability []restModel.APITaskReliability
}

// GetTaskReliabilityScores returns the cached task stats, only enforcing the Limit field of the filter.
func (msc *MockTaskReliabilityConnector) GetTaskReliabilityScores(filter reliability.TaskReliabilityFilter) ([]restModel.APITaskReliability, error) {
	if filter.Limit > len(msc.CachedTaskReliability) {
		return msc.CachedTaskReliability, nil
	} else {
		return msc.CachedTaskReliability[:filter.Limit], nil
	}
}

// SetTaskReliabilityScores sets the cached task stats by generating 'numStats' stats.
func (msc *MockTaskReliabilityConnector) SetTaskReliabilityScores(baseTaskName string, numStats int) {
	msc.CachedTaskReliability = make([]restModel.APITaskReliability, numStats)
	day := utility.GetUTCDay(time.Now()).Format("2006-01-02")
	for i := 0; i < numStats; i++ {
		msc.CachedTaskReliability[i] = restModel.APITaskReliability{
			TaskName:     utility.ToStringPtr(fmt.Sprintf("%v%v", baseTaskName, i)),
			BuildVariant: utility.ToStringPtr("variant"),
			Distro:       utility.ToStringPtr("distro"),
			Date:         utility.ToStringPtr(day),
		}
	}
}
