package testresults

import (
	"testing"

	"github.com/evergreen-ci/timber"
	"github.com/stretchr/testify/assert"
)

func TestTestResultsGetOptionsValidate(t *testing.T) {
	for _, test := range []struct {
		name   string
		opts   TestResultsGetOptions
		hasErr bool
	}{
		{
			name: "InvalidCedarOpts",
			opts: TestResultsGetOptions{
				TaskID: "task",
			},
			hasErr: true,
		},
		{
			name: "MissingTaskIDAndDisplayTaskID",
			opts: TestResultsGetOptions{
				CedarOpts: timber.GetOptions{
					BaseURL: "https://url.com",
				},
			},
			hasErr: true,
		},
		{
			name: "MissingTaskIDWithTestName",
			opts: TestResultsGetOptions{
				CedarOpts: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				DisplayTaskID: "display",
				TestName:      "test",
			},
			hasErr: true,
		},
		{
			name: "TaskID",
			opts: TestResultsGetOptions{
				CedarOpts: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				TaskID: "task",
			},
		},
		{
			name: "DisplayTaskID",
			opts: TestResultsGetOptions{
				CedarOpts: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				DisplayTaskID: "display",
			},
		},
		{
			name: "TestName",
			opts: TestResultsGetOptions{
				CedarOpts: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				TaskID:   "task",
				TestName: "test",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.opts.Validate()
			if test.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
