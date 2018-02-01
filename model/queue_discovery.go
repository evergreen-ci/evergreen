package model

// TaskQueueAccessor is a wrapper for the TaskQueue type, to enable
// both mocking and different approaches to queue construction.
type TaskQueueAccessor interface {
	Length() int
	FindNextTask(TaskSpec) *TaskQueueItem
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
