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

// managementClient provides a wrapper for communicating with an amboy rest
// service for queue management. It implements the management.Manager
// interface and allows you to remotely interact with running jobs on a system.
// with running jobs on a system.
type managementClient struct {
	client *http.Client
	url    string
}

// NewManagementClient constructs a management.Manager instance,
// with its own HTTP client. All calls
func NewManagementClient(url string) management.Manager {
	return NewManagementClientFromExisting(&http.Client{}, url)
}

// NewManagementClientFromExisting constructs a new managementClient instance
// from an existing HTTP Client.
func NewManagementClientFromExisting(client *http.Client, url string) management.Manager {
	return &managementClient{
		client: client,
		url:    url,
	}
}

func (c *managementClient) doRequest(ctx context.Context, path string, out interface{}) error {
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
// StatusFilter values are defined as constants in the management package.
func (c *managementClient) JobStatus(ctx context.Context, filter management.StatusFilter) (*management.JobStatusReport, error) {
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
func (c *managementClient) RecentTiming(ctx context.Context, dur time.Duration, filter management.RuntimeFilter) (*management.JobRuntimeReport, error) {
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
func (c *managementClient) JobIDsByState(ctx context.Context, jobType string, filter management.StatusFilter) (*management.JobReportIDs, error) {
	out := &management.JobReportIDs{}

	if err := c.doRequest(ctx, fmt.Sprintf("/status/%s/%s", string(filter), jobType), out); err != nil {
		return nil, errors.Wrap(err, "problem with request")
	}

	return out, nil
}

// RecentErrors returns an error report for jobs that have completed in the
// window that have had errors. Use the filter to de-duplicate errors.
// ErrorFilter values are defined as constants in the management package.
func (c *managementClient) RecentErrors(ctx context.Context, dur time.Duration, filter management.ErrorFilter) (*management.JobErrorsReport, error) {
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
func (c *managementClient) RecentJobErrors(ctx context.Context, jobType string, dur time.Duration, filter management.ErrorFilter) (*management.JobErrorsReport, error) {
	out := &management.JobErrorsReport{}

	path := fmt.Sprintf("/errors/%s/%s/%d", string(filter), jobType, int64(dur.Seconds()))
	if err := c.doRequest(ctx, path, out); err != nil {
		return nil, errors.Wrap(err, "problem with request")
	}

	return out, nil
}

// CompleteJob marks the job with the given name complete.
func (c *managementClient) CompleteJob(ctx context.Context, name string) error {
	path := fmt.Sprintf("/jobs/mark_complete/%s", name)
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
			msg = errors.Wrap(err, "problem reading response body").Error()
		} else {
			msg = string(data)
		}
		return errors.Errorf("status code '%s' returned with message: '%s'", resp.Status, msg)
	}

	return nil
}

// CompleteJobsByType marks all jobs of the given type complete.
func (c *managementClient) CompleteJobsByType(ctx context.Context, f management.StatusFilter, jobType string) error {
	path := fmt.Sprintf("/jobs/mark_complete_by_type/%s/%s", jobType, f)
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
			msg = errors.Wrap(err, "problem reading response body").Error()
		} else {
			msg = string(data)
		}
		return errors.Errorf("status code '%s' returned with message: '%s'", resp.Status, msg)
	}

	return nil
}

// CompleteJobs marks all jobs of the given type complete.
func (c *managementClient) CompleteJobs(ctx context.Context, f management.StatusFilter) error {
	path := fmt.Sprintf("/jobs/mark_many_complete/%s", f)
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
			msg = errors.Wrap(err, "problem reading response body").Error()
		} else {
			msg = string(data)
		}
		return errors.Errorf("status code '%s' returned with message: '%s'", resp.Status, msg)
	}

	return nil
}
