package taskoutput

import "github.com/evergreen-ci/evergreen"

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

// InitializeTaskOutput initializes the task output for a new task run.
func InitializeTaskOutput(env evergreen.Environment, opts TaskOptions) *TaskOutput {
	settings := evergreen.GetEnvironment().Settings()

	output := &TaskOutput{}
	if settings.LoggerConfig.DefaultLogger != "buildlogger" {
		output.TaskLogs.Version = 1
		output.TaskLogs.BucketName = settings.Buckets.LogBucket.Name
		output.TaskLogs.BucketType = settings.Buckets.LogBucket.Type
		output.TestLogs.Version = 1
		output.TestLogs.BucketName = settings.Buckets.LogBucket.Name
		output.TestLogs.BucketType = settings.Buckets.LogBucket.Type
	}

	return output
}
