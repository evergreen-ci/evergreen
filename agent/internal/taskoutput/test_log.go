package taskoutput

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/evergreen/taskoutput"
	"github.com/evergreen-ci/timber/buildlogger"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/papertrail/go-tail/follower"
	"github.com/pkg/errors"
	"github.com/radovskyb/watcher"
	"gopkg.in/yaml.v2"
)

// AppendTestLog appends log lines to the specified test log for the given task
// run.
func AppendTestLog(ctx context.Context, comm client.Communicator, tsk *task.Task, testLog *testlog.TestLog) error {
	if tsk.TaskOutputInfo.TestLogs.Version == 0 {
		return sendTestLogToCedar(ctx, comm, tsk, testLog)
	}

	var lines []log.LogLine
	for i := range testLog.Lines {
		for _, line := range strings.Split(testLog.Lines[i], "\n") {
			lines = append(lines, log.LogLine{
				Priority:  level.Info,
				Timestamp: time.Now().UnixNano(),
				Data:      line,
			})
		}
	}

	return tsk.TaskOutputInfo.TestLogs.Append(ctx, taskoutput.TaskOptions{
		ProjectID: tsk.Project,
		TaskID:    tsk.Id,
		Execution: tsk.Execution,
	}, testLog.Name, lines)
}

// TODO (DEVPROD-75): Remove this logic once we cut over to Evergreen logs.
func sendTestLogToCedar(ctx context.Context, comm client.Communicator, tsk *task.Task, testLog *testlog.TestLog) error {
	conn, err := comm.GetCedarGRPCConn(ctx)
	if err != nil {
		return errors.Wrapf(err, "getting the Cedar gRPC connection for test '%s'", testLog.Name)
	}

	timberOpts := &buildlogger.LoggerOptions{
		Project:    tsk.Project,
		Version:    tsk.Version,
		Variant:    tsk.BuildVariant,
		TaskName:   tsk.DisplayName,
		TaskID:     tsk.Id,
		Execution:  int32(tsk.Execution),
		TestName:   testLog.Name,
		Mainline:   !tsk.IsPatchRequest(),
		Storage:    buildlogger.LogStorageS3,
		ClientConn: conn,
	}
	levelInfo := send.LevelInfo{Default: level.Info, Threshold: level.Debug}
	sender, err := buildlogger.NewLoggerWithContext(ctx, testLog.Name, levelInfo, timberOpts)
	if err != nil {
		return errors.Wrapf(err, "creating buildlogger logger for test result '%s'", testLog.Name)
	}

	sender.Send(message.ConvertToComposer(level.Info, strings.Join(testLog.Lines, "\n")))
	if err = sender.Close(); err != nil {
		return errors.Wrapf(err, "closing buildlogger logger for test result '%s'", testLog.Name)
	}

	return nil
}

// testLogDirectoryHandler implements automatic and asynchronous task output
// handling for the reserved test log directory.
type testLogDirectoryHandler struct {
	dir             string
	logger          client.LoggerProducer
	watcher         *watcher.Watcher
	spec            testLogSpec
	createSender    func(context.Context, string) (send.Sender, error)
	followFileCount int

	once sync.Once
	wg   sync.WaitGroup
}

// newTestLogDirectoryHandler returns a new test log directory handler for the
// specified task.
func newTestLogDirectoryHandler(output taskoutput.TestLogOutput, taskOpts taskoutput.TaskOptions, logger client.LoggerProducer) *testLogDirectoryHandler {
	h := &testLogDirectoryHandler{logger: logger}
	h.createSender = func(ctx context.Context, logPath string) (send.Sender, error) {
		return output.NewSender(ctx, taskOpts, taskoutput.EvergreenSenderOptions{
			Local: logger.Task().GetSender(),
			Parse: h.spec.getParser(),
		}, logPath)
	}

	return h
}

func (h *testLogDirectoryHandler) start(ctx context.Context, dir string) error {
	h.dir = dir
	h.watcher = watcher.New()
	h.watcher.FilterOps(watcher.Create)
	if err := h.watcher.AddRecursive(h.dir); err != nil {
		return errors.Wrap(err, "configuring test log directory watcher as recursive")
	}
	if err := h.watcher.Ignore(filepath.Join(h.dir, testLogSpecFilename)); err != nil {
		return errors.Wrap(err, "configuring test log directory watcher to ignore the test log spec file")
	}

	started := make(chan struct{})
	startErr := make(chan error)
	go func() {
		h.watcher.Wait()
		close(started)
	}()
	go func() {
		defer func() {
			h.logger.Execution().Critical(recovery.HandlePanicWithError(recover(), nil, "test log directory watcher start"))
		}()

		if err := h.watcher.Start(time.Millisecond); err != nil {
			startErr <- err
		}
	}()

	select {
	case <-started:
	case err := <-startErr:
		return errors.Wrap(err, "starting test log directory watcher")
	}

	go h.watch(ctx)

	return nil
}

// watch recursively watches the reserved test log directory for newly created
// log files. When a new file is detected, an asynchronous file follower is
// created to ingest the log data as it is written for the duration of the
// task.
func (h *testLogDirectoryHandler) watch(ctx context.Context) {
	defer func() {
		h.logger.Execution().Critical(recovery.HandlePanicWithError(recover(), nil, "test log directory watcher"))
	}()

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for {
		select {
		case event := <-h.watcher.Event:
			if event.IsDir() {
				continue
			}

			h.followFileCount++
			h.wg.Add(1)
			go h.followFile(workerCtx, event)
		case err := <-h.watcher.Error:
			h.logger.Execution().Critical(errors.Wrap(err, "watching test log directory"))
			return
		case <-ctx.Done():
			h.logger.Execution().Warning("context canceled, exiting test log directory watcher")
			return
		case <-h.watcher.Closed:
			return
		}
	}
}

// getSpecFile looks for the test log specification file in the top level of
// the reserved test log directory. If the spec file cannot be read for any
// reason, an error is logged and the handler uses the default spec.
//
// Called once per task run immediately after the first log file is detected
// and before any data is ingested.
func (h *testLogDirectoryHandler) getSpecFile() {
	h.logger.Task().Infof("detected first test log file, getting test log spec file")

	data, err := os.ReadFile(filepath.Join(h.dir, testLogSpecFilename))
	if err != nil {
		h.logger.Task().Warning(errors.Wrap(err, "reading test log spec; falling back to default spec"))
		return
	}
	if err = yaml.Unmarshal(data, &h.spec); err != nil {
		h.logger.Task().Warning(errors.Wrap(err, "unmarshalling test log spec; falling back to default spec"))
		return
	}

	if err = h.spec.Format.validate(); err != nil {
		h.logger.Task().Warning(errors.Wrapf(err, "invalid test log format specified; falling back to default text format"))
	}
}

// followFile tails a test log file for the duration of a task, ingesting and
// persisting log lines as they are written.
func (h *testLogDirectoryHandler) followFile(ctx context.Context, event watcher.Event) {
	defer func() {
		h.logger.Task().Critical(recovery.HandlePanicWithError(recover(), nil, "test log file follower"))
	}()
	defer h.wg.Done()

	h.once.Do(h.getSpecFile)

	h.logger.Task().Infof("new test log file '%s' found, initiating automated ingestion", event.Path)

	// The persisted log path should be relative to the reserved directory.
	logPath, err := filepath.Rel(h.dir, event.Path)
	if err != nil {
		h.logger.Task().Error(errors.Wrapf(err, "getting relative path for test log file '%s'", event.Path))
		return
	}

	t, err := follower.New(event.Path, follower.Config{
		Whence: io.SeekStart,
		Offset: 0,
		Reopen: true,
	})
	if err != nil {
		h.logger.Task().Error(errors.Wrapf(err, "creating follower for test log file '%s'", event.Path))
		return
	}
	defer t.Close()

	sender, err := h.createSender(ctx, logPath)
	if err != nil {
		h.logger.Task().Error(errors.Wrapf(err, "creating Sender for test log '%s'", event.Path))
		return
	}
	defer func() {
		if err = t.Err(); err != nil {
			h.logger.Task().Error(errors.Wrapf(err, "following test log file '%s'", event.Path))
		}
		if err = sender.Close(); err != nil {
			h.logger.Task().Error(errors.Wrapf(err, "closing Sender for test log '%s'", event.Path))
		}
	}()

	lines := t.Lines()
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-lines:
			if !ok {
				return
			}
			sender.Send(message.NewDefaultMessage(level.Info, line.String()))
		}
	}
}

func (h *testLogDirectoryHandler) close(_ context.Context) error {
	h.watcher.Close()
	h.wg.Wait()

	return nil
}

// testLogSpec represents the test log specification file written at the top
// level of the reserved test log directory.
//
// The spec file enables schema versioning, robust log parsing, and richer
// feature development.
type testLogSpec struct {
	SchemaVersion string        `yaml:"schema_version"`
	Format        testLogFormat `yaml:"format"`
}

const testLogSpecFilename = "log_spec.yaml"

func (s testLogSpec) getParser() taskoutput.LogLineParser {
	switch s.Format {
	case testLogFormatTextTimestamp:
		return func(data string) (log.LogLine, error) {
			lineParts := strings.SplitN(strings.TrimSpace(data), " ", 2)
			if len(lineParts) != 2 {
				return log.LogLine{}, errors.New("malformed log line")
			}

			ts, err := strconv.ParseInt(lineParts[0], 10, 64)
			if err != nil {
				return log.LogLine{}, errors.Wrap(err, "invalid log timestamp prefix")
			}

			return log.LogLine{
				Timestamp: ts,
				Data:      strings.TrimSuffix(lineParts[1], "\n"),
			}, nil
		}
	default:
		// Use the default log line parser.
		return nil
	}
}

// testLogFormat specifies the expected format of log lines written to files in
// the test log directory. The format maps to a log line parser.
type testLogFormat string

const (
	// testLogFormatDefault is a plain text string.
	testLogFormatDefault testLogFormat = "text"
	// testLogFormatTextWithTimestamp is a plain text string prefixed by a
	// Unix timestamp in nanoseconds and one or more whitespace characters.
	// 		1575743479637000000 This is a log line.
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
