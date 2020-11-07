package apimodels

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/testresults"
	"github.com/pkg/errors"
)

// GetCedarTestResultsOptions represents the arguments passed into the
// GetCedarTestResults function.
type GetCedarTestResultsOptions struct {
	BaseURL   string
	TaskID    string
	TestName  string
	Execution int
}

// GetCedarTestResults makes request to cedar for a a task's test results.
func GetCedarTestResults(ctx context.Context, opts GetCedarTestResultsOptions) ([]byte, error) {
	getOpts := timber.GetOptions{
		BaseURL:   fmt.Sprintf("https://%s", opts.BaseURL),
		TaskID:    opts.TaskID,
		TestName:  opts.TestName,
		Execution: opts.Execution,
	}
	data, err := testresults.GetTestResults(ctx, getOpts)
	return data, errors.Wrapf(err, "failed to get test results for '%s' from cedar, using evergreen test results", opts.TaskID)
}
