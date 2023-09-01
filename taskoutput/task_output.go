package taskoutput

import (
	"github.com/evergreen-ci/evergreen/taskoutput/tasklogs"
	"github.com/evergreen-ci/evergreen/taskoutput/testlogs"
)

// TaskOutput is the versioned entry point for coordinating persistent storage
// of a task run's output data. The int value represents the centralized
// version of the task output, which then maps to the underlying versions of
// each recognized output type collected during a task run.
type TaskOutput int

// TaskLogs returns the versioned entry point of the task logs output type.
func (o TaskOutput) TaskLogs() tasklogs.TaskLogs {
	switch {
	case o >= 0:
		return tasklogs.TaskLogs(0)
	default:
		return tasklogs.TaskLogs(-1)
	}
}

// TestLogs returns the versioned entry point of the test logs output type.
func (o TaskOutput) TestLogs() testlogs.TestLogs {
	switch {
	case o >= 0:
		return testlogs.TestLogs(0)
	default:
		return testlogs.TestLogs(-1)
	}
}
