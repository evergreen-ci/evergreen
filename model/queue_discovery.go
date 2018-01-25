package model

//

type TaskQueueAccessor interface {
	Length() int
	NextTask() TaskQueueItem
	FindTask(TaskSpec)
	Save() error
	DequeueTask(string) error
}

func NewDBQueue(distro string) {}

type TaskSpec struct {
	GroupName    string
	BuildVariant string
	ProjectID    string
	Version      string
}
