package apimodels

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/perf"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// GetCedarPerfCountOptions represents the arguments for getting a count
// of perf results for a given task id from Cedar.
type GetCedarPerfCountOptions struct {
	BaseURL   string `json:"-"`
	TaskID    string `json:"-"`
	Execution int    `json:"_"`
}

func (opts GetCedarPerfCountOptions) convert() perf.GetOptions {
	return perf.GetOptions{
		Cedar: timber.GetOptions{
			BaseURL: fmt.Sprintf("https://%s", opts.BaseURL),
		},
		TaskID:    opts.TaskID,
		Execution: utility.ToIntPtr(opts.Execution),
		Count:     true,
	}
}

// CedarPerfCount holds one element, NumberOfResults, matching the json returned by
// Cedar's perf count rest route.
type CedarPerfCount struct {
	NumberOfResults int `json:"number_of_results"`
}

// CedarPerfResultsCount queries Cedar for the number of perf results attached to a task.
func CedarPerfResultsCount(ctx context.Context, opts GetCedarPerfCountOptions) (*CedarPerfCount, error) {
	data, err := perf.Get(ctx, opts.convert())
	if err != nil {
		return nil, errors.Wrap(err, "getting test results from Cedar")
	}

	testCount := &CedarPerfCount{}
	if err = json.Unmarshal(data, testCount); err != nil {
		return nil, errors.Wrap(err, "unmarshaling test results from Cedar")
	}

	return testCount, nil
}
