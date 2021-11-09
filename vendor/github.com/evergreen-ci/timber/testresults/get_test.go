package testresults

import (
	"fmt"
	"net/url"
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
			name: "FailedSampleAndStats",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				TaskID:       "task",
				FailedSample: true,
				Stats:        true,
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
		{
			name: "TaskIDAndFailedSample",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				TaskID:       "task",
				FailedSample: true,
			},
		},
		{
			name: "TaskIDAndStats",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				TaskID:       "task",
				FailedSample: true,
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
			expectedURL: baseURL + "/task_id/task",
		},
		{
			name: "TaskIDWithParams",
			opts: GetOptions{
				Cedar:        cedarOpts,
				TaskID:       "task?",
				Execution:    utility.ToIntPtr(1),
				DisplayTask:  true,
				TestName:     "test?",
				Statuses:     []string{"fail&", "silentfail"},
				GroupID:      "group/1?",
				SortBy:       "sort/by",
				SortOrderDSC: true,
				BaseTaskID:   "base_task?",
				Limit:        100,
				Page:         5,
			},
			expectedURL: fmt.Sprintf(
				"%s/task_id/%s?execution=1&display_task=true&test_name=%s&status=%s&status=silentfail&group_id=%s&sort_by=%s&sort_order_dsc=true&base_task_id=%s&limit=100&page=5",
				baseURL,
				url.PathEscape("task?"),
				url.QueryEscape("test?"),
				url.QueryEscape("fail&"),
				url.QueryEscape("group/1?"),
				url.QueryEscape("sort/by"),
				url.QueryEscape("base_task?"),
			),
		},
		{
			name: "FailedSample",
			opts: GetOptions{
				Cedar:        cedarOpts,
				TaskID:       "task",
				FailedSample: true,
			},
			expectedURL: baseURL + "/task_id/task/failed_sample",
		},
		{
			name: "Stats",
			opts: GetOptions{
				Cedar:  cedarOpts,
				TaskID: "task",
				Stats:  true,
			},
			expectedURL: baseURL + "/task_id/task/stats",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedURL, test.opts.parse())
		})
	}
}
