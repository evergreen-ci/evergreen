package client

import (
	"net/http"
	"sync"
	"time"

	"github.com/evergreen-ci/utility"
	"google.golang.org/grpc"
)

// baseCommunicator provides common methods for Communicator functionality but
// does not implement the entire interface.
type baseCommunicator struct {
	serverURL       string
	retry           utility.RetryOptions
	httpClient      *http.Client
	reqHeaders      map[string]string
	cedarHTTPClient *http.Client
	cedarGRPCClient *grpc.ClientConn
	loggerInfo      LoggerMetadata

	lastMessageSent time.Time
	mutex           sync.RWMutex
}

func newBaseCommunicator(serverURL string, reqHeaders map[string]string) baseCommunicator {
	return baseCommunicator{
		retry: utility.RetryOptions{
			MaxAttempts: defaultMaxAttempts,
			MinDelay:    defaultTimeoutStart,
			MaxDelay:    defaultTimeoutMax,
		},
		serverURL:  serverURL,
		reqHeaders: reqHeaders,
	}
}

// Close cleans up the resources being used by the communicator.
func (c *baseCommunicator) Close() {
	if c.httpClient != nil {
		utility.PutHTTPClient(c.httpClient)
	}
}

// SetTimeoutStart sets the initial timeout for a request.
func (c *baseCommunicator) SetTimeoutStart(timeoutStart time.Duration) {
	c.retry.MinDelay = timeoutStart
}

// SetTimeoutMax sets the maximum timeout for a request.
func (c *baseCommunicator) SetTimeoutMax(timeoutMax time.Duration) {
	c.retry.MaxDelay = timeoutMax
}

// SetMaxAttempts sets the number of attempts a request will be made.
func (c *baseCommunicator) SetMaxAttempts(attempts int) {
	c.retry.MaxAttempts = attempts
}

func (c *baseCommunicator) UpdateLastMessageTime() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.lastMessageSent = time.Now()
}

func (c *baseCommunicator) LastMessageAt() time.Time {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.lastMessageSent
}

func (c *baseCommunicator) GetLoggerMetadata() LoggerMetadata {
	return c.loggerInfo
}

func (c *baseCommunicator) resetClient() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.httpClient != nil {
		utility.PutHTTPClient(c.httpClient)

		c.httpClient = utility.GetDefaultHTTPRetryableClient()
		c.httpClient.Timeout = heartbeatTimeout
	}
	if c.cedarHTTPClient != nil {
		utility.PutHTTPClient(c.cedarHTTPClient)

		// We need to create a new HTTP client since cedar gRPC requests may
		// often exceed one minute or use a stream.
		c.cedarHTTPClient = utility.GetDefaultHTTPRetryableClient()
		c.cedarHTTPClient.Timeout = 0
	}
}
