package model

//

type TaskQueueAccessor interface {
	Length() int
	NextTask() *TaskQueueItem
	FindTask(TaskSpec) *TaskQueueItem
	Save() error
	DequeueTask(string) error
}

type TaskSpec struct {
	GroupName    string
	BuildVariant string
	ProjectID    string
	Version      string
}

func MatchingOrNextTask(queue TaskQueueAccessor, spec TaskSpec) *TaskQueueItem {
	if queue.Length() == 0 {
		return nil
	}

	it := queue.FindTask(spec)
	if it == nil {
		it = queue.NextTask()
	}

	return it
}
