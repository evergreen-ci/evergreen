package mock

import "github.com/evergreen-ci/evergreen/model"

type TaskQueueAccessor struct {
	LenghtValue      int
	QueueItem        *model.TaskQueueItem
	SpecInput        model.TaskSpec
	SaveError        error
	DequeuedTasks    []string
	DequeueTaskError error
}

var _ model.TaskQueueAccessor = &TaskQueueAccessor{}

func (q *TaskQueueAccessor) Length() int { return q.LenghtValue }
func (q *TaskQueueAccessor) Save() error { return q.SaveError }
func (q *TaskQueueAccessor) FindNextTask(spec model.TaskSpec) *model.TaskQueueItem {
	q.SpecInput = spec
	return q.QueueItem
}

func (q *TaskQueueAccessor) DequeueTask(t string) error {
	q.DequeuedTasks = append(q.DequeuedTasks, t)
	return q.DequeueTaskError
}
