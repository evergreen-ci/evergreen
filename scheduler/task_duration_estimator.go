package scheduler

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
)

// TaskDurationEstimator is responsible for fetching the expected duration for a
// given set of runnable tasks.
type TaskDurationEstimator interface {
	GetExpectedDurations(runnableTasks []task.Task) (
		model.ProjectTaskDurations, error)
}

// DBTaskDurationEstimator retrives the estimated duration of runnable tasks.
// Implements TaskDurationEstimator.
type DBTaskDurationEstimator struct{}

// GetExpectedDurations returns the expected duration of tasks
// (by display name) on a project, buildvariant basis.
func (self *DBTaskDurationEstimator) GetExpectedDurations(
	runnableTasks []task.Task) (model.ProjectTaskDurations, error) {
	durations := model.ProjectTaskDurations{}

	// get the average task duration for all the runnable tasks
	for _, t := range runnableTasks {
		if durations.TaskDurationByProject == nil {
			durations.TaskDurationByProject =
				make(map[string]*model.BuildVariantTaskDurations)
		}

		_, ok := durations.TaskDurationByProject[t.Project]
		if !ok {
			durations.TaskDurationByProject[t.Project] =
				&model.BuildVariantTaskDurations{}
		}

		projectDurations := durations.TaskDurationByProject[t.Project]
		if projectDurations.TaskDurationByBuildVariant == nil {
			durations.TaskDurationByProject[t.Project].
				TaskDurationByBuildVariant = make(map[string]*model.TaskDurations)
		}

		_, ok = projectDurations.TaskDurationByBuildVariant[t.BuildVariant]
		if !ok {
			expTaskDurationByDisplayName, err := task.ExpectedTaskDuration(
				t.Project, t.BuildVariant,
				model.TaskCompletionEstimateWindow)
			if err != nil {
				return durations, fmt.Errorf("Error fetching "+
					"expected task duration for %v on %v: %v",
					t.BuildVariant, t.Project, err)
			}
			durations.TaskDurationByProject[t.Project].
				TaskDurationByBuildVariant[t.BuildVariant] =
				&model.TaskDurations{expTaskDurationByDisplayName}
		}
	}
	return durations, nil
}
