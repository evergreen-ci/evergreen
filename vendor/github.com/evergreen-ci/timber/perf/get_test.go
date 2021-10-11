package testresults

import (
	"testing"

	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
)

func TestGetOptionsValidate(t *testing.T) {
	for _, test := range []struct {
		name   string
		opts   GetOptions
		hasErr bool
	}{
		{
			name: "InvalidCedarOpts",
			opts: GetOptions{
				TaskID:    "task",
				Execution: utility.ToIntPtr(0),
			},
			hasErr: true,
		},
		{
			name: "MissingTaskID",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				Execution: utility.ToIntPtr(0),
			},
			hasErr: true,
		},
		{
			name: "MissingExecution",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				TaskID: "task",
			},
			hasErr: true,
		},

		{
			name: "TaskID",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				TaskID:    "task",
				Execution: utility.ToIntPtr(0),
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

func TestParse(t *testing.T) {
	cedarOpts := timber.GetOptions{BaseURL: "https://url.com"}
	baseURL := cedarOpts.BaseURL + "/rest/v1/test_results"
	for _, test := range []struct {
		name        string
		opts        GetOptions
		expectedURL string
	}{
		{
			name: "TaskID",
			opts: GetOptions{
				Cedar:     cedarOpts,
				TaskID:    "task",
				Execution: utility.ToIntPtr(0),
			},
			expectedURL: baseURL + "/task_id/task/0/count",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedURL, test.opts.parse())
		})
	}
}
