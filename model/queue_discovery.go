package model

import "github.com/pkg/errors"

//

type TaskQueueAccessor interface {
	Length() int
	NextTask() TaskQueueItem
	FindTask(TaskSpec) TaskQueueItem
	Save() error
	DequeueTask(string) error
}

func FetchQueue(distro string) (TaskQueueAccessor, error) {
	queue, err := FindTaskQueueForDistro(distro)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return queue, nil
}

type TaskSpec struct {
	GroupName    string
	BuildVariant string
	ProjectID    string
	Version      string
}
