package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
)

// dependencyPath represents the path of tasks that can
// occur by taking one from each layer of the dependencies
// TotalTime is the sum of all task's time taken to run that are in Tasks.
type dependencyPath struct {
	TaskId    string
	TotalTime time.Duration
	Tasks     []string
}

// CalculateActualMakespan finds the amount of time it took for the build to complete from
// the first task start to the last task finishing.
func CalculateActualMakespan(tasks []task.Task) time.Duration {
	// find the minimum start time and the maximum finish time and take the difference
	if len(tasks) == 0 {
		return time.Duration(0)
	}

	minStart := tasks[0].StartTime
	maxFinish := tasks[0].FinishTime

	for _, t := range tasks {
		if t.StartTime.Before(minStart) {
			minStart = t.StartTime
		}
		if t.FinishTime.After(maxFinish) {
			maxFinish = t.FinishTime
		}
	}
	return maxFinish.Sub(minStart)
}

// hasTaskId returns true if the dependency list has the task
func hasTaskId(taskId string, dependsOn []task.Dependency) bool {
	for _, d := range dependsOn {
		if d.TaskId == taskId {
			return true
		}
	}
	return false
}

// getMaxDependencyPath recursively traverses a task's dependencies to get the dependency path object with the maximum
// total time.
func getMaxDependencyPath(tasks []task.Task, depPath dependencyPath) dependencyPath {
	maxDepPath := depPath
	maxTime := time.Duration(0)
	// find tasks that depend on the current task in the depPath
	for _, t := range tasks {
		if hasTaskId(depPath.TaskId, t.DependsOn) {
			newDepPath := dependencyPath{
				TaskId:    t.Id,
				Tasks:     append(depPath.Tasks, t.Id),
				TotalTime: depPath.TotalTime + t.TimeTaken,
			}
			newDepPath = getMaxDependencyPath(tasks, newDepPath)
			if newDepPath.TotalTime > maxTime {
				maxTime = newDepPath.TotalTime
				maxDepPath = newDepPath
			}
		}
	}
	return maxDepPath
}

// FindPredictedMakespan, given a list of tasks that have been completed, finds the optimal makespan of that build.
// While it's possible for tasks to depend on tasks outside its build, this function does not take that
// into account because it is meant to compute the optimal makespan for a single build
func FindPredictedMakespan(tasks []task.Task) dependencyPath {
	maxTime := time.Duration(0)
	var maxDepPath dependencyPath

	for _, t := range tasks {
		if len(t.DependsOn) == 0 {
			depPath := dependencyPath{
				TaskId:    t.Id,
				Tasks:     []string{t.Id},
				TotalTime: t.TimeTaken,
			}
			fullDepPath := getMaxDependencyPath(tasks, depPath)
			if fullDepPath.TotalTime > maxTime {
				maxTime = fullDepPath.TotalTime
				maxDepPath = fullDepPath
			}
		}
	}
	return maxDepPath
}
