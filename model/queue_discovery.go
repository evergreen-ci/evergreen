package model

// TaskQueueAccessor is a wrapper for the TaskQueue type, to enable
// both mocking and different approaches to queue construction.
type TaskQueueAccessor interface {
	Length() int
	NextTask() *TaskQueueItem
	FindTask(TaskSpec) *TaskQueueItem
	Save() error
	DequeueTask(string) error
}

// TaskSpec is an argument structure to formalize the way that callers
// may query/select a task from an existing task queue to support
// out-of-order task execution for the purpose of task-groups.
type TaskSpec struct {
	Group         string
	BuildVariant  string
	ProjectID     string
	Version       string
	GroupMaxHosts int
}

// MatchingOrNextTask provides a generic implementation of logic to
// support selecting either a task matching the given task spec or the
// next task in the queue if none matches.
//
// Returns nil if the queue is empty.
func MatchingOrNextTask(queue TaskQueueAccessor, spec TaskSpec) *TaskQueueItem {
	if queue.Length() == 0 {
		return nil
	}
	if it := queue.FindTask(spec); it != nil {
		return it
	}
	return queue.NextTask()
}
