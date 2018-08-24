package rest

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy/reporting"
	"github.com/pkg/errors"
)

// ReportingClient provides a wrapper for communicating with an amboy
// rest service for queue reporting. It implements the
// reporting.Reporter interface and allows you to remotely interact
// with running jobs on a system.
type ReportingClient struct {
	client *http.Client
	url    string
}

// NewReportingClient constructs a ReportingClient instance
// constructing a new HTTP client.
func NewReportingClient(url string) *ReportingClient {
	return NewReportingClientFromExisting(&http.Client{}, url)
}

// NewReportingClientFromExisting constructs a new ReportingClient
// instance from an existing HTTP Client.
func NewReportingClientFromExisting(client *http.Client, url string) *ReportingClient {
	return &ReportingClient{
		client: client,
		url:    url,
	}
}

func (c *ReportingClient) doRequest(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequest(http.MethodGet, c.url+path, nil)
	if err != nil {
		return errors.Wrap(err, "problem building request")
	}

	req = req.WithContext(ctx)
	resp, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "error processing request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("found '%s' for request to '%s' on '%s'",
			http.StatusText(resp.StatusCode), path, c.url)
	}

	if err = gimlet.GetJSON(resp.Body, out); err != nil {
		return errors.Wrap(err, "problem reading response")
	}

	return nil
}

// JobStatus a count, by job type, for all jobs that match the Counter
// filter. CounterFilter values are defined as constants in the
// reporting package.
func (c *ReportingClient) JobStatus(ctx context.Context, filter reporting.CounterFilter) (*reporting.JobStatusReport, error) {
	out := &reporting.JobStatusReport{}

	if err := c.doRequest(ctx, "/status/"+string(filter), out); err != nil {
		return nil, errors.WithStack(err)
	}

	return out, nil
}

// RecentTiming returns timing data latency or duration of job run
// time for jobs in the window defined by the duration value. You must
// specify a timing filter (e.g. Latency or Duration) with a constant
// defined in the reporting package.
func (c *ReportingClient) RecentTiming(ctx context.Context, dur time.Duration, filter reporting.RuntimeFilter) (*reporting.JobRuntimeReport, error) {
	out := &reporting.JobRuntimeReport{}

	path := fmt.Sprintf("/timing/%s/%d", string(filter), int64(dur.Seconds()))

	if err := c.doRequest(ctx, path, out); err != nil {
		return nil, errors.Wrap(err, "problem with request")
	}

	return out, nil
}

// JobIDsByState returns a list of job IDs for each job type, for all
// jobs matching the filter. Filuter value are defined as constants in
// the reporting package.
func (c *ReportingClient) JobIDsByState(ctx context.Context, jobType string, filter reporting.CounterFilter) (*reporting.JobReportIDs, error) {
	out := &reporting.JobReportIDs{}

	if err := c.doRequest(ctx, fmt.Sprintf("/status/%s/%s", string(filter), jobType), out); err != nil {
		return nil, errors.Wrap(err, "problem with request")
	}

	return out, nil
}

// RecentErrors returns an error report for jobs that have
// completed in the given window that have had errors. Use the filter
// to de-duplicate errors. ErrorFilter values are defined as constants
// in the reporting package.
func (c *ReportingClient) RecentErrors(ctx context.Context, dur time.Duration, filter reporting.ErrorFilter) (*reporting.JobErrorsReport, error) {
	out := &reporting.JobErrorsReport{}

	path := fmt.Sprintf("/errors/%s/%d", string(filter), int64(dur.Seconds()))
	if err := c.doRequest(ctx, path, out); err != nil {
		return nil, errors.Wrap(err, "problem with request")
	}

	return out, nil
}

// RecentJobErrors returns an error report for jobs of a specific type
// that have encountered errors that have completed within the
// specified window. The ErrorFilter values are defined as constants
// in the reporting package.
func (c *ReportingClient) RecentJobErrors(ctx context.Context, jobType string, dur time.Duration, filter reporting.ErrorFilter) (*reporting.JobErrorsReport, error) {
	out := &reporting.JobErrorsReport{}

	path := fmt.Sprintf("/errors/%s/%s/%d", string(filter), jobType, int64(dur.Seconds()))
	if err := c.doRequest(ctx, path, out); err != nil {
		return nil, errors.Wrap(err, "problem with request")
	}

	return out, nil
}
