package taskoutput

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/papertrail/go-tail/follower"
	"github.com/pkg/errors"
	"github.com/radovskyb/watcher"
	"gopkg.in/yaml.v2"
)

// TestLogOutput is the versioned entry point for coordinating persistent
// storage of a task run's test log data.
type TestLogOutput struct {
	Version      int                    `bson:"version" json:"version"`
	BucketConfig evergreen.BucketConfig `bson:"bucket_config" json:"bucket_config"`
}

// ID returns the unique identifier of the test log output type.
func (TestLogOutput) ID() string { return "test_logs" }

// TestLogGetOptions represents the arguments for fetching test logs belonging
// to a task run.
type TestLogGetOptions struct {
	// LogPaths are the paths of the logs to fetch and merge, prefixes may
	// be specified. At least one value must be specified.
	LogPaths []string
	// Start is the start time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Defaults to the
	// first timestamp of the first specified log.
	Start *int64
	// End is the end time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Defaults to the last
	// timestamp of the first specified log.
	End *int64
	// LineLimit limits the number of lines read from the log. Ignored if
	// less than or equal to 0.
	LineLimit int
	// TailN is the number of lines to read from the tail of the log.
	// Ignored if less than or equal to 0.
	TailN int
}

// NewSender returns a new test log sender for the given task run.
func (o TestLogOutput) NewSender(ctx context.Context, taskOpts TaskOptions, senderOpts EvergreenSenderOptions, logPath string) (send.Sender, error) {
	svc, err := o.getLogService(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting log service")
	}

	senderOpts.appendLines = func(ctx context.Context, lines []log.LogLine) error {
		return svc.Append(ctx, o.getLogNames(taskOpts, []string{logPath})[0], lines)
	}

	return newEvergreenSender(ctx, fmt.Sprintf("%s-%s", taskOpts.TaskID, logPath), senderOpts)
}

// Append appends log lines to the specified test log for the given task run.
func (o TestLogOutput) Append(ctx context.Context, taskOpts TaskOptions, logPath string, lines []log.LogLine) error {
	svc, err := o.getLogService(ctx)
	if err != nil {
		return errors.Wrap(err, "getting log service")
	}

	return svc.Append(ctx, o.getLogNames(taskOpts, []string{logPath})[0], lines)
}

// Get returns test logs belonging to the specified task run.
func (o TestLogOutput) Get(ctx context.Context, env evergreen.Environment, taskOpts TaskOptions, getOpts TestLogGetOptions) (log.LogIterator, error) {
	if o.Version == 0 {
		return o.getBuildloggerLogs(ctx, env, taskOpts, getOpts)
	}

	svc, err := o.getLogService(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting log service")
	}

	return svc.Get(ctx, log.GetOptions{
		LogNames:                   o.getLogNames(taskOpts, getOpts.LogPaths),
		Start:                      getOpts.Start,
		End:                        getOpts.End,
		DefaultTimeRangeOfFirstLog: true,
		LineLimit:                  getOpts.LineLimit,
		TailN:                      getOpts.TailN,
	})
}

func (o TestLogOutput) getLogNames(taskOpts TaskOptions, logPaths []string) []string {
	prefix := fmt.Sprintf("%s/%s/%d/%s", taskOpts.ProjectID, taskOpts.TaskID, taskOpts.Execution, o.ID())

	logNames := make([]string, len(logPaths))
	for i, path := range logPaths {
		logNames[i] = fmt.Sprintf("%s/%s", prefix, path)
	}

	return logNames
}

func (o TestLogOutput) getLogService(ctx context.Context) (log.LogService, error) {
	b, err := newBucket(ctx, o.BucketConfig)
	if err != nil {
		return nil, err
	}

	return log.NewLogServiceV0(b), nil
}

// getBuildloggerLogs makes request to Cedar Buildlogger for logs.
func (o TestLogOutput) getBuildloggerLogs(ctx context.Context, env evergreen.Environment, taskOpts TaskOptions, getOpts TestLogGetOptions) (log.LogIterator, error) {
	if len(getOpts.LogPaths) != 1 {
		return nil, errors.New("must request exactly one test log from Cedar Buildlogger")
	}

	return apimodels.GetBuildloggerLogs(ctx, apimodels.GetBuildloggerLogsOptions{
		BaseURL:   env.Settings().Cedar.BaseURL,
		TaskID:    taskOpts.TaskID,
		Execution: utility.ToIntPtr(taskOpts.Execution),
		TestName:  getOpts.LogPaths[0],
		Start:     utility.FromInt64Ptr(getOpts.Start),
		End:       utility.FromInt64Ptr(getOpts.End),
		Limit:     getOpts.LineLimit,
		Tail:      getOpts.TailN,
	})
}

type testLogDirectoryHandler struct {
	dir          string
	logger       grip.Journaler
	watcher      *watcher.Watcher
	spec         *testLogSpec
	createSender func(context.Context, string) (send.Sender, error)

	mu sync.Mutex
}

func (o TestLogOutput) newDirectoryHandler(rootDir string, taskOpts TaskOptions, logger grip.Journaler) *testLogDirectoryHandler {
	h := &testLogDirectoryHandler{
		dir:    filepath.Join(rootDir, o.ID()),
		logger: logger,
	}

	h.watcher = watcher.New()
	h.watcher.FilterOps(watcher.Create)
	h.watcher.AddRecursive(h.dir)

	h.createSender = func(ctx context.Context, logPath string) (send.Sender, error) {
		return o.NewSender(ctx, taskOpts, EvergreenSenderOptions{
			Local: logger.GetSender(),
			parse: h.spec.getParser(),
		}, logPath)
	}

	return h
}

func (h *testLogDirectoryHandler) start(ctx context.Context) error {
	if err := h.watcher.Start(time.Second); err != nil {
		return errors.Wrap(err, "starting test log directory watcher")
	}

	go h.watch(ctx)

	return nil
}

func (h *testLogDirectoryHandler) watch(ctx context.Context) {
	defer func() {
		h.logger.Critical(recovery.HandlePanicWithError(recover(), nil, "test log directory watcher"))
	}()

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for {
		select {
		case event := <-h.watcher.Event:
			go h.followFile(workerCtx, event)
		case err := <-h.watcher.Error:
			h.logger.Critical(errors.Wrap(err, "watching test log directory"))
			return
		case <-ctx.Done():
			h.logger.Warning("context canceled, exiting test log directory watcher")
			return
		case <-h.watcher.Closed:
			return
		}
	}
}

func (h *testLogDirectoryHandler) getSpecFile() {
	if !h.mu.TryLock() {
		return
	}
	defer h.mu.Unlock()

	h.spec = &testLogSpec{}

	data, err := os.ReadFile(filepath.Join(h.dir, "log_spec.yaml"))
	if err != nil {
		h.logger.Warning(errors.Wrap(err, "reading test log spec; falling back to default spec"))
		return
	}
	if err = yaml.Unmarshal(data, h.spec); err != nil {
		h.logger.Warning(errors.Wrap(err, "unmarshalling test log spec; falling back to default spec"))
		return
	}

	if err = h.spec.Format.validate(); err != nil {
		h.logger.Warning(errors.Wrapf(err, "invalid test log format specified; falling back to default text format"))
	}
}

func (h *testLogDirectoryHandler) followFile(ctx context.Context, event watcher.Event) {
	defer func() {
		h.logger.Critical(recovery.HandlePanicWithError(recover(), nil, "test log file follower"))
	}()

	if event.IsDir() {
		return
	}

	h.logger.Infof("new test log file '%s' found, initiating automated ingestion", event.Path)

	if h.spec == nil {
		h.getSpecFile()
	}

	t, err := follower.New(event.Path, follower.Config{
		Whence: io.SeekEnd,
		Offset: 0,
		Reopen: true,
	})
	if err != nil {
		h.logger.Error(errors.Wrapf(err, "creating follower for test log file '%s'", event.Path))
		return
	}
	defer t.Close()

	sender, err := h.createSender(ctx, event.Path)
	if err != nil {
		h.logger.Error(errors.Wrapf(err, "creating Sender for test log '%s'", event.Path))
		return
	}
	defer func() {
		if err = sender.Close(); err != nil {
			h.logger.Error(errors.Wrapf(err, "closing Sender for test log '%s'", event.Path))
		}
	}()

	lines := t.Lines()
	for {
		select {
		case line := <-lines:
			sender.Send(message.NewDefaultMessage(level.Info, line.String()))
		case <-ctx.Done():
			break
		}
	}

	if err := t.Err(); err != nil {
		h.logger.Error(errors.Wrapf(err, "following test log file '%s'", event.Path))
	}
}

func (w *testLogDirectoryHandler) close(_ context.Context) error {
	w.watcher.Close()

	return nil
}

type testLogSpec struct {
	SchemaVersion string        `yaml:"schema_version"`
	Format        testLogFormat `yaml:"format"`
}

func (s testLogSpec) getParser() logLineParser {
	switch s.Format {
	case testLogFormatTextTimestamp:
		return func(data string) (log.LogLine, error) {
			lineParts := strings.SplitN(data, " ", 2)
			if len(lineParts) != 2 {
				return log.LogLine{}, errors.New("malformed log line")
			}

			ts, err := strconv.ParseInt(strings.TrimSpace(lineParts[0]), 10, 64)
			if err != nil {
				return log.LogLine{}, errors.Wrap(err, "malformed log timestamp prefix")
			}

			return log.LogLine{
				Timestamp: ts,
				Data:      strings.TrimSuffix(lineParts[2], "\n"),
			}, nil
		}
	default:
		return nil
	}
}

type testLogFormat string

const (
	testLogFormatDefault       testLogFormat = "text"
	testLogFormatTextTimestamp testLogFormat = "text-timestamp"
)

func (f testLogFormat) validate() error {
	switch f {
	case testLogFormatDefault, testLogFormatTextTimestamp:
		return nil
	default:
		return errors.Errorf("unrecognized test log format '%s'", f)
	}
}
