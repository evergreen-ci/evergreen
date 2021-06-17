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
	TaskID        string
	DisplayTaskID string
	TestName      string
	Execution     int
}

// Validate ensures TestResultsGetOptions is configured correctly.
func (opts *TestResultsGetOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(opts.CedarOpts.Validate())
	catcher.AddWhen(opts.TaskID == "" && opts.DisplayTaskID == "", errors.New("must provide a task id or a display task id"))
	catcher.AddWhen(opts.TaskID != "" && opts.DisplayTaskID != "", errors.New("cannot provide both a task id and a display task id"))
	catcher.AddWhen(opts.TestName != "" && opts.TaskID == "", errors.New("must provide a task id when a test name is specified"))

	return catcher.Resolve()
}

// GetTestResults returns with the test results requested via HTTP to a cedar
// service.
func GetTestResults(ctx context.Context, opts TestResultsGetOptions) ([]byte, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	var urlString string
	if opts.DisplayTaskID != "" {
		urlString = fmt.Sprintf("%s/rest/v1/test_results/display_task_id/%s", opts.CedarOpts.BaseURL, url.PathEscape(opts.DisplayTaskID))
	} else if opts.TestName == "" {
		urlString = fmt.Sprintf("%s/rest/v1/test_results/task_id/%s", opts.CedarOpts.BaseURL, url.PathEscape(opts.TaskID))
	} else {
		urlString = fmt.Sprintf("%s/rest/v1/test_results/test_name/%s/%s", opts.CedarOpts.BaseURL, url.PathEscape(opts.TaskID), url.PathEscape(opts.TestName))
	}
	urlString += fmt.Sprintf("?execution=%d", opts.Execution)

	catcher := grip.NewBasicCatcher()
	resp, err := opts.CedarOpts.DoReq(ctx, urlString)
	if err != nil {
		return nil, errors.Wrap(err, "problem requesting test results from cedar")
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
