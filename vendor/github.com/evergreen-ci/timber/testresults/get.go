package testresults

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/evergreen-ci/timber"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// Valid sort by keys.
const (
	SortByStart      = "start"
	SortByDuration   = "duration"
	SortByTestName   = "test_name"
	SortByStatus     = "status"
	SortByBaseStatus = "base_status"
)

// GetOptions specify the required and optional information to create the test
// results HTTP GET request to Cedar.
type GetOptions struct {
	Cedar timber.GetOptions

	// Request information. See Cedar's REST documentation for more
	// information:
	// `https://github.com/evergreen-ci/cedar/wiki/Rest-V1-Usage`.
	TaskID       string
	FailedSample bool
	Stats        bool

	// Query parameters.
	Execution    *int
	DisplayTask  bool
	TestName     string
	Statuses     []string
	GroupID      string
	SortBy       string
	SortOrderDSC bool
	BaseTaskID   string
	Limit        int
	Page         int
}

// Validate ensures TestResultsGetOptions is configured correctly.
func (opts GetOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(opts.Cedar.Validate())
	catcher.NewWhen(opts.TaskID == "", "must provide a task id")
	catcher.NewWhen(opts.FailedSample && opts.Stats, "cannot request the failed sample and stats, must be one or the other")

	return catcher.Resolve()
}

func (opts GetOptions) parse() string {
	urlString := fmt.Sprintf("%s/rest/v1/test_results/task_id/%s", opts.Cedar.BaseURL, url.PathEscape(opts.TaskID))
	if opts.FailedSample {
		urlString += "/failed_sample"
	}
	if opts.Stats {
		urlString += "/stats"
	}

	var params []string
	if opts.Execution != nil {
		params = append(params, fmt.Sprintf("execution=%d", *opts.Execution))
	}
	if opts.DisplayTask {
		params = append(params, "display_task=true")
	}
	if opts.TestName != "" {
		params = append(params, fmt.Sprintf("test_name=%s", url.QueryEscape(opts.TestName)))
	}
	for _, status := range opts.Statuses {
		params = append(params, fmt.Sprintf("status=%s", url.QueryEscape(status)))
	}
	if opts.GroupID != "" {
		params = append(params, fmt.Sprintf("group_id=%s", url.QueryEscape(opts.GroupID)))
	}
	if opts.SortBy != "" {
		params = append(params, fmt.Sprintf("sort_by=%s", url.QueryEscape(opts.SortBy)))
	}
	if opts.SortOrderDSC {
		params = append(params, "sort_order_dsc=true")
	}
	if opts.BaseTaskID != "" {
		params = append(params, fmt.Sprintf("base_task_id=%s", url.QueryEscape(opts.BaseTaskID)))
	}
	if opts.Limit > 0 {
		params = append(params, fmt.Sprintf("limit=%d", opts.Limit))
	}
	if opts.Page > 0 {
		params = append(params, fmt.Sprintf("page=%d", opts.Page))
	}
	if len(params) > 0 {
		urlString += "?" + strings.Join(params, "&")
	}

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

// GetWithPaginatedReadCloser returns a paginated read closer for the test
// results requested via HTTP to a Cedar service.
func GetWithPaginatedReadCloser(ctx context.Context, opts GetOptions) (io.ReadCloser, error) {
	resp, err := get(ctx, opts)
	if err != nil {
		return nil, err
	}

	return timber.NewPaginatedReadCloser(ctx, resp, opts.Cedar), nil
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
