package testresults

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/evergreen-ci/timber"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// TestResultsGetOptions specify the required and optional information to
// create the test results HTTP GET request to cedar.
type TestResultsGetOptions struct {
	CedarOpts timber.GetOptions

	// Request information. See cedar's REST documentation for more
	// information:
	// `https://github.com/evergreen-ci/cedar/wiki/Rest-V1-Usage`.
	TaskID          string
	DisplayTaskID   string
	TestName        string
	Execution       int
	LatestExecution bool
	FailedSample    bool
}

// Validate ensures TestResultsGetOptions is configured correctly.
func (opts *TestResultsGetOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(opts.CedarOpts.Validate())
	catcher.AddWhen(opts.TaskID == "" && opts.DisplayTaskID == "", errors.New("must provide a task id or a display task id"))
	catcher.AddWhen(opts.TaskID != "" && opts.DisplayTaskID != "", errors.New("cannot provide both a task id and a display task id"))
	catcher.AddWhen(opts.TestName != "" && opts.TaskID == "", errors.New("must provide a task id when a test name is specified"))
	catcher.AddWhen(opts.LatestExecution && opts.Execution > 0, errors.New("cannot provide an execution when requesting latest"))
	catcher.AddWhen(opts.FailedSample && opts.TestName != "", errors.New("cannot request the failed sample when requesting a single test result"))

	return catcher.Resolve()
}

// GetTestResults returns with the test results requested via HTTP to a cedar
// service.
func GetTestResults(ctx context.Context, opts TestResultsGetOptions) ([]byte, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	var urlString string
	if opts.DisplayTaskID != "" && !opts.FailedSample {
		urlString = fmt.Sprintf("%s/rest/v1/test_results/display_task_id/%s", opts.CedarOpts.BaseURL, url.PathEscape(opts.DisplayTaskID))
	} else if opts.TestName == "" {
		urlString = fmt.Sprintf("%s/rest/v1/test_results/task_id/%s", opts.CedarOpts.BaseURL, url.PathEscape(opts.TaskID))
	} else {
		urlString = fmt.Sprintf("%s/rest/v1/test_results/test_name/%s/%s", opts.CedarOpts.BaseURL, url.PathEscape(opts.TaskID), url.PathEscape(opts.TestName))
	}
	if opts.FailedSample {
		urlString += "/failed_sample"
	}

	var params string
	if !opts.LatestExecution {
		params += fmt.Sprintf("execution=%d", opts.Execution)
	}
	if opts.FailedSample && opts.DisplayTaskID != "" {
		params += "display_task=true"
	}
	if len(params) > 0 {
		urlString += "?" + params
	}

	catcher := grip.NewBasicCatcher()
	resp, err := opts.CedarOpts.DoReq(ctx, urlString)
	if err != nil {
		return nil, errors.Wrap(err, "requesting test results from cedar")
	}
	if resp.StatusCode != http.StatusOK {
		catcher.Add(resp.Body.Close())
		catcher.Add(errors.Errorf("failed to fetch test results with resp '%s'", resp.Status))
		return nil, catcher.Resolve()
	}

	data, err := ioutil.ReadAll(resp.Body)
	catcher.Add(err)
	catcher.Add(resp.Body.Close())

	return data, catcher.Resolve()
}
