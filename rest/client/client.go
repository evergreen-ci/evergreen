package client

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/mongodb/grip/level"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

const (
	defaultMaxAttempts  = 10
	defaultTimeoutStart = time.Second * 2
	defaultTimeoutMax   = time.Minute * 10
	heartbeatTimeout    = time.Minute * 1

	defaultLogBufferTime = 15 * time.Second
	defaultLogBufferSize = 1000
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

	lastMessageSent time.Time
	mutex           sync.RWMutex
}

// TaskData contains the taskData.ID and taskData.Secret. It must be set for some client methods.
type TaskData struct {
	ID                 string
	Secret             string
	OverrideValidation bool
}

type LoggerConfig struct {
	System LogOpts
	Agent  LogOpts
	Task   LogOpts
}

type LogOpts struct {
	Sender          LogSender
	SplunkServerURL string
	SplunkToken     string
	Filepath        string
	LogkeeperURL    string
	BufferDuration  time.Duration
	BufferSize      int
}

type LogSender int

const (
	EvergreenLogSender LogSender = iota
	FileLogSender
	LogkeeperLogSender
	SplunkLogSender
)

// NewCommunicator returns a Communicator capable of making HTTP REST requests against
// the API server. To change the default retry behavior, use the SetTimeoutStart, SetTimeoutMax,
// and SetMaxAttempts methods.
func NewCommunicator(serverURL string) Communicator {
	c := &communicatorImpl{
		maxAttempts:  defaultMaxAttempts,
		timeoutStart: defaultTimeoutStart,
		timeoutMax:   defaultTimeoutMax,
		serverURL:    serverURL,
	}

	c.resetClient()
	return c
}

func (c *communicatorImpl) resetClient() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.httpClient != nil {
		util.PutHTTPClient(c.httpClient)
	}

	c.httpClient = util.GetHTTPClient()
	c.httpClient.Timeout = heartbeatTimeout
}

func (c *communicatorImpl) Close() { util.PutHTTPClient(c.httpClient) }

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

func (c *communicatorImpl) UpdateLastMessageTime() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.lastMessageSent = time.Now()
}

func (c *communicatorImpl) LastMessageAt() time.Time {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.lastMessageSent
}

// GetLogProducer
func (c *communicatorImpl) GetLoggerProducer(ctx context.Context, taskData TaskData, config *LoggerConfig) LoggerProducer {
	local := grip.GetSender()

	if config == nil {
		config = &LoggerConfig{}
	}

	exec := c.makeSender(ctx, taskData, config.Agent, apimodels.AgentLogPrefix)
	grip.Warning(exec.SetFormatter(send.MakeDefaultFormatter()))
	exec = send.NewConfiguredMultiSender(local, exec)

	task := c.makeSender(ctx, taskData, config.Task, apimodels.TaskLogPrefix)
	grip.Warning(task.SetFormatter(send.MakeDefaultFormatter()))
	task = send.NewConfiguredMultiSender(local, task)

	system := c.makeSender(ctx, taskData, config.System, apimodels.SystemLogPrefix)
	grip.Warning(system.SetFormatter(send.MakeDefaultFormatter()))
	system = send.NewConfiguredMultiSender(local, system)

	return &logHarness{
		execution: logging.MakeGrip(exec),
		task:      logging.MakeGrip(task),
		system:    logging.MakeGrip(system),
	}
}

func (c *communicatorImpl) makeSender(ctx context.Context, taskData TaskData, opts LogOpts, prefix string) send.Sender {
	levelInfo := send.LevelInfo{Default: level.Info, Threshold: level.Debug}
	var sender send.Sender
	var err error
	bufferDuration := defaultLogBufferTime
	if opts.BufferDuration > 0 {
		bufferDuration = opts.BufferDuration
	}
	bufferSize := defaultLogBufferSize
	if opts.BufferSize > 0 {
		bufferSize = opts.BufferSize
	}
	switch opts.Sender {
	// TODO: placeholder until implemented
	case FileLogSender:
		sender, err = send.NewPlainFileLogger(prefix, opts.Filepath, levelInfo)
		if err != nil {
			grip.Critical(errors.Wrap(err, "error creating file logger"))
			return nil
		}
		err = sender.SetFormatter(send.MakePlainFormatter())
		if err != nil {
			grip.Critical(errors.Wrap(err, "error setting file logger format"))
			return nil
		}
		sender = send.NewBufferedSender(sender, bufferDuration, bufferSize)
	// TODO: placeholder until implemented
	case SplunkLogSender:
		info := send.SplunkConnectionInfo{
			ServerURL: opts.SplunkServerURL,
			Token:     opts.SplunkToken,
		}
		sender, err = send.NewSplunkLogger(prefix, info, levelInfo)
		if err != nil {
			grip.Critical(errors.Wrap(err, "error creating splunk logger"))
			return nil
		}
		sender = send.NewBufferedSender(sender, bufferDuration, bufferSize)
	// TODO: placeholder until implemented
	case LogkeeperLogSender:
		fallback, err := send.NewNativeLogger(prefix, levelInfo)
		if err != nil {
			grip.Critical(errors.Wrap(err, "error creating native fallback logger"))
			return nil
		}
		config := send.BuildloggerConfig{
			CreateTest: true,
			URL:        opts.LogkeeperURL,
			Local:      fallback,
		}
		sender, err = send.NewBuildlogger(prefix, &config, levelInfo)
		if err != nil {
			grip.Critical(errors.Wrap(err, "error creating logkeeper logger"))
			return nil
		}
		sender = send.NewBufferedSender(sender, bufferDuration, bufferSize)
	default:
		sender = newEvergreenLogSender(ctx, c, prefix, taskData, bufferSize, bufferDuration)
	}
	sender = makeTimeoutLogSender(sender, c)

	return sender
}
