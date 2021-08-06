package client

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/evergreen-ci/juniper/gopb"
	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// baseCommunicator provides common methods for Communicator functionality but
// does not implement the entire interface.
type baseCommunicator struct {
	serverURL       string
	retry           utility.RetryOptions
	httpClient      *http.Client
	reqHeaders      map[string]string
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
	}

	c.httpClient = utility.GetDefaultHTTPRetryableClient()
	c.httpClient.Timeout = heartbeatTimeout
}

func (c *baseCommunicator) createCedarGRPCConn(ctx context.Context, comm Communicator) error {
	if c.cedarGRPCClient == nil {
		cc, err := comm.GetCedarConfig(ctx)
		if err != nil {
			return errors.Wrap(err, "getting cedar config")
		}

		if cc.BaseURL == "" {
			// No cedar base URL probably means we are running
			// evergreen locally or in some testing mode.
			return nil
		}

		dialOpts := timber.DialCedarOptions{
			BaseAddress: cc.BaseURL,
			RPCPort:     cc.RPCPort,
			Username:    cc.Username,
			APIKey:      cc.APIKey,
			Retries:     10,
		}
		c.cedarGRPCClient, err = timber.DialCedar(ctx, c.httpClient, dialOpts)
		if err != nil {
			return errors.Wrap(err, "creating cedar grpc client connection")
		}
	}

	// We should always check the health of the conn as a sanity check,
	// this way we can fail the agent early and avoid task system failures.
	healthClient := gopb.NewHealthClient(c.cedarGRPCClient)
	_, err := healthClient.Check(ctx, &gopb.HealthCheckRequest{})
	return errors.Wrap(err, "checking cedar grpc health")
}
