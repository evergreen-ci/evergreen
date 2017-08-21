package client

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
)

const (
	defaultMaxAttempts  = 10
	defaultTimeoutStart = time.Second * 2
	defaultTimeoutMax   = time.Minute * 10
	heartbeatTimeout    = time.Minute * 1

	v1 = "/api/2"
)

// communicatorImpl implements Communicator and makes requests to API endpoints for the agent.
type communicatorImpl struct {
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

// TaskData contains the taskData.ID and taskData.Secret. It must be set for some client methods.
type TaskData struct {
	ID                 string
	Secret             string
	OverrideValidation bool
}

// NewCommunicator returns a Communicator capable of making HTTP REST requests against
// the API server. To change the default retry behavior, use the SetTimeoutStart, SetTimeoutMax,
// and SetMaxAttempts methods.
func NewCommunicator(serverURL string) Communicator {
	evergreen := &communicatorImpl{
		maxAttempts:  defaultMaxAttempts,
		timeoutStart: defaultTimeoutStart,
		timeoutMax:   defaultTimeoutMax,
		serverURL:    serverURL,
		httpClient:   &http.Client{},
	}
	return evergreen
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

// SetHostID sets the host ID.
func (c *communicatorImpl) SetHostID(hostID string) {
	c.hostID = hostID
}

// SetHostSecret sets the host secret.
func (c *communicatorImpl) SetHostSecret(hostSecret string) {
	c.hostSecret = hostSecret
}

// GetHostID returns the host ID.
func (c *communicatorImpl) GetHostID() string {
	return c.hostID
}

// GetHostSecret returns the host secret.
func (c *communicatorImpl) GetHostSecret() string {
	return c.hostSecret
}

// SetAPIUser sets the API user.
func (c *communicatorImpl) SetAPIUser(apiUser string) {
	c.apiUser = apiUser
}

// SetAPIKey sets the API key.
func (c *communicatorImpl) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}

// GetLogProducer
func (c *communicatorImpl) GetLoggerProducer(taskData TaskData) LoggerProducer {
	const (
		bufferTime  = 15 * time.Second
		bufferCount = 100
	)

	local := grip.GetSender()

	exec := newLogSender(c, apimodels.AgentLogPrefix, taskData)
	grip.CatchWarning(exec.SetFormatter(send.MakeDefaultFormatter()))
	exec = send.NewBufferedSender(exec, bufferTime, bufferCount)
	exec = send.NewConfiguredMultiSender(local, exec)

	task := newLogSender(c, apimodels.TaskLogPrefix, taskData)
	grip.CatchWarning(task.SetFormatter(send.MakeDefaultFormatter()))
	task = send.NewBufferedSender(task, bufferTime, bufferCount)
	task = send.NewConfiguredMultiSender(local, task)

	system := newLogSender(c, apimodels.SystemLogPrefix, taskData)
	grip.CatchWarning(system.SetFormatter(send.MakeDefaultFormatter()))
	system = send.NewBufferedSender(system, bufferTime, bufferCount)
	system = send.NewConfiguredMultiSender(local, system)

	taskWriter := newLogSender(c, apimodels.TaskLogPrefix, taskData)
	taskWriter = send.NewBufferedSender(taskWriter, bufferTime, bufferCount)
	taskWriter = send.NewConfiguredMultiSender(local, taskWriter)

	systemWriter := newLogSender(c, apimodels.SystemLogPrefix, taskData)
	systemWriter = send.NewBufferedSender(systemWriter, bufferTime, bufferCount)
	systemWriter = send.NewConfiguredMultiSender(local, systemWriter)

	return &logHarness{
		execution:        &logging.Grip{Sender: exec},
		task:             &logging.Grip{Sender: task},
		system:           &logging.Grip{Sender: system},
		taskWriterBase:   taskWriter,
		systemWriterBase: systemWriter,
	}
}
