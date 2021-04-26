package rest

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

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
func (c *managementClient) JobStatus(ctx context.Context, filter management.StatusFilter) ([]management.JobTypeCount, error) {
	out := []management.JobTypeCount{}

	if err := c.doRequest(ctx, "/status/"+string(filter), &out); err != nil {
		return nil, errors.WithStack(err)
	}

	return out, nil
}

// JobIDsByState returns a list of job IDs for each job type, for all jobs
// matching the filter. Filter value are defined as constants in the management
// package.
func (c *managementClient) JobIDsByState(ctx context.Context, jobType string, filter management.StatusFilter) ([]management.GroupedID, error) {
	out := []management.GroupedID{}

	if err := c.doRequest(ctx, fmt.Sprintf("/id/status/%s/type/%s", string(filter), jobType), &out); err != nil {
		return nil, errors.Wrap(err, "problem with request")
	}

	return out, nil
}

// CompleteJob marks the job with the given name complete.
func (c *managementClient) CompleteJob(ctx context.Context, name string) error {
	path := fmt.Sprintf("/jobs/mark_complete/id/%s", name)
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

// CompleteJobs marks all jobs with the given status complete. If a matching job is
// retrying, it will no longer retry.
func (c *managementClient) CompleteJobs(ctx context.Context, f management.StatusFilter) error {
	path := fmt.Sprintf("/jobs/mark_complete/status/%s", f)
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

// CompleteJobsByType marks all jobs with the given status and job type
// complete. If a matching job is retrying, it will not longer retry.
func (c *managementClient) CompleteJobsByType(ctx context.Context, f management.StatusFilter, jobType string) error {
	path := fmt.Sprintf("/jobs/mark_complete/status/%s/type/%s", f, jobType)
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

// CompleteJobsByPattern marks all jobs with the given status and job ID pattern
// complete. If a matching job is retrying, it will no longer retry.
func (c *managementClient) CompleteJobsByPattern(ctx context.Context, f management.StatusFilter, pattern string) error {
	path := fmt.Sprintf("/jobs/mark_complete/status/%s/pattern/%s", f, pattern)
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
