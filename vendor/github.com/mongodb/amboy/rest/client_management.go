package rest

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy/management"
	"github.com/pkg/errors"
)

// ManagementClient provides a wrapper for communicating with an amboy rest
// service for queue management. It implements the management.Management
// interface and allows you to remotely interact with running jobs on a system.
// with running jobs on a system.
type ManagementClient struct {
	client *http.Client
	url    string
}

// NewManagementClient constructs a ManagementClient instance constructing a
// new HTTP client.
func NewManagementClient(url string) *ManagementClient {
	return NewManagementClientFromExisting(&http.Client{}, url)
}

// NewManagementClientFromExisting constructs a new ManagementClient instance
// from an existing HTTP Client.
func NewManagementClientFromExisting(client *http.Client, url string) *ManagementClient {
	return &ManagementClient{
		client: client,
		url:    url,
	}
}

func (c *ManagementClient) doRequest(ctx context.Context, path string, out interface{}) error {
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

// JobStatus a count, by job type, for all jobs that match the Counter filter.
// CounterFilter values are defined as constants in the management package.
func (c *ManagementClient) JobStatus(ctx context.Context, filter management.CounterFilter) (*management.JobStatusReport, error) {
	out := &management.JobStatusReport{}

	if err := c.doRequest(ctx, "/status/"+string(filter), out); err != nil {
		return nil, errors.WithStack(err)
	}

	return out, nil
}

// RecentTiming returns timing data latency or duration of job run time for
// in the window defined by the duration value. You must specify a timing
// filter (e.g. Latency or Duration) with a constant defined in the management
// package.
func (c *ManagementClient) RecentTiming(ctx context.Context, dur time.Duration, filter management.RuntimeFilter) (*management.JobRuntimeReport, error) {
	out := &management.JobRuntimeReport{}

	path := fmt.Sprintf("/timing/%s/%d", string(filter), int64(dur.Seconds()))

	if err := c.doRequest(ctx, path, out); err != nil {
		return nil, errors.Wrap(err, "problem with request")
	}

	return out, nil
}

// JobIDsByState returns a list of job IDs for each job type, for all jobs
// matching the filter. Filter value are defined as constants in the management
// package.
func (c *ManagementClient) JobIDsByState(ctx context.Context, jobType string, filter management.CounterFilter) (*management.JobReportIDs, error) {
	out := &management.JobReportIDs{}

	if err := c.doRequest(ctx, fmt.Sprintf("/status/%s/%s", string(filter), jobType), out); err != nil {
		return nil, errors.Wrap(err, "problem with request")
	}

	return out, nil
}

// RecentErrors returns an error report for jobs that have completed in the
// window that have had errors. Use the filter to de-duplicate errors.
// ErrorFilter values are defined as constants in the management package.
func (c *ManagementClient) RecentErrors(ctx context.Context, dur time.Duration, filter management.ErrorFilter) (*management.JobErrorsReport, error) {
	out := &management.JobErrorsReport{}

	path := fmt.Sprintf("/errors/%s/%d", string(filter), int64(dur.Seconds()))
	if err := c.doRequest(ctx, path, out); err != nil {
		return nil, errors.Wrap(err, "problem with request")
	}

	return out, nil
}

// RecentJobErrors returns an error report for jobs of a specific type that
// have encountered errors that have completed within the specified window. The
// ErrorFilter values are defined as constants in the management package.
func (c *ManagementClient) RecentJobErrors(ctx context.Context, jobType string, dur time.Duration, filter management.ErrorFilter) (*management.JobErrorsReport, error) {
	out := &management.JobErrorsReport{}

	path := fmt.Sprintf("/errors/%s/%s/%d", string(filter), jobType, int64(dur.Seconds()))
	if err := c.doRequest(ctx, path, out); err != nil {
		return nil, errors.Wrap(err, "problem with request")
	}

	return out, nil
}

// MarkCompleteByType marks all jobs of the given type complete.
func (c *ManagementClient) MarkCompleteByType(ctx context.Context, jobType string) error {
	path := fmt.Sprintf("/jobs/mark_complete_by_type/%s", jobType)
	req, err := http.NewRequest(http.MethodPost, c.url+path, nil)
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
		var msg string
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			msg = fmt.Sprintf("problem reading response body: '%s'", err.Error())
		} else {
			msg = string(data)
		}
		return errors.Errorf("status code '%s' returned with message: '%s'", resp.Status, msg)
	}

	return nil
}
