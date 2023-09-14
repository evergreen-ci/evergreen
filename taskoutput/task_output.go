package taskoutput

// TaskOutput is the versioned entry point for coordinating persistent storage
// of a task run's output data.
type TaskOutput struct {
	TaskLogs TaskLogOutput `bson:"task_logs" json:"task_logs"`
	TestLogs TestLogOutput `bson:"test_logs" json:"test_logs"`
}

// TaskOptions represents the task-level information required for accessing
// task logs belonging to a task run.
type TaskOptions struct {
	// ProjectID is the project ID of the task run.
	ProjectID string
	// TaskID is the task ID of the task run.
	TaskID string
	// Execution is the execution number of the task run.
	Execution int
}
