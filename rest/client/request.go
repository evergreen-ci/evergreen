package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/jpillora/backoff"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

// RequestInfo holds metadata about a request
type requestInfo struct {
	method   method
	path     string
	version  apiVersion
	taskData *TaskData
}

// Version is an "enum" for the different API versions
type apiVersion string

const (
	apiVersion1 apiVersion = "/api/2"
	apiVersion2            = "/rest/v2"
)

// Method is an "enum" for the supported HTTP methods
type method string

const (
	get    method = "GET"
	post          = "POST"
	put           = "PUT"
	delete        = "DELETE"
	patch         = "PATCH"
)

func (c *communicatorImpl) newRequest(method, path, taskSecret, version string, data interface{}) (*http.Request, error) {
	url := c.getPath(path, version)
	r, err := http.NewRequest(method, url, nil)
	if data != nil {
		var out []byte
		out, err = json.Marshal(data)
		if err != nil {
			return nil, err
		}
		r.Body = ioutil.NopCloser(bytes.NewReader(out))
	}
	if err != nil {
		return nil, errors.New("Error building request")
	}
	if taskSecret != "" {
		r.Header.Add(evergreen.TaskSecretHeader, taskSecret)
	}
	if c.hostID != "" {
		r.Header.Add(evergreen.HostHeader, c.hostID)
	}
	if c.apiUser != "" {
		r.Header.Add(evergreen.APIUserHeader, c.apiUser)
	}
	if c.apiUser != "" {
		r.Header.Add(evergreen.APIKeyHeader, c.apiKey)
	}
	if c.hostSecret != "" {
		r.Header.Add(evergreen.HostSecretHeader, c.hostSecret)
	}
	r.Header.Add(evergreen.ContentTypeHeader, evergreen.ContentTypeValue)
	return r, nil
}

func (c *communicatorImpl) request(ctx context.Context, info requestInfo, data interface{}) (*http.Response, error) {
	r, err := c.createRequest(info, data)
	if err != nil {
		return nil, err
	}
	return c.doRequest(ctx, c.httpClient, r)
}

func (c *communicatorImpl) createRequest(info requestInfo, data interface{}) (*http.Request, error) {
	if info.method == post && data == nil {
		return nil, errors.New("Attempting to post a nil body")
	}
	if err := info.validateRequestInfo(); err != nil {
		return nil, err
	}
	r, err := c.newRequest(string(info.method), info.path, info.taskData.Secret, string(info.version), data)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating request")
	}
	return r, nil
}

func (c *communicatorImpl) doRequest(ctx context.Context, data interface{}, r *http.Request) (*http.Response, error) {
	response, err := ctxhttp.Do(ctx, c.httpClient, r)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, errors.New("received nil response")
	}
	return response, nil
}

func (c *communicatorImpl) retryRequest(ctx context.Context, info requestInfo, data interface{}) (*http.Response, error) {
	if info.taskData != nil && !info.taskData.OverrideValidation && info.taskData.Secret == "" {
		err := errors.New("no task secret provided")
		grip.Error(err)
		return nil, err
	}
	r, err := c.createRequest(info, data)
	if err != nil {
		return nil, err
	}

	var dur time.Duration
	timer := time.NewTimer(0)
	defer timer.Stop()
	backoff := c.getBackoff()
	for i := 1; i <= c.maxAttempts; i++ {
		select {
		case <-ctx.Done():
			return nil, errors.New("request canceled")
		case <-timer.C:
			resp, err := c.doRequest(ctx, &data, r)
			if err != nil {
				// for an error, don't return, just retry
				grip.Warningf("error response from api server: %v (attempt %d of %d)", err, i, c.maxAttempts)
			} else if resp.StatusCode == http.StatusOK {
				return resp, nil
			} else {
				grip.Warningf("unexpected status code: %d (attempt %d of %d)", resp.StatusCode, i, c.maxAttempts)
			}
			dur = backoff.Duration()
			timer.Reset(dur)
		}

	}
	return nil, errors.Errorf("Failed to make request after %d attempts", c.maxAttempts)
}

func (c *communicatorImpl) getBackoff() *backoff.Backoff {
	return &backoff.Backoff{
		Min:    c.timeoutStart,
		Max:    c.timeoutMax,
		Factor: 2,
		Jitter: true,
	}
}

func (c *communicatorImpl) getPath(path string, version string) string {
	return fmt.Sprintf("%s%s/%s", c.serverURL, version, path)
}

func (r *requestInfo) validateRequestInfo() error {
	if r.method != get && r.method != post && r.method != put && r.method != delete && r.method != patch {
		return errors.New("invalid HTTP method")
	}

	if r.version != apiVersion1 && r.version != apiVersion2 {
		return errors.New("invalid API version")
	}

	return nil
}

func (r *requestInfo) setTaskPathSuffix(path, taskID string) {
	r.path = fmt.Sprintf("task/%s/%s", taskID, path)
}
