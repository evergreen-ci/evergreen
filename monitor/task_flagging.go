package monitor

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
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
	// fetch any running tasks whose last heartbeat was too long in the past
	threshold := time.Now().Add(-HeartbeatTimeoutThreshold)

	tasks, err := task.Find(task.ByRunningLastHeartbeat(threshold))
	if err != nil {
		return nil, errors.Wrap(err,
			"error finding tasks with timed-out heartbeats")
	}

	// convert to be returned
	wrappers := make([]doomedTaskWrapper, 0, len(tasks))

	for _, task := range tasks {
		wrappers = append(wrappers, doomedTaskWrapper{task, HeartbeatTimeout})
	}

	grip.Info(message.Fields{
		"runner":  RunnerName,
		"message": "Found tasks with timed out heartbeats",
		"count":   len(wrappers),
	})

	return wrappers, nil
}
