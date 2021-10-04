package testresults

import (
	"testing"

	"github.com/evergreen-ci/timber"
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
				TaskID: "task",
			},
			hasErr: true,
		},
		{
			name: "MissingTaskID",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
			},
			hasErr: true,
		},
		{
			name: "TaskID",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				TaskID: "task",
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
				Cedar:  cedarOpts,
				TaskID: "task",
			},
			expectedURL: baseURL + "/task_id/task/count",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedURL, test.opts.parse())
		})
	}
}
