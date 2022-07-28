package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// requestInfo holds metadata about a request
type requestInfo struct {
	method   string
	path     string
	version  apiVersion
	taskData *TaskData
}

// Version is an "enum" for the different API versions
type apiVersion string

const (
	apiVersion1 apiVersion = "/api/2"
	apiVersion2 apiVersion = evergreen.APIRoutePrefixV2
)

var HTTPConflictError = errors.New(evergreen.TaskConflict)

func (c *baseCommunicator) newRequest(method, path, taskID, taskSecret, version string, data interface{}) (*http.Request, error) {
	url := c.getPath(path, version)
	r, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, errors.New("Error building request")
	}
	if data != nil {
		if rc, ok := data.(io.ReadCloser); ok {
			r.Body = rc
		} else {
			var out []byte
			out, err = json.Marshal(data)
			if err != nil {
				return nil, err
			}
			r.Header.Add(evergreen.ContentLengthHeader, strconv.Itoa(len(out)))
			r.Body = ioutil.NopCloser(bytes.NewReader(out))
		}
	}

	if taskID != "" {
		r.Header.Add(evergreen.TaskHeader, taskID)
	}
	if taskSecret != "" {
		r.Header.Add(evergreen.TaskSecretHeader, taskSecret)
	}
	for name, val := range c.reqHeaders {
		r.Header.Add(name, val)
	}
	r.Header.Add(evergreen.ContentTypeHeader, evergreen.ContentTypeValue)

	return r, nil
}

func (c *baseCommunicator) createRequest(info requestInfo, data interface{}) (*http.Request, error) {
	if info.method == http.MethodPost && data == nil {
		return nil, errors.New("Attempting to post a nil body")
	}
	if err := info.validateRequestInfo(); err != nil {
		return nil, errors.WithStack(err)
	}

	var taskID, secret string
	if info.taskData != nil {
		taskID = info.taskData.ID
		secret = info.taskData.Secret
	}
	r, err := c.newRequest(info.method, info.path, taskID, secret, string(info.version), data)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating request")
	}

	return r, nil
}

func (c *baseCommunicator) request(ctx context.Context, info requestInfo, data interface{}) (*http.Response, error) {
	r, err := c.createRequest(info, data)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	resp, err := c.doRequest(ctx, r)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return resp, nil
}

func (c *baseCommunicator) doRequest(ctx context.Context, r *http.Request) (*http.Response, error) {
	var (
		response *http.Response
		err      error
	)

	r = r.WithContext(ctx)

	func() {
		c.mutex.RLock()
		defer c.mutex.RUnlock()
		response, err = c.httpClient.Do(r)
	}()

	if err != nil {
		c.resetClient()
		return nil, errors.WithStack(err)
	}

	if response == nil {
		return nil, errors.New("received nil response")
	}

	return response, nil
}

func (c *baseCommunicator) retryRequest(ctx context.Context, info requestInfo, data interface{}) (*http.Response, error) {
	var err error
	if info.taskData != nil && !info.taskData.OverrideValidation && info.taskData.Secret == "" {
		err = errors.New("no task secret provided")
		grip.Error(err)
		return nil, err
	}

	var out []byte
	if data != nil {
		out, err = json.Marshal(data)
		if err != nil {
			return nil, err
		}
	}

	r, err := c.createRequest(info, ioutil.NopCloser(bytes.NewReader(out)))
	if err != nil {
		return nil, err
	}

	r.Header.Add(evergreen.ContentLengthHeader, strconv.Itoa(len(out)))

	resp, err := utility.RetryRequest(ctx, r, c.retry)
	if err != nil && resp != nil && resp.StatusCode == 400 {
		var taskId, start, end string
		if info.taskData != nil {
			taskId = info.taskData.ID
		}
		if len(out) >= 100 {
			start = string(out[0:100])
			end = string(out[len(out)-100:])
		}
		grip.Debug(message.Fields{
			"message":          "error sending request",
			"method":           info.method,
			"path":             info.path,
			"task":             taskId,
			"len_request":      len(out),
			"start_of_request": start,
			"end_of_request":   end,
		})
	}
	return resp, err
}

func (c *baseCommunicator) getPath(path string, version string) string {
	return fmt.Sprintf("%s%s/%s", c.serverURL, version, strings.TrimPrefix(path, "/"))
}

func (r *requestInfo) validateRequestInfo() error {
	switch r.method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
	default:
		return errors.New("invalid HTTP method")
	}

	if r.version != apiVersion1 && r.version != apiVersion2 {
		return errors.New("invalid API version")
	}

	return nil
}

func (r *requestInfo) setTaskPathSuffix(path string) {
	r.path = fmt.Sprintf("task/%s/%s", r.taskData.ID, strings.TrimPrefix(path, "/"))
}
