package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
)

const (
	// if we have no data on a given task, default to 10 minutes so we
	// have some new hosts spawned
	DefaultTaskDuration = time.Duration(10) * time.Minute

	// for the UI, if we have no data on a given task, we want to default to
	// 0 time
	UnknownTaskDuration = time.Duration(0)

	// indicates the window of completed tasks we want to use in computing
	// average task duration. By default we use tasks that have
	// completed within the last 7 days
	TaskCompletionEstimateWindow = time.Duration(24*7) * time.Hour
)

// ProjectTaskDurations maintans a mapping of a given project's name
// and its accompanying BuildVariantTaskDurations
type ProjectTaskDurations struct {
	TaskDurationByProject map[string]*BuildVariantTaskDurations
}

// BuildVariantTaskDurations maintains a mapping between a buildvariant
// and its accompanying TaskDurations
type BuildVariantTaskDurations struct {
	TaskDurationByBuildVariant map[string]*TaskDurations
}

// TaskDurations maintains a mapping between a given task (by display name)
// and its accompanying aggregate expected duration
type TaskDurations struct {
	TaskDurationByDisplayName map[string]time.Duration
}

// GetTaskExpectedDuration returns the expected duration for a given task
// if the task does not exist, it returns model.DefaultTaskDuration
func GetTaskExpectedDuration(task task.Task, allDurations ProjectTaskDurations) time.Duration {
	projectDur, ok := allDurations.TaskDurationByProject[task.Project]
	if ok {
		buildVariantDur := projectDur.TaskDurationByBuildVariant
		projectDur, ok := buildVariantDur[task.BuildVariant]
		if ok {
			taskDur := projectDur.TaskDurationByDisplayName
			duration, ok := taskDur[task.DisplayName]
			if ok {
				return duration
			}
		}
	}
	return DefaultTaskDuration
}
