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
	taskData TaskData
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

func (c *communicatorImpl) get(ctx context.Context, path string, taskData TaskData, version apiVersion) (*http.Response, error) {
	info := requestInfo{
		method:   get,
		path:     path,
		version:  version,
		taskData: taskData,
	}
	response, err := c.request(ctx, info, nil)
	return response, errors.Wrap(err, "Error performing HTTP GET request")
}

// this function is commented out because it is not yet used
// func (c *communicatorImpl) delete(ctx context.Context, path string, taskSecret, version string) (*http.Response, error) {
//	response, err := c.request(ctx, "DELETE", path, taskSecret, version, nil)
//	return response, errors.Wrap(err, "Error performing HTTP DELETE request")
// }

// this function is commented out because it is not yet used
// func (c *communicatorImpl) put(ctx context.Context, path, taskSecret, version string, data *interface{}) (*http.Response, error) {
//	response, err := c.request(ctx, "PUT", path, taskSecret, version, data)
//	return response, errors.Wrap(err, "Error performing HTTP PUT request")
// }

func (c *communicatorImpl) post(ctx context.Context, path string, taskData TaskData, version apiVersion, data interface{}) (*http.Response, error) {
	info := requestInfo{
		method:   post,
		path:     path,
		version:  version,
		taskData: taskData,
	}
	response, err := c.request(ctx, info, data)
	return response, errors.Wrap(err, "Error performing HTTP POST request")
}

func (c *communicatorImpl) retryPost(ctx context.Context, path string, taskData TaskData, version apiVersion, data interface{}) (*http.Response, error) {
	info := requestInfo{
		method:   post,
		path:     path,
		taskData: taskData,
		version:  version,
	}
	return c.retryRequest(ctx, info, data)
}

func (c *communicatorImpl) retryGet(ctx context.Context, path string, taskData TaskData, version apiVersion) (*http.Response, error) {
	info := requestInfo{
		method:   get,
		path:     path,
		taskData: taskData,
		version:  version,
	}
	return c.retryRequest(ctx, info, nil)
}

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
	if info.method == post && data == nil {
		return nil, errors.New("Attempting to post a nil body")
	}
	if err := validateRequestInfo(info); err != nil {
		return nil, err
	}
	r, err := c.newRequest(string(info.method), info.path, info.taskData.Secret, string(info.version), data)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating request")
	}
	response, err := ctxhttp.Do(ctx, c.httpClient, r)
	if err != nil {
		return nil, errors.Wrap(err, "Error performing http request")
	}
	return response, nil
}

func (c *communicatorImpl) retryRequest(ctx context.Context, info requestInfo, data interface{}) (*http.Response, error) {
	if !info.taskData.OverrideValidation && info.taskData.Secret == "" {
		err := errors.New("no task secret provided")
		grip.Error(err)
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
			resp, err := c.request(ctx, info, &data)
			if resp == nil {
				grip.Error("HTTP response is nil")
				return nil, errors.New("HTTP response is nil")
			} else if err != nil {
				grip.Error(err)
			} else if resp.StatusCode == http.StatusConflict {
				grip.Error("HTTP conflict error")
			} else if resp.StatusCode == http.StatusOK {
				return resp, nil
			} else {
				grip.Errorf("unexpected status code: %d", resp.StatusCode)
			}
			dur = backoff.Duration()
			timer.Reset(dur)
		}

	}
	return nil, errors.Errorf("Failed to make request after %d attempts", c.maxAttempts)
}

func validateRequestInfo(info requestInfo) error {
	if info.method != get && info.method != post && info.method != put && info.method != delete && info.method != patch {
		return errors.New("invalid HTTP method")
	}

	if info.version != apiVersion1 && info.version != apiVersion2 {
		return errors.New("invalid API version")
	}

	return nil
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

func (c *communicatorImpl) getTaskPathSuffix(path, taskID string) string {
	return fmt.Sprintf("task/%s/%s", taskID, path)
}
