package model

import (
	"10gen.com/mci/model/host"
)

// TODO:
//	This is a temporary package for storing host-related interactions that involve
//	multiple models. We will break this up once MCI-2221 is finished.

// NextTaskForHost the next task that should be run on the host.
func NextTaskForHost(h *host.Host) (*Task, error) {
	taskQueue, err := FindTaskQueueForDistro(h.Distro)
	if err != nil {
		return nil, err
	}

	if taskQueue == nil || taskQueue.IsEmpty() {
		return nil, nil
	}

	nextTaskId := taskQueue.Queue[0].Id
	fullTask, err := FindTask(nextTaskId)
	if err != nil {
		return nil, err
	}

	return fullTask, nil
}
