package task

import (
	"github.com/evergreen-ci/evergreen"
)

// TaskOutput is the versioned entry point for coordinating persistent storage
// of a task run's output data.
type TaskOutput struct {
	TaskLogs    TaskLogOutput    `bson:"task_logs,omitempty" json:"task_logs"`
	TestLogs    TestLogOutput    `bson:"test_logs,omitempty" json:"test_logs"`
	TestResults TestResultOutput `bson:"test_results,omitempty" json:"test_results"`
}

// InitializeTaskOutput initializes the task output for a new task run.
func InitializeTaskOutput(env evergreen.Environment) *TaskOutput {
	settings := env.Settings()

	return &TaskOutput{
		TaskLogs: TaskLogOutput{
			Version:      TestResultServiceEvergreen,
			BucketConfig: settings.Buckets.LogBucket,
		},
		TestLogs: TestLogOutput{
			Version:      TestResultServiceEvergreen,
			BucketConfig: settings.Buckets.LogBucket,
		},
		TestResults: TestResultOutput{
			Version:      TestResultServiceEvergreen,
			BucketConfig: settings.Buckets.TestResultsBucket,
		},
	}
}
