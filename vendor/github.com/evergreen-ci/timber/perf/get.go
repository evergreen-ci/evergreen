package testresults

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/evergreen-ci/timber"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// GetOptions specify the required and optional information to create the test
// results HTTP GET request to Cedar.
type GetOptions struct {
	Cedar timber.GetOptions

	// Request information. See Cedar's REST documentation for more
	// information:
	// `https://github.com/evergreen-ci/cedar/wiki/Rest-V1-Usage`.
	TaskID string
}

// Validate ensures TestResultsGetOptions is configured correctly.
func (opts GetOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(opts.Cedar.Validate())
	catcher.NewWhen(opts.TaskID == "", "must provide a task id")

	return catcher.Resolve()
}

func (opts GetOptions) parse() string {
	urlString := fmt.Sprintf("%s/rest/v1/test_results/task_id/%s/count", opts.Cedar.BaseURL, url.PathEscape(opts.TaskID))
	return urlString
}

// Get returns the test results requested via HTTP to a Cedar service.
func Get(ctx context.Context, opts GetOptions) ([]byte, error) {
	resp, err := get(ctx, opts)
	if err != nil {
		return nil, err
	}

	catcher := grip.NewBasicCatcher()
	data, err := io.ReadAll(resp.Body)
	catcher.Wrap(err, "reading response body")
	catcher.Wrap(resp.Body.Close(), "closing response body")

	return data, catcher.Resolve()
}

func get(ctx context.Context, opts GetOptions) (*http.Response, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := opts.Cedar.DoReq(ctx, opts.parse())
	if err != nil {
		return nil, errors.Wrap(err, "requesting test results from cedar")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to fetch test results with resp '%s'", resp.Status)
	}

	return resp, nil
}
