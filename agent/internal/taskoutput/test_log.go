package taskoutput

import (
	"bufio"
	"context"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/agent/internal/redactor"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/evergreen/taskoutput"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"gopkg.in/yaml.v2"
)

// AppendTestLog appends log lines to the specified test log for the given task
// run.
func AppendTestLog(ctx context.Context, tsk *task.Task, redactionOpts redactor.RedactionOptions, testLog *testlog.TestLog) error {
	taskOpts := taskoutput.TaskOptions{
		ProjectID: tsk.Project,
		TaskID:    tsk.Id,
		Execution: tsk.Execution,
	}
	sender, err := tsk.TaskOutputInfo.TestLogs.NewSender(ctx, taskOpts, taskoutput.EvergreenSenderOptions{}, testLog.Name, 0)
	if err != nil {
		return errors.Wrapf(err, "creating Evergreen logger for test log '%s'", testLog.Name)
	}
	sender = redactor.NewRedactingSender(sender, redactionOpts)
	sender.Send(message.ConvertToComposer(level.Info, strings.Join(testLog.Lines, "\n")))

	return errors.Wrapf(sender.Close(), "closing Evergreen logger for test result '%s'", testLog.Name)
}

// testLogDirectoryHandler implements automatic task output handling for the
// reserved test log directory.
type testLogDirectoryHandler struct {
	dir          string
	logger       client.LoggerProducer
	spec         testLogSpec
	createSender func(context.Context, string, int) (send.Sender, error)
	logFileCount int
}

// newTestLogDirectoryHandler returns a new test log directory handler for the
// specified task.
func newTestLogDirectoryHandler(dir string, output *taskoutput.TaskOutput, taskOpts taskoutput.TaskOptions, redactionOpts redactor.RedactionOptions, logger client.LoggerProducer) directoryHandler {
	h := &testLogDirectoryHandler{
		dir:    dir,
		logger: logger,
	}
	h.createSender = func(ctx context.Context, logPath string, sequence int) (send.Sender, error) {
		evgSender, err := output.TestLogs.NewSender(ctx, taskOpts, taskoutput.EvergreenSenderOptions{
			Local: logger.Task().GetSender(),
			Parse: h.spec.getParser(),
		}, logPath, sequence)
		if err != nil {
			return nil, errors.Wrap(err, "making test log sender")
		}
		return evgSender, nil
		//return redactor.NewRedactingSender(evgSender, redactionOpts), nil
	}

	return h
}

func (h *testLogDirectoryHandler) run(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "test-log-ingestion")
	defer span.End()

	h.getSpecFile()

	var wg sync.WaitGroup
	ignore := filepath.Join(h.dir, testLogSpecFilename)
	err := filepath.WalkDir(h.dir, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			h.logger.Execution().Warning(errors.Wrap(err, "walking test log directory"))
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if info.IsDir() {
			return nil
		}
		if path == ignore {
			return nil
		}

		h.logFileCount++

		fileInfo, err := info.Info()
		if err != nil {
			h.logger.Execution().Warning(errors.Wrap(err, "getting test log file info"))
			return nil
		}

		seqSize := int64(1e7)
		fileSize := fileInfo.Size()
		type workInfo struct {
			sequence int
			offset   int64
			limit    int64
		}
		work := make(chan workInfo, int(math.Ceil(float64(fileSize)/float64(seqSize))))
		var (
			currentByte int64
			sequence    int
		)
		for currentByte < fileSize {
			work <- workInfo{
				sequence: sequence,
				offset:   currentByte,
				limit:    currentByte + seqSize,
			}

			currentByte += seqSize
			sequence++
		}
		close(work)

		for i := 0; i < runtime.NumCPU(); i++ {
			wg.Add(1)
			go func() {
				defer func() {
					h.logger.Task().Critical(recovery.HandlePanicWithError(recover(), nil, "test log ingestion worker"))
					wg.Done()
				}()

				for chunk := range work {
					if err := ctx.Err(); err != nil {
						h.logger.Execution().Warning(errors.Wrap(err, "context error test log ingestion worker"))
						return
					}

					h.ingest(ctx, path, chunk.sequence, chunk.offset, chunk.limit)
				}
			}()
		}

		/*
			// TODO: probably want to only break up the file if it's large.
			var (
				currentByte int64
				sequence    int
			)
			fileSize := fileInfo.Size()
			chunkSize := int64(math.Ceil(float64(fileSize) / float64(runtime.NumCPU())))
			for currentByte < fileSize {
				wg.Add(1)
				go func(sequence int, offset, limit int64) {
					defer func() {
						h.logger.Task().Critical(recovery.HandlePanicWithError(recover(), nil, "ingesting test log"))
					}()
					defer wg.Done()

					h.ingest(ctx, path, sequence, offset, limit)
				}(sequence, currentByte, currentByte+chunkSize)

				currentByte += chunkSize
				sequence++
			}
		*/

		return nil
	})
	wg.Wait()

	span.SetAttributes(attribute.KeyValue{Key: "test_log_file_count", Value: attribute.IntValue(h.logFileCount)})

	return err
}

// getSpecFile looks for the test log specification file in the top level of
// the reserved test log directory. If the spec file cannot be read for any
// reason, an error is logged and the handler uses the default spec.
//
// Called once per task run before sweeping the directory for test log files.
func (h *testLogDirectoryHandler) getSpecFile() {
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

// ingest reads and ships a test log file.
func (h *testLogDirectoryHandler) ingest(ctx context.Context, path string, sequence int, offset, limit int64) {
	h.logger.Task().Infof("new test log file '%s' found, initiating automated ingestion", path)

	// The persisted log path should be relative to the reserved directory
	// and contain only slash ('/') separators.
	logPath, err := filepath.Rel(h.dir, path)
	if err != nil {
		h.logger.Task().Error(errors.Wrapf(err, "getting relative path for test log file '%s'", path))
		return
	}
	logPath = filepath.ToSlash(logPath)
	h.logger.Task().Infof("storing test log file '%s' as '%s'", path, logPath)

	f, err := os.Open(path)
	if err != nil {
		h.logger.Task().Error(errors.Wrapf(err, "opening test log file '%s'", path))
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			h.logger.Task().Error(errors.Wrapf(err, "closing test log file '%s'", path))
		}
	}()

	sender, err := h.createSender(ctx, logPath, sequence)
	if err != nil {
		h.logger.Task().Error(errors.Wrapf(err, "creating Sender for test log '%s'", path))
		return
	}

	if offset > 0 {
		_, err := f.Seek(offset-1, io.SeekStart)
		if err != nil {
			h.logger.Task().Error(errors.Wrapf(err, "seeking offset for test log '%s'", path))
			return
		}
	}
	r := bufio.NewReader(f)
	data, err := r.Peek(1)
	if err == io.EOF {
		return
	}
	if err != nil {
		h.logger.Task().Error(errors.Wrapf(err, "reading test log '%s'", path))
		return
	}
	currentPos := offset
	if offset > 0 && data[0] != '\n' {
		data, err := r.ReadBytes('\n')
		if err == io.EOF {
			return
		}
		if err != nil {
			h.logger.Task().Error(errors.Wrapf(err, "reading test log '%s'", path))
			return
		}
		currentPos += int64(len(data)) - 1
	}
	for currentPos < limit {
		data, err := r.ReadBytes('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			h.logger.Task().Error(errors.Wrapf(err, "reading test log '%s'", path))
			return
		}
		currentPos += int64(len(data)) - 1

		sender.Send(message.NewDefaultMessage(level.Info, string(data)))
	}

	if err = sender.Close(); err != nil {
		h.logger.Task().Error(errors.Wrapf(err, "closing Sender for test log '%s'", path))
	}
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
		return func(line string) (log.LogLine, error) {
			lineParts := strings.SplitN(strings.TrimSpace(line), " ", 2)

			ts, err := strconv.ParseInt(lineParts[0], 10, 64)
			if err != nil {
				return log.LogLine{}, errors.Wrap(err, "invalid log timestamp prefix")
			}

			var data string
			if len(lineParts) == 2 {
				data = lineParts[1]
			}

			return log.LogLine{
				Timestamp: ts,
				Data:      data,
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
