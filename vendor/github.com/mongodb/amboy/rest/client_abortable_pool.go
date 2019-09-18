package rest

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// ManagementClient provides a go wrapper to the management
// service.
type ManagementClient struct {
	client *http.Client
	url    string
}

// NewManagementClient constructs a new ManagementClient instance
// that constructs a new http.Client.
func NewManagementClient(url string) *ManagementClient {
	return NewManagementClientFromExisting(&http.Client{}, url)
}

// NewManagementClientFromExisting builds a ManagementClient instance
// from an existing http.Client.
func NewManagementClientFromExisting(client *http.Client, url string) *ManagementClient {
	return &ManagementClient{
		client: client,
		url:    url,
	}
}

// ListJobs returns a full list of all running jobs managed by the
// pool that the service reflects.
func (c *ManagementClient) ListJobs(ctx context.Context) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, c.url+"/v1/jobs/list", nil)
	if err != nil {
		return nil, errors.Wrap(err, "problem building request")
	}

	req = req.WithContext(ctx)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error processing request")
	}
	defer resp.Body.Close()
	out := []string{}
	if err = gimlet.GetJSON(resp.Body, &out); err != nil {
		return nil, errors.Wrap(err, "problem reading response")
	}

	return out, nil
}

// AbortAllJobs issues the request to terminate all currently running
// jobs managed by the pool that backs the request.
func (c *ManagementClient) AbortAllJobs(ctx context.Context) error {
	req, err := http.NewRequest(http.MethodDelete, c.url+"/v1/jobs/abort", nil)
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
		return errors.New("failed to abort jobs")
	}

	return nil
}

// IsRunning checks if a job with a specified id is currently running
// in the remote queue. Check the error value to identify if false
// response is due to a communication problem with the service or is
// legitimate.
func (c *ManagementClient) IsRunning(ctx context.Context, job string) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, c.url+"/v1/jobs/"+job, nil)
	if err != nil {
		return false, errors.Wrap(err, "problem building request")
	}

	req = req.WithContext(ctx)
	resp, err := c.client.Do(req)
	if err != nil {
		return false, errors.Wrap(err, "error processing request")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	return true, nil
}

// AbortJob sends the abort signal for a running job to the management
// service, return any errors from the service. A nil response
// indicates that the job has been successfully terminated.
func (c *ManagementClient) AbortJob(ctx context.Context, job string) error {
	req, err := http.NewRequest(http.MethodDelete, c.url+"/v1/jobs/"+job, nil)
	if err != nil {
		return errors.Wrap(err, "problem building request")
	}

	req = req.WithContext(ctx)
	resp, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "error processing request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		rerr := &gimlet.ErrorResponse{}
		if err := gimlet.GetJSON(resp.Body, rerr); err != nil {
			return errors.Wrapf(err, "problem reading error response with %s",
				http.StatusText(resp.StatusCode))

		}
		return errors.Wrap(rerr, "remove server returned error")
	}

	return nil
}
