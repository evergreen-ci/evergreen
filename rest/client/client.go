package client

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

const (
	defaultMaxAttempts  = 10
	defaultTimeoutStart = time.Second * 2
	defaultTimeoutMax   = time.Minute * 10
	defaultTimeout      = time.Minute * 1
)

// communicatorImpl implements Communicator and makes requests to API endpoints
// for the CLI.
type communicatorImpl struct {
	serverURL    string
	maxAttempts  int
	timeoutStart time.Duration
	timeoutMax   time.Duration
	httpClient   *http.Client

	// these fields have setters
	apiUser string
	apiKey  string
	jwt     string

	hostID     string
	hostSecret string
}

// NewCommunicator returns a Communicator capable of making HTTP REST requests
// against the API server. To change the default retry behavior, use the
// SetTimeoutStart, SetTimeoutMax, and SetMaxAttempts methods.
func NewCommunicator(serverURL string) (Communicator, error) {
	if serverURL == "" {
		return nil, errors.New("API server URL cannot be empty")
	}
	c := &communicatorImpl{
		maxAttempts:  defaultMaxAttempts,
		timeoutStart: defaultTimeoutStart,
		timeoutMax:   defaultTimeoutMax,
		serverURL:    serverURL,
	}
	c.resetClient()

	return c, nil
}

func (c *communicatorImpl) resetClient() {
	if c.httpClient != nil {
		utility.PutHTTPClient(c.httpClient)
	}

	c.httpClient = utility.GetDefaultHTTPRetryableClient()
	c.httpClient.Timeout = defaultTimeout
}

func (c *communicatorImpl) Close() {
	utility.PutHTTPClient(c.httpClient)
}

// SetTimeoutStart sets the initial timeout for a request.
func (c *communicatorImpl) SetTimeoutStart(timeoutStart time.Duration) {
	c.timeoutStart = timeoutStart
}

// SetTimeoutMax sets the maximum timeout for a request.
func (c *communicatorImpl) SetTimeoutMax(timeoutMax time.Duration) {
	c.timeoutMax = timeoutMax
}

// SetMaxAttempts sets the number of attempts a request will be made.
func (c *communicatorImpl) SetMaxAttempts(attempts int) {
	c.maxAttempts = attempts
}

// SetAPIUser sets the API user.
func (c *communicatorImpl) SetAPIUser(apiUser string) {
	c.apiUser = apiUser
}

// SetAPIKey sets the API key.
func (c *communicatorImpl) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}

// SetJWT sets the JWT for authentication.
func (c *communicatorImpl) SetJWT(jwt string) {
	c.jwt = jwt
}

// SetAPIServerHost sets the API server host.
func (c *communicatorImpl) SetAPIServerHost(serverURL string) {
	c.serverURL = serverURL
}

// SetHostID sets the host ID for authentication using host credentials instead
// of API keys.
func (c *communicatorImpl) SetHostID(hostID string) {
	c.hostID = hostID
}

// SetHostSecret sets the host secret for authentication using host credentials
// instead of API keys.
func (c *communicatorImpl) SetHostSecret(hostSecret string) {
	c.hostSecret = hostSecret
}
