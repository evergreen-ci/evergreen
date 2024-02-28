package testutil

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/taskoutput"
)

func InitializeTaskOutput(t *testing.T) *taskoutput.TaskOutput {
	return &taskoutput.TaskOutput{
		TaskLogs: taskoutput.TaskLogOutput{
			Version: 1,
			BucketConfig: evergreen.BucketConfig{
				Name: t.TempDir(),
				Type: evergreen.BucketTypeLocal,
			},
		},
		TestLogs: taskoutput.TestLogOutput{
			Version: 1,
			BucketConfig: evergreen.BucketConfig{
				Name: t.TempDir(),
				Type: evergreen.BucketTypeLocal,
			},
		},
	}

}
