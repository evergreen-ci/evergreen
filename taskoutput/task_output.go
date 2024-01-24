package taskoutput

import "github.com/evergreen-ci/evergreen"

// TaskOutput is the versioned entry point for coordinating persistent storage
// of a task run's output data.
type TaskOutput struct {
	TaskLogs TaskLogOutput `bson:"task_logs,omitempty" json:"task_logs"`
	TestLogs TestLogOutput `bson:"test_logs,omitempty" json:"test_logs"`
}

// TaskOptions represents the task-level information required for accessing
// task logs belonging to a task run.
type TaskOptions struct {
	// ProjectID is the project ID of the task run.
	ProjectID string `bson:"-" json:"-"`
	// TaskID is the task ID of the task run.
	TaskID string `bson:"-" json:"-"`
	// Execution is the execution number of the task run.
	Execution int `bson:"-" json:"-"`
}

// InitializeTaskOutput initializes the task output for a new task run.
func InitializeTaskOutput(env evergreen.Environment, opts TaskOptions) *TaskOutput {
	settings := env.Settings()

	output := &TaskOutput{}
	if settings.LoggerConfig.DefaultLogger != "buildlogger" {
		output.TaskLogs.Version = 1
		output.TaskLogs.BucketConfig = settings.Buckets.LogBucket
		output.TestLogs.Version = 1
		output.TestLogs.BucketConfig = settings.Buckets.LogBucket
	}

	return output
}
