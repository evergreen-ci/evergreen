package monitor

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"time"
)

const (
	// reasons for cleaning up a task
	HeartbeatTimeout = "task heartbeat timed out"
)

var (
	// threshold for a task's heartbeat to time out
	HeartbeatTimeoutThreshold = 7 * time.Minute
)

// function that spits out a list of tasks that need to be stopped
// and cleaned up
type taskFlaggingFunc func() ([]doomedTaskWrapper, error)

// wrapper for a task to be cleaned up. contains the task, as well as the
// reason it is being cleaned up
type doomedTaskWrapper struct {
	// the task to be cleaned up
	task task.Task
	// why the task is being cleaned up
	reason string
}

// flagTimedOutHeartbeats is a taskFlaggingFunc to flag any tasks whose
// heartbeats have timed out
func flagTimedOutHeartbeats() ([]doomedTaskWrapper, error) {

	evergreen.Logger.Logf(slogger.INFO, "Finding tasks with timed-out heartbeats...")

	// fetch any running tasks whose last heartbeat was too long in the past
	threshold := time.Now().Add(-HeartbeatTimeoutThreshold)

	tasks, err := task.Find(task.ByRunningLastHeartbeat(threshold))
	if err != nil {
		return nil, fmt.Errorf("error finding tasks with timed-out"+
			" heartbeats: %v", err)
	}

	// convert to be returned
	wrappers := make([]doomedTaskWrapper, 0, len(tasks))

	for _, task := range tasks {
		wrappers = append(wrappers, doomedTaskWrapper{task, HeartbeatTimeout})
	}

	evergreen.Logger.Logf(slogger.INFO, "Found %v tasks whose heartbeats timed out",
		len(wrappers))

	return wrappers, nil
}
