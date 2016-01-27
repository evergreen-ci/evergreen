package scheduler

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/model"
)

// TaskDurationEstimator is responsible for fetching the expected duration for a
// given set of runnable tasks.
type TaskDurationEstimator interface {
	GetExpectedDurations(runnableTasks []model.Task) (
		model.ProjectTaskDurations, error)
}

// DBTaskDurationEstimator retrives the estimated duration of runnable tasks.
// Implements TaskDurationEstimator.
type DBTaskDurationEstimator struct{}

// GetExpectedDurations returns the expected duration of tasks
// (by display name) on a project, buildvariant basis.
func (self *DBTaskDurationEstimator) GetExpectedDurations(
	runnableTasks []model.Task) (model.ProjectTaskDurations, error) {
	durations := model.ProjectTaskDurations{}

	// get the average task duration for all the runnable tasks
	for _, task := range runnableTasks {
		if durations.TaskDurationByProject == nil {
			durations.TaskDurationByProject =
				make(map[string]*model.BuildVariantTaskDurations)
		}

		_, ok := durations.TaskDurationByProject[task.Project]
		if !ok {
			durations.TaskDurationByProject[task.Project] =
				&model.BuildVariantTaskDurations{}
		}

		projectDurations := durations.TaskDurationByProject[task.Project]
		if projectDurations.TaskDurationByBuildVariant == nil {
			durations.TaskDurationByProject[task.Project].
				TaskDurationByBuildVariant = make(map[string]*model.TaskDurations)
		}

		_, ok = projectDurations.TaskDurationByBuildVariant[task.BuildVariant]
		if !ok {
			expTaskDurationByDisplayName, err := model.ExpectedTaskDuration(
				task.Project, task.BuildVariant,
				model.TaskCompletionEstimateWindow)
			if err != nil {
				return durations, fmt.Errorf("Error fetching "+
					"expected task duration for %v on %v: %v",
					task.BuildVariant, task.Project, err)
			}
			durations.TaskDurationByProject[task.Project].
				TaskDurationByBuildVariant[task.BuildVariant] =
				&model.TaskDurations{expTaskDurationByDisplayName}
		}
	}
	return durations, nil
}
