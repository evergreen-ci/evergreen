package testutil

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
)

func InitializeTaskOutput(t *testing.T) *task.TaskOutput {
	return &task.TaskOutput{
		TaskLogs: task.TaskLogOutput{
			Version: 1,
			BucketConfig: evergreen.BucketConfig{
				Name: t.TempDir(),
				Type: evergreen.BucketTypeLocal,
			},
		},
		TestLogs: task.TestLogOutput{
			Version: 1,
			BucketConfig: evergreen.BucketConfig{
				Name: t.TempDir(),
				Type: evergreen.BucketTypeLocal,
			},
		},
		TestResults: task.TestResultOutput{
			Version: task.TestResultServiceEvergreen,
			BucketConfig: evergreen.BucketConfig{
				Name: t.TempDir(),
				Type: evergreen.BucketTypeLocal,
			},
		},
	}

}
