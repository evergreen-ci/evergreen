package timber

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// GetOptions specify the required and optional information to create an HTTP
// GET request to cedar.
type GetOptions struct {
	// The cedar service's base HTTP URL for the request.
	BaseURL string
	// The user cookie for cedar authorization. Optional.
	Cookie *http.Cookie
	// User API key and name for request header.
	UserKey  string
	UserName string
}

// Validate ensures GetOptions is configured correctly.
func (opts GetOptions) Validate() error {
	if opts.BaseURL == "" {
		return errors.New("must provide a base URL")
	}

	return nil
}

// DoReq makes an HTTP request to the cedar service.
func (opts GetOptions) DoReq(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error creating http request for cedar")
	}
	if opts.Cookie != nil {
		req.AddCookie(opts.Cookie)
	}
	if opts.UserKey != "" && opts.UserName != "" {
		req.Header.Set("Evergreen-Api-Key", opts.UserKey)
		req.Header.Set("Evergreen-Api-User", opts.UserName)
	}
	req = req.WithContext(ctx)

	c := utility.GetHTTPClient()
	defer utility.PutHTTPClient(c)

	return c.Do(req)
}
