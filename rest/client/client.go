package client

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/buildlogger"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
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
	serverURL       string
	maxAttempts     int
	timeoutStart    time.Duration
	timeoutMax      time.Duration
	httpClient      *http.Client
	cedarGRPCClient *grpc.ClientConn
	loggerInfo      LoggerMetadata

	// these fields have setters
	hostID     string
	hostSecret string
	apiUser    string
	apiKey     string

	lastMessageSent time.Time
	mutex           sync.RWMutex
}

type LoggerMetadata struct {
	Agent  []LogkeeperMetadata
	System []LogkeeperMetadata
	Task   []LogkeeperMetadata
}

type LogkeeperMetadata struct {
	Build string
	Test  string
}

// TaskData contains the taskData.ID and taskData.Secret. It must be set for some client methods.
type TaskData struct {
	ID                 string
	Secret             string
	OverrideValidation bool
}

type LoggerConfig struct {
	System []LogOpts
	Agent  []LogOpts
	Task   []LogOpts
}

type LogOpts struct {
	Sender            string
	SplunkServerURL   string
	SplunkToken       string
	Filepath          string
	LogkeeperURL      string
	LogkeeperBuildNum int
	BuilderID         string
	BufferDuration    time.Duration
	BufferSize        int
}

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
		utility.PutHTTPClient(c.httpClient)
	}

	c.httpClient = utility.GetDefaultHTTPRetryableClient()
	c.httpClient.Timeout = heartbeatTimeout
}

func (c *communicatorImpl) Close() { utility.PutHTTPClient(c.httpClient) }

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

func (c *communicatorImpl) GetLoggerMetadata() LoggerMetadata {
	return c.loggerInfo
}

// GetLogProducer
func (c *communicatorImpl) GetLoggerProducer(ctx context.Context, td TaskData, config *LoggerConfig) (LoggerProducer, error) {
	if config == nil {
		config = &LoggerConfig{
			Agent:  []LogOpts{{Sender: model.EvergreenLogSender}},
			System: []LogOpts{{Sender: model.EvergreenLogSender}},
			Task:   []LogOpts{{Sender: model.EvergreenLogSender}},
		}
	}

	exec, err := c.makeSender(ctx, td, config.Agent, apimodels.AgentLogPrefix, evergreen.LogTypeAgent)
	if err != nil {
		return nil, errors.Wrap(err, "error making agent logger")
	}
	task, err := c.makeSender(ctx, td, config.Task, apimodels.TaskLogPrefix, evergreen.LogTypeTask)
	if err != nil {
		return nil, errors.Wrap(err, "error making task logger")
	}
	system, err := c.makeSender(ctx, td, config.System, apimodels.SystemLogPrefix, evergreen.LogTypeSystem)
	if err != nil {
		return nil, errors.Wrap(err, "error making system logger")
	}

	return &logHarness{
		execution: logging.MakeGrip(exec),
		task:      logging.MakeGrip(task),
		system:    logging.MakeGrip(system),
	}, nil
}

func (c *communicatorImpl) makeSender(ctx context.Context, td TaskData, opts []LogOpts, prefix string, logType string) (send.Sender, error) {
	levelInfo := send.LevelInfo{Default: level.Info, Threshold: level.Debug}
	senders := []send.Sender{grip.GetSender()}

	for _, opt := range opts {
		var sender send.Sender
		var err error
		bufferDuration := defaultLogBufferTime
		if opt.BufferDuration > 0 {
			bufferDuration = opt.BufferDuration
		}
		bufferSize := defaultLogBufferSize
		if opt.BufferSize > 0 {
			bufferSize = opt.BufferSize
		}
		// disallow sending system logs to S3 or logkeeper for security reasons
		if prefix == apimodels.SystemLogPrefix && (opt.Sender == model.FileLogSender || opt.Sender == model.LogkeeperLogSender) {
			opt.Sender = model.EvergreenLogSender
		}
		switch opt.Sender {
		case model.FileLogSender:
			sender, err = send.NewPlainFileLogger(prefix, opt.Filepath, levelInfo)
			if err != nil {
				return nil, errors.Wrap(err, "error creating file logger")
			}

			sender = send.NewBufferedSender(sender, bufferDuration, bufferSize)
		case model.SplunkLogSender:
			info := send.SplunkConnectionInfo{
				ServerURL: opt.SplunkServerURL,
				Token:     opt.SplunkToken,
			}
			sender, err = send.NewSplunkLogger(prefix, info, levelInfo)
			if err != nil {
				return nil, errors.Wrap(err, "error creating splunk logger")
			}
			sender = send.NewBufferedSender(newAnnotatedWrapper(td.ID, prefix, sender), bufferDuration, bufferSize)
		case model.LogkeeperLogSender:
			config := send.BuildloggerConfig{
				URL:        opt.LogkeeperURL,
				Number:     opt.LogkeeperBuildNum,
				Local:      grip.GetSender(),
				Test:       prefix,
				CreateTest: true,
			}
			sender, err = send.NewBuildlogger(opt.BuilderID, &config, levelInfo)
			if err != nil {
				return nil, errors.Wrap(err, "error creating logkeeper logger")
			}
			sender = send.NewBufferedSender(sender, bufferDuration, bufferSize)
			metadata := LogkeeperMetadata{
				Build: config.GetBuildID(),
				Test:  config.GetTestID(),
			}
			switch prefix {
			case apimodels.AgentLogPrefix:
				c.loggerInfo.Agent = append(c.loggerInfo.Agent, metadata)
			case apimodels.SystemLogPrefix:
				c.loggerInfo.System = append(c.loggerInfo.System, metadata)
			case apimodels.TaskLogPrefix:
				c.loggerInfo.Task = append(c.loggerInfo.Task, metadata)
			}
		case model.BuildloggerLogSender:
			tk, err := c.GetTask(ctx, td)
			if err != nil {
				return nil, errors.Wrap(err, "error setting up buildlogger sender")
			}

			if err = c.createCedarGRPCConn(ctx); err != nil {
				return nil, errors.Wrap(err, "error setting up cedar grpc connection")
			}

			timberOpts := &buildlogger.LoggerOptions{
				Project:       tk.Project,
				Version:       tk.Version,
				Variant:       tk.BuildVariant,
				TaskName:      tk.DisplayName,
				TaskID:        tk.Id,
				Execution:     int32(tk.Execution),
				Tags:          append(tk.Tags, utility.RandomString()),
				ProcessName:   logType,
				Mainline:      !evergreen.IsPatchRequester(tk.Requester),
				Storage:       buildlogger.LogStorageS3,
				MaxBufferSize: opt.BufferSize,
				FlushInterval: opt.BufferDuration,
				ClientConn:    c.cedarGRPCClient,
			}
			sender, err = buildlogger.NewLoggerWithContext(ctx, opt.BuilderID, levelInfo, timberOpts)
			if err != nil {
				return nil, errors.Wrap(err, "error creating buildlogger logger")
			}
		default:
			sender = newEvergreenLogSender(ctx, c, prefix, td, bufferSize, bufferDuration)
		}

		grip.Error(sender.SetFormatter(send.MakeDefaultFormatter()))
		if prefix == apimodels.TaskLogPrefix {
			sender = makeTimeoutLogSender(sender, c)
		}
		senders = append(senders, sender)
	}

	return send.NewConfiguredMultiSender(senders...), nil
}

func (c *communicatorImpl) createCedarGRPCConn(ctx context.Context) error {
	if c.cedarGRPCClient == nil {
		bi, err := c.GetBuildloggerInfo(ctx)
		if err != nil {
			return errors.Wrap(err, "error setting up buildlogger sender")
		}

		dialOpts := timber.DialCedarOptions{
			BaseAddress: bi.BaseURL,
			RPCPort:     bi.RPCPort,
			Username:    bi.Username,
			Password:    bi.Password,
			APIKey:      bi.APIKey,
			Retries:     10,
		}
		c.cedarGRPCClient, err = timber.DialCedar(ctx, c.httpClient, dialOpts)
		if err != nil {
			return errors.Wrap(err, "error creating cedar grpc client connection")
		}
	}
	return nil
}
