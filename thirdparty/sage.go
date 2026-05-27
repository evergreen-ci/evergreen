package thirdparty

import (
	"net/http"

	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// SageClient is a client for communicating with the Sage API service.
type SageClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewSageClient creates a new Sage API client.
func NewSageClient(baseURL string) (*SageClient, error) {
	if baseURL == "" {
		return nil, errors.New("Sage base URL is not configured")
	}
	return &SageClient{
		httpClient: utility.GetHTTPClient(),
		baseURL:    baseURL,
	}, nil
}

// Close releases resources used by the client.
func (c *SageClient) Close() {
	utility.PutHTTPClient(c.httpClient)
}

// TODO: DEVPROD-33674 - Add GitHub OAuth credential methods here.
