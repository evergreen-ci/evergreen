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
	"github.com/jpillora/backoff"
	"github.com/pkg/errors"
)

// RequestInfo holds metadata about a request
type requestInfo struct {
	method string
	path   string
}

// AuthError is a special error when the CLI receives 401 Unauthorized to
// suggest logging in again as a possible solution to the error.
var AuthError = errors.New("401 Unauthorized: User credentials are likely expired, try logging in again via the Evergreen web UI.")

func (c *communicatorImpl) newRequest(method, path string, data interface{}) (*http.Request, error) {
	url := c.getPath(path)
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

	if c.apiUser != "" {
		r.Header.Add(evergreen.APIUserHeader, c.apiUser)
	}
	if c.apiUser != "" {
		r.Header.Add(evergreen.APIKeyHeader, c.apiKey)
	}
	r.Header.Add(evergreen.ContentTypeHeader, evergreen.ContentTypeValue)

	return r, nil
}

func (c *communicatorImpl) createRequest(info requestInfo, data interface{}) (*http.Request, error) {
	if info.method == http.MethodPost && data == nil {
		return nil, errors.New("Attempting to post a nil body")
	}
	if err := info.validateRequestInfo(); err != nil {
		return nil, errors.WithStack(err)
	}

	r, err := c.newRequest(info.method, info.path, data)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating request")
	}

	return r, nil
}

func (c *communicatorImpl) request(ctx context.Context, info requestInfo, data interface{}) (*http.Response, error) {
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

func (c *communicatorImpl) doRequest(ctx context.Context, r *http.Request) (*http.Response, error) {
	var (
		response *http.Response
		err      error
	)

	r = r.WithContext(ctx)

	func() {
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

func (c *communicatorImpl) retryRequest(ctx context.Context, info requestInfo, data interface{}) (*http.Response, error) {
	var err error

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

	resp, err := utility.RetryRequest(ctx, r, utility.RetryOptions{
		MaxAttempts: c.maxAttempts,
		MinDelay:    c.timeoutStart,
		MaxDelay:    c.timeoutMax,
	})
	if resp != nil && resp.StatusCode == http.StatusUnauthorized {
		return resp, AuthError
	} else if err != nil {
		return resp, err
	}
	return resp, nil
}

func (c *communicatorImpl) getBackoff() *backoff.Backoff {
	return &backoff.Backoff{
		Min:    c.timeoutStart,
		Max:    c.timeoutMax,
		Factor: 2,
		Jitter: true,
	}
}

func (c *communicatorImpl) getPath(path string) string {
	return fmt.Sprintf("%s%s/%s", c.serverURL, evergreen.APIRoutePrefixV2, strings.TrimPrefix(path, "/"))
}

func (r *requestInfo) validateRequestInfo() error {
	switch r.method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
	default:
		return errors.New("invalid HTTP method")
	}

	return nil
}
