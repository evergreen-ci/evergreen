package client

import (
	"net/http"
	"time"
)

const (
	defaultMaxAttempts  = 10
	defaultTimeoutStart = time.Second * 2
	defaultTimeoutMax   = time.Minute * 10
	heartbeatTimeout    = time.Minute * 1

	v1 = "/api/2"
	// this variable is commented out because it is not yet used
	// v2 = "/rest/v2"
)

// evergreenREST implements Communicator and makes requests to API endpoints for the agent.
type evergreenREST struct {
	serverURL    string
	maxAttempts  int
	timeoutStart time.Duration
	timeoutMax   time.Duration
	httpClient   *http.Client

	// these fields have setters
	hostID     string
	hostSecret string
	apiUser    string
	apiKey     string
}

// NewEvergreenREST returns a Communicator capable of making HTTP REST requests against
// the API server. To change the default retry behavior, use the SetTimeoutStart, SetTimeoutMax,
// and SetMaxAttempts methods.
func NewEvergreenREST(serverURL string) Communicator {
	evergreen := &evergreenREST{
		maxAttempts:  defaultMaxAttempts,
		timeoutStart: defaultTimeoutStart,
		timeoutMax:   defaultTimeoutMax,
		serverURL:    serverURL,
		httpClient:   &http.Client{},
	}
	return evergreen
}

// SetTimeoutStart sets the initial timeout for a request.
func (c *evergreenREST) SetTimeoutStart(timeoutStart time.Duration) {
	c.timeoutStart = timeoutStart
}

// SetTimeoutMax sets the maximum timeout for a request.
func (c *evergreenREST) SetTimeoutMax(timeoutMax time.Duration) {
	c.timeoutMax = timeoutMax
}

// SetMaxAttempts sets the number of attempts a request will be made.
func (c *evergreenREST) SetMaxAttempts(attempts int) {
	c.maxAttempts = attempts
}

// SetHostID sets the host ID.
func (c *evergreenREST) SetHostID(hostID string) {
	c.hostID = hostID
}

// SetHostSecret sets the host secret.
func (c *evergreenREST) SetHostSecret(hostSecret string) {
	c.hostSecret = hostSecret
}

// SetAPIUser sets the API user.
func (c *evergreenREST) SetAPIUser(apiUser string) {
	c.apiUser = apiUser
}

// SetAPIKey sets the API key.
func (c *evergreenREST) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}
