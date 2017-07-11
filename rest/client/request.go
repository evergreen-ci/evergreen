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

func (c *communicatorImpl) get(ctx context.Context, path string, taskData TaskData, version string) (*http.Response, error) {
	response, err := c.request(ctx, "GET", path, taskData.Secret, version, nil)
	return response, errors.Wrap(err, "Error performing HTTP GET request")
}

// this function is commented out because it is not yet used
// func (c *communicatorImpl) delete(ctx context.Context, path string, taskSecret, version string) (*http.Response, error) {
// 	response, err := c.request(ctx, "DELETE", path, taskSecret, version, nil)
// 	return response, errors.Wrap(err, "Error performing HTTP DELETE request")
// }

// this function is commented out because it is not yet used
// func (c *communicatorImpl) put(ctx context.Context, path, taskSecret, version string, data *interface{}) (*http.Response, error) {
// 	response, err := c.request(ctx, "PUT", path, taskSecret, version, data)
// 	return response, errors.Wrap(err, "Error performing HTTP PUT request")
// }

func (c *communicatorImpl) post(ctx context.Context, path string, taskData TaskData, version string, data *interface{}) (*http.Response, error) {
	response, err := c.request(ctx, "POST", path, taskData.Secret, version, data)
	return response, errors.Wrap(err, "Error performing HTTP POST request")
}

func (c *communicatorImpl) retryPost(ctx context.Context, path string, taskData TaskData, version string, data interface{}) (*http.Response, error) {
	if !taskData.OverrideValidation && taskData.Secret == "" {
		err := errors.New("no task secret provided")
		grip.Error(err)
		return nil, err
	}

	var dur time.Duration
	timer := time.NewTimer(0)
	defer timer.Stop()
	backoff := c.getBackoff()
	for i := 0; i < c.maxAttempts; i++ {
		select {
		case <-ctx.Done():
			return nil, errors.New("request canceled")
		case <-timer.C:
			resp, err := c.post(ctx, path, taskData, version, &data)
			if resp == nil {
				grip.Error("HTTP Post response is nil")
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
	return nil, errors.Errorf("Failed to post JSON after %d attempts", c.maxAttempts)
}

func (c *communicatorImpl) retryGet(ctx context.Context, path string, taskData TaskData, version string) (resp *http.Response, err error) {
	if !taskData.OverrideValidation && taskData.Secret == "" {
		err := errors.New("no task secret provided")
		grip.Error(err)
		return nil, err
	}

	timer := time.NewTimer(0)
	defer timer.Stop()
	backoff := c.getBackoff()
	for i := 1; i <= c.maxAttempts; i++ {
		select {
		case <-ctx.Done():
			return nil, errors.New("request canceled")
		case <-timer.C:
			resp, err := c.get(ctx, path, taskData, version)
			if err != nil {
				grip.Error(err)
			} else if resp == nil {
				// return immediately if response is nil
				err = errors.New("empty response from API server")
				grip.Error(err)
				return nil, err
			} else {
				return resp, nil
			}
			if i < c.maxAttempts {
				dur := backoff.Duration()
				timer.Reset(dur)
			}
		}
	}
	return nil, errors.Errorf("Failed to get after %d attempts", c.maxAttempts)
}

func (c *communicatorImpl) newRequest(method, path, taskSecret, version string, data *interface{}) (*http.Request, error) {
	url := c.getPath(path, version)
	r, err := http.NewRequest(method, url, nil)
	if data != nil {
		var out []byte
		out, err = json.Marshal(*data)
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

func (c *communicatorImpl) request(ctx context.Context, method, path, taskSecret, version string, data *interface{}) (*http.Response, error) {
	r, err := c.newRequest(method, path, taskSecret, version, data)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating request")
	}

	if ctx.Err() != nil {
		return nil, errors.New("request cancled")
	}
	response, err := ctxhttp.Do(ctx, c.httpClient, r)
	if ctx.Err() != nil {
		return nil, errors.New("request cancled")
	}
	if err != nil {
		return nil, errors.Wrap(err, "Error performing http request")
	}
	return response, nil
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
