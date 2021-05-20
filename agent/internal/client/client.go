package client

import (
	"context"
	"io/ioutil"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/juniper/gopb"
	"github.com/evergreen-ci/pail"
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

// communicatorImpl implements Communicator and makes requests to API endpoints
// for the CLI.
type communicatorImpl struct {
	serverURL       string
	maxAttempts     int
	timeoutStart    time.Duration
	timeoutMax      time.Duration
	httpClient      *http.Client
	cedarHTTPClient *http.Client
	cedarGRPCClient *grpc.ClientConn
	loggerInfo      LoggerMetadata

	// these fields have setters
	hostID     string
	hostSecret string

	lastMessageSent time.Time
	mutex           sync.RWMutex
}

// NewCommunicator returns a Communicator capable of making HTTP REST requests
// against the API server. To change the default retry behavior, use the
// SetTimeoutStart, SetTimeoutMax, and SetMaxAttempts methods.
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
	if c.cedarHTTPClient != nil {
		utility.PutHTTPClient(c.cedarHTTPClient)
	}

	c.httpClient = utility.GetDefaultHTTPRetryableClient()
	c.httpClient.Timeout = heartbeatTimeout

	// We need to create a new HTTP client since cedar gRPC requests may
	// often exceed one minute or use a stream.
	c.cedarHTTPClient = utility.GetDefaultHTTPRetryableClient()
	c.cedarHTTPClient.Timeout = 0
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

func (c *communicatorImpl) GetLoggerProducer(ctx context.Context, td TaskData, config *LoggerConfig) (LoggerProducer, error) {
	if config == nil {
		config = &LoggerConfig{
			Agent:  []LogOpts{{Sender: model.EvergreenLogSender}},
			System: []LogOpts{{Sender: model.EvergreenLogSender}},
			Task:   []LogOpts{{Sender: model.EvergreenLogSender}},
		}
	}
	underlying := []send.Sender{}

	exec, senders, err := c.makeSender(ctx, td, config.Agent, apimodels.AgentLogPrefix, evergreen.LogTypeAgent)
	if err != nil {
		return nil, errors.Wrap(err, "making agent logger")
	}
	underlying = append(underlying, senders...)
	task, senders, err := c.makeSender(ctx, td, config.Task, apimodels.TaskLogPrefix, evergreen.LogTypeTask)
	if err != nil {
		return nil, errors.Wrap(err, "making task logger")
	}
	underlying = append(underlying, senders...)
	system, senders, err := c.makeSender(ctx, td, config.System, apimodels.SystemLogPrefix, evergreen.LogTypeSystem)
	if err != nil {
		return nil, errors.Wrap(err, "making system logger")
	}
	underlying = append(underlying, senders...)

	return &logHarness{
		execution:                 logging.MakeGrip(exec),
		task:                      logging.MakeGrip(task),
		system:                    logging.MakeGrip(system),
		underlyingBufferedSenders: underlying,
	}, nil
}

func (c *communicatorImpl) makeSender(ctx context.Context, td TaskData, opts []LogOpts, prefix string, logType string) (send.Sender, []send.Sender, error) {
	levelInfo := send.LevelInfo{Default: level.Info, Threshold: level.Debug}
	senders := []send.Sender{grip.GetSender()}
	underlyingBufferedSenders := []send.Sender{}

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
				return nil, nil, errors.Wrap(err, "creating file logger")
			}

			underlyingBufferedSenders = append(underlyingBufferedSenders, sender)
			sender = send.NewBufferedSender(sender, bufferDuration, bufferSize)
		case model.SplunkLogSender:
			info := send.SplunkConnectionInfo{
				ServerURL: opt.SplunkServerURL,
				Token:     opt.SplunkToken,
			}
			sender, err = send.NewSplunkLogger(prefix, info, levelInfo)
			if err != nil {
				return nil, nil, errors.Wrap(err, "creating splunk logger")
			}
			underlyingBufferedSenders = append(underlyingBufferedSenders, sender)
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
				return nil, nil, errors.Wrap(err, "creating logkeeper logger")
			}
			underlyingBufferedSenders = append(underlyingBufferedSenders, sender)
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
				return nil, nil, errors.Wrap(err, "setting up buildlogger sender")
			}

			if err = c.createCedarGRPCConn(ctx); err != nil {
				return nil, nil, errors.Wrap(err, "setting up cedar grpc connection")
			}

			timberOpts := &buildlogger.LoggerOptions{
				Project:       tk.Project,
				Version:       tk.Version,
				Variant:       tk.BuildVariant,
				TaskName:      tk.DisplayName,
				TaskID:        tk.Id,
				Execution:     int32(tk.Execution),
				Tags:          append(tk.Tags, logType, utility.RandomString()),
				Mainline:      !evergreen.IsPatchRequester(tk.Requester),
				Storage:       buildlogger.LogStorageS3,
				MaxBufferSize: opt.BufferSize,
				FlushInterval: opt.BufferDuration,
				ClientConn:    c.cedarGRPCClient,
			}
			sender, err = buildlogger.NewLoggerWithContext(ctx, opt.BuilderID, levelInfo, timberOpts)
			if err != nil {
				return nil, nil, errors.Wrap(err, "creating buildlogger logger")
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

	return send.NewConfiguredMultiSender(senders...), underlyingBufferedSenders, nil
}

func (c *communicatorImpl) createCedarGRPCConn(ctx context.Context) error {
	if c.cedarGRPCClient == nil {
		cc, err := c.GetCedarConfig(ctx)
		if err != nil {
			return errors.Wrap(err, "getting cedar config")
		}

		// TODO (EVG-14557): Remove TLS dial option fallback once cedar
		// gRPC is on API auth.
		catcher := grip.NewBasicCatcher()
		dialOpts := timber.DialCedarOptions{
			BaseAddress: cc.BaseURL,
			RPCPort:     cc.RPCPort,
			Username:    cc.Username,
			APIKey:      cc.APIKey,
			Retries:     10,
		}
		if runtime.GOOS == "windows" {
			cas, err := c.getAWSCACerts(ctx)
			if err != nil {
				return errors.Wrap(err, "getting AWS root CA certs for cedar gRPC client conn on windows")
			}
			dialOpts.CACerts = [][]byte{cas}
		}
		c.cedarGRPCClient, err = timber.DialCedar(ctx, c.cedarHTTPClient, dialOpts)
		if err != nil {
			catcher.Wrap(err, "creating cedar grpc client connection with API auth.")
		} else {
			healthClient := gopb.NewHealthClient(c.cedarGRPCClient)
			_, err = healthClient.Check(ctx, &gopb.HealthCheckRequest{})
			if err == nil {
				return nil
			}
			catcher.Wrap(err, "checking cedar grpc health with API auth")
		}

		// Try again, this time with TLS auth.
		dialOpts.TLSAuth = true
		c.cedarGRPCClient, err = timber.DialCedar(ctx, c.cedarHTTPClient, dialOpts)
		if err == nil {
			return nil
		}
		catcher.Wrap(err, "creating cedar grpc client connection with TLS auth")

		return catcher.Resolve()
	}

	return nil
}

// getAWSCACerts fetches AWS's root CA certificates stored in s3. This is a
// workaround for the fact that Go cannot access window's system certifacte
// pool (which would have these certificates).
// TODO: If and when the windows system cert issue is fixed, we can get rid of
// this workaround. See https://github.com/golang/go/issues/16736.
func (c *communicatorImpl) getAWSCACerts(ctx context.Context) ([]byte, error) {
	setupData, err := c.GetAgentSetupData(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting setup data")
	}

	// We are hardcoding this magic object in s3 because these certificates
	// are not set to expire for another 20 years. Also, we are hopeful
	// that this windows system cert issue will go away in future versions
	// of Go.
	bucket, err := pail.NewS3Bucket(pail.S3Options{
		Name:        "boxes.10gen.com",
		Prefix:      "build/amazontrust",
		Region:      endpoints.UsEast1RegionID,
		Credentials: pail.CreateAWSCredentials(setupData.S3Key, setupData.S3Secret, ""),
		MaxRetries:  10,
	})
	if err != nil {
		return nil, errors.Wrap(err, "creating pail bucket")
	}

	r, err := bucket.Get(ctx, "AmazonRootCA_all.pem")
	if err != nil {
		return nil, errors.Wrap(err, "getting AWS root CA certificates")
	}

	catcher := grip.NewBasicCatcher()
	cas, err := ioutil.ReadAll(r)
	catcher.Wrap(err, "reading AWS root CA certificates")
	catcher.Wrap(r.Close(), "closing the ReadCloser")

	return cas, catcher.Resolve()
}
