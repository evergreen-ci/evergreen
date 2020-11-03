package buildlogger

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/evergreen-ci/aviation"
	"github.com/evergreen-ci/aviation/services"
	"github.com/evergreen-ci/juniper/gopb"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

const (
	defaultMaxBufferSize int = 1e7
	defaultFlushInterval     = time.Minute
)

// LogFormat describes the format of the log.
type LogFormat int32

// Valid LogFormat values.
const (
	LogFormatUnknown LogFormat = 0
	LogFormatText    LogFormat = 1
	LogFormatJSON    LogFormat = 2
	LogFormatBSON    LogFormat = 3
)

func (f LogFormat) validate() error {
	switch f {
	case LogFormatUnknown, LogFormatText, LogFormatJSON, LogFormatBSON:
		return nil
	default:
		return errors.New("invalid log format specified")
	}
}

// LogStorage describes the blob storage location type of the log.
type LogStorage int32

// Valid LogStorage values.
const (
	LogStorageS3     LogStorage = 0
	LogStorageGridFS LogStorage = 1
	LogStorageLocal  LogStorage = 2
)

func (s LogStorage) validate() error {
	switch s {
	case LogStorageS3, LogStorageGridFS, LogStorageLocal:
		return nil
	default:
		return errors.New("invalid log storage specified")
	}
}

type buildlogger struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	opts       *LoggerOptions
	conn       *grpc.ClientConn
	client     gopb.BuildloggerClient
	buffer     []*gopb.LogLine
	bufferSize int
	lastFlush  time.Time
	timer      *time.Timer
	closed     bool
	*send.Base
}

// LoggerOptions support the use and creation of a Buildlogger log.
type LoggerOptions struct {
	// Unique information to identify the log.
	Project     string            `bson:"project" json:"project" yaml:"project"`
	Version     string            `bson:"version" json:"version" yaml:"version"`
	Variant     string            `bson:"variant" json:"variant" yaml:"variant"`
	TaskName    string            `bson:"task_name" json:"task_name" yaml:"task_name"`
	TaskID      string            `bson:"task_id" json:"task_id" yaml:"task_id"`
	Execution   int32             `bson:"execution" json:"execution" yaml:"execution"`
	TestName    string            `bson:"test_name" json:"test_name" yaml:"test_name"`
	Trial       int32             `bson:"trial" json:"trial" yaml:"trial"`
	ProcessName string            `bson:"proc_name" json:"proc_name" yaml:"proc_name"`
	Format      LogFormat         `bson:"format" json:"format" yaml:"format"`
	Tags        []string          `bson:"tags" json:"tags" yaml:"tags"`
	Arguments   map[string]string `bson:"args" json:"args" yaml:"args"`
	Mainline    bool              `bson:"mainline" json:"mainline" yaml:"mainline"`

	// Storage location type for this log.
	Storage LogStorage `bson:"storage" json:"storage" yaml:"storage"`

	// Configure a local sender for "fallback" operations and to collect
	// the location of the buildlogger output.
	Local send.Sender `bson:"-" json:"-" yaml:"-"`

	// The number max number of bytes to buffer before sending log data
	// over rpc to cedar. Defaults to 10MB.
	MaxBufferSize int `bson:"max_buffer_size" json:"max_buffer_size" yaml:"max_buffer_size"`
	// The interval at which to flush log lines, regardless of whether the
	// max buffer size has been reached or not. Setting FlushInterval to a
	// duration less than 0 will disable timed flushes. Defaults to 1
	// minute.
	FlushInterval time.Duration `bson:"flush_interval" json:"flush_interval" yaml:"flush_interval"`

	// Disable checking for new lines in messages. If this is set to true,
	// make sure log messages do not contain new lines, otherwise the logs
	// will be stored incorrectly.
	DisableNewLineCheck bool `bson:"disable_new_line_check" json:"disable_new_line_check" yaml:"disable_new_line_check"`

	// The gRPC client connection. If nil, a new connection will be
	// established with the gRPC connection configuration.
	ClientConn *grpc.ClientConn `bson:"-" json:"-" yaml:"-"`

	// Configuration for gRPC client connection.
	HTTPClient  *http.Client `bson:"-" json:"-" yaml:"-"`
	BaseAddress string       `bson:"base_address" json:"base_address" yaml:"base_address"`
	RPCPort     string       `bson:"rpc_port" json:"rpc_port" yaml:"rpc_port"`
	Insecure    bool         `bson:"insecure" json:"insecure" yaml:"insecure"`
	Username    string       `bson:"username" json:"username" yaml:"username"`
	APIKey      string       `bson:"api_key" json:"api_key" yaml:"api_key"`

	logID    string
	exitCode int32
}

func (opts *LoggerOptions) validate() error {
	if err := opts.Format.validate(); err != nil {
		return err
	}
	if err := opts.Storage.validate(); err != nil {
		return err
	}

	if opts.ClientConn == nil {
		if opts.BaseAddress == "" || opts.RPCPort == "" {
			return errors.New("must specify a base address and rpc port when a client connection is not provided")
		}
		if !opts.Insecure && (opts.Username == "" || opts.APIKey == "") {
			return errors.New("must specify username and API key when making a secure connection over RPC")
		}
	}

	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{}
	}

	if opts.Local == nil {
		opts.Local = send.MakeNative()
		opts.Local.SetName("local")
	}

	if opts.MaxBufferSize == 0 {
		opts.MaxBufferSize = defaultMaxBufferSize
	}

	if opts.FlushInterval == 0 {
		opts.FlushInterval = defaultFlushInterval
	}

	return nil
}

// SetExitCode sets the exit code variable.
func (opts *LoggerOptions) SetExitCode(i int32) { opts.exitCode = i }

// GetLogID returns the unique buildlogger log ID set after NewLogger is
// called.
func (opts *LoggerOptions) GetLogID() string { return opts.logID }

// NewLogger returns a grip Sender backed by cedar Buildlogger with level
// information set.
func NewLogger(name string, l send.LevelInfo, opts *LoggerOptions) (send.Sender, error) {
	return NewLoggerWithContext(context.Background(), name, l, opts)
}

// NewLoggerWithContext returns a grip Sender backed by cedar Buildlogger with
// level information set, using the passed in context.
func NewLoggerWithContext(ctx context.Context, name string, l send.LevelInfo, opts *LoggerOptions) (send.Sender, error) {
	b, err := MakeLoggerWithContext(ctx, name, opts)
	if err != nil {
		return nil, errors.Wrap(err, "problem making new logger")
	}

	if err := b.SetLevel(l); err != nil {
		return nil, errors.Wrap(err, "problem setting grip level")
	}

	return b, nil
}

// MakeLogger returns a grip Sender backed by cedar Buildlogger.
func MakeLogger(name string, opts *LoggerOptions) (send.Sender, error) {
	return MakeLoggerWithContext(context.Background(), name, opts)
}

// MakeLoggerWithContext returns a grip Sender backed by cedar Buildlogger
// using the passed in context.
func MakeLoggerWithContext(ctx context.Context, name string, opts *LoggerOptions) (send.Sender, error) {
	if err := opts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid cedar buildlogger options")
	}

	var conn *grpc.ClientConn
	var err error
	if opts.ClientConn == nil {
		if opts.Insecure {
			rpcOpts := []grpc.DialOption{
				grpc.WithUnaryInterceptor(aviation.MakeRetryUnaryClientInterceptor(10)),
				grpc.WithStreamInterceptor(aviation.MakeRetryStreamClientInterceptor(10)),
				grpc.WithInsecure(),
			}
			opts.ClientConn, err = grpc.DialContext(ctx, opts.BaseAddress+":"+opts.RPCPort, rpcOpts...)
		} else {
			dialOpts := &services.DialCedarOptions{
				BaseAddress: opts.BaseAddress,
				RPCPort:     opts.RPCPort,
				Username:    opts.Username,
				APIKey:      opts.APIKey,
				Retries:     10,
			}
			opts.ClientConn, err = services.DialCedar(ctx, opts.HTTPClient, dialOpts)
		}
		if err != nil {
			return nil, errors.Wrap(err, "problem dialing rpc server")
		}
	}

	b := &buildlogger{
		ctx:    ctx,
		opts:   opts,
		conn:   conn,
		client: gopb.NewBuildloggerClient(opts.ClientConn),
		buffer: []*gopb.LogLine{},
		Base:   send.NewBase(name),
	}

	if err := b.SetErrorHandler(send.ErrorHandlerFromSender(b.opts.Local)); err != nil {
		return nil, errors.Wrap(err, "problem setting default error handler")
	}

	if err := b.createNewLog(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	b.ctx = ctx
	b.cancel = cancel

	if opts.FlushInterval > 0 {
		go b.timedFlush()
	}

	return b, nil
}

// Send sends the given message with a timestamp created when the function is
// called to the cedar Buildlogger backend. This function buffers the messages
// until the maximum allowed buffer size is reached, at which point the
// messages in the buffer are sent to the Buildlogger server via RPC. Send is
// thread safe.
func (b *buildlogger) Send(m message.Composer) {
	if !b.Level().ShouldLog(m) {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	ts := time.Now()
	if b.closed {
		b.opts.Local.Send(message.NewErrorMessage(level.Error, errors.New("cannot call Send on a closed Buildlogger Sender")))
		return
	}

	_, ok := m.(*message.GroupComposer)
	var lines []string
	if b.opts.DisableNewLineCheck && !ok {
		lines = []string{m.String()}
	} else {
		lines = strings.Split(m.String(), "\n")
	}

	for _, line := range lines {
		if line == "" {
			continue
		}
		logLine := &gopb.LogLine{
			Priority:  int32(m.Priority()),
			Timestamp: &timestamp.Timestamp{Seconds: ts.Unix(), Nanos: int32(ts.Nanosecond())},
			Data:      strings.TrimRightFunc(line, unicode.IsSpace),
		}

		b.buffer = append(b.buffer, logLine)
		b.bufferSize += len(logLine.Data)
		if b.bufferSize > b.opts.MaxBufferSize {
			if err := b.flush(b.ctx); err != nil {
				b.opts.Local.Send(message.NewErrorMessage(level.Error, err))
				return
			}
		}
	}
}

// Flush flushes anything messages that may be in the buffer to cedar
// Buildlogger backend via RPC.
func (b *buildlogger) Flush(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	return b.flush(ctx)
}

// Close flushes anything that may be left in the underlying buffer and closes
// out the log with a completed at timestamp and the exit code. If the gRPC
// client connection was created in NewLogger or MakeLogger, this connection is
// also closed. Close is thread safe but should only be called once no more
// calls to Send are needed; after Close has been called any subsequent calls
// to Send will error. After the first call to Close subsequent calls will
// no-op.
func (b *buildlogger) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	defer b.cancel()

	if b.closed {
		return nil
	}
	catcher := grip.NewBasicCatcher()

	if len(b.buffer) > 0 {
		if err := b.flush(b.ctx); err != nil {
			b.opts.Local.Send(message.NewErrorMessage(level.Error, err))
			catcher.Add(errors.Wrap(err, "problem flushing buffer"))
		}
	}

	if !catcher.HasErrors() {
		endInfo := &gopb.LogEndInfo{
			LogId:    b.opts.logID,
			ExitCode: b.opts.exitCode,
		}
		_, err := b.client.CloseLog(b.ctx, endInfo)
		b.opts.Local.Send(message.NewErrorMessage(level.Error, err))
		catcher.Add(errors.Wrap(err, "problem closing log"))
	}

	if b.conn != nil {
		catcher.Add(b.conn.Close())
	}

	b.closed = true

	return catcher.Resolve()
}

func (b *buildlogger) createNewLog() error {
	data := &gopb.LogData{
		Info: &gopb.LogInfo{
			Project:   b.opts.Project,
			Version:   b.opts.Version,
			Variant:   b.opts.Variant,
			TaskName:  b.opts.TaskName,
			TaskId:    b.opts.TaskID,
			Execution: b.opts.Execution,
			TestName:  b.opts.TestName,
			Trial:     b.opts.Trial,
			ProcName:  b.opts.ProcessName,
			Format:    gopb.LogFormat(b.opts.Format),
			Tags:      b.opts.Tags,
			Arguments: b.opts.Arguments,
			Mainline:  b.opts.Mainline,
		},
		Storage: gopb.LogStorage(b.opts.Storage),
	}
	resp, err := b.client.CreateLog(b.ctx, data)
	if err != nil {
		b.opts.Local.Send(message.NewErrorMessage(level.Error, err))
		return errors.Wrap(err, "problem creating log")
	}
	b.opts.logID = resp.LogId

	return nil
}

func (b *buildlogger) timedFlush() {
	b.mu.Lock()
	b.timer = time.NewTimer(b.opts.FlushInterval)
	b.mu.Unlock()
	defer b.timer.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-b.timer.C:
			b.mu.Lock()
			if len(b.buffer) > 0 && time.Since(b.lastFlush) >= b.opts.FlushInterval {
				if err := b.flush(b.ctx); err != nil {
					b.opts.Local.Send(message.NewErrorMessage(level.Error, err))
				}
			}
			_ = b.timer.Reset(b.opts.FlushInterval)
			b.mu.Unlock()
		}
	}
}

func (b *buildlogger) flush(ctx context.Context) error {
	_, err := b.client.AppendLogLines(ctx, &gopb.LogLines{
		LogId: b.opts.logID,
		Lines: b.buffer,
	})
	if err != nil {
		return err
	}

	b.buffer = []*gopb.LogLine{}
	b.bufferSize = 0
	b.lastFlush = time.Now()

	return nil
}
