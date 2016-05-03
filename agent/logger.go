package agent

import (
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
)

// StreamLogger holds a set of stream-delineated loggers. Each logger is used
// to communicate different kinds of logs to the API Server or local file system.
// StreamLogger is used to distinguish logs of system statistics, shell output,
// and internal agent logs.
type StreamLogger struct {
	// Local is used for file system logging on the host the agent is running on.
	Local *slogger.Logger
	// System is used for logging system stats gotten from commands like df, ps, etc.
	System *slogger.Logger
	// Task is used for logging command input, output and errors of the task.
	Task *slogger.Logger
	// Execution is used for logging the agent's internal state.
	Execution *slogger.Logger

	// apiLogger is used to send data back to the API server.
	apiLogger APILogger
}

// GetTaskLogWriter returns an io.Writer of the given level. Useful for
// working with other libraries seamlessly.
func (lgr *StreamLogger) GetTaskLogWriter(level slogger.Level) io.Writer {
	return &evergreen.LoggingWriter{
		Logger:   lgr.Task,
		Severity: level,
	}
}

// LogLocal logs a message to the agent logs on the machine's local file system.
//
// Anything logged by this method will not be sent to the server, so only use it
// to log information that would only be useful when debugging locally.
func (lgr *StreamLogger) LogLocal(level slogger.Level, messageFmt string, args ...interface{}) {
	lgr.Local.Logf(level, messageFmt, args...)
}

// LogExecution logs a message related to the agent's internal workings.
//
// Internally this is used to log things like heartbeats and command internals that
// would pollute the regular task test output.
func (lgr *StreamLogger) LogExecution(level slogger.Level, messageFmt string, args ...interface{}) {
	lgr.Execution.Logf(level, messageFmt, args...)
}

// LogTask logs a message to the task's logs.
//
// This log type is for main task input and output. LogTask should be used for logging
// first-class information like test results and shell script output.
func (lgr *StreamLogger) LogTask(level slogger.Level, messageFmt string, args ...interface{}) {
	lgr.Task.Logf(level, messageFmt, args...)
}

// LogSystem logs passive system messages.
//
// Internally this is used for periodically logging process information and CPU usage.
func (lgr *StreamLogger) LogSystem(level slogger.Level, messageFmt string, args ...interface{}) {
	lgr.System.Logf(level, messageFmt, args...)
}

// Flush flushes the logs to the server. Returns immediately.
func (lgr *StreamLogger) Flush() {
	lgr.apiLogger.Flush()
}

// FlushAndWait flushes and blocks until the HTTP request to send the logs has completed.
// This works in contrast with Flush, which triggers the flush asynchronously.
func (lgr *StreamLogger) FlushAndWait() int {
	return lgr.apiLogger.FlushAndWait()
}

// Wraps an Logger, with additional context about which command is currently being run.
type CommandLogger struct {
	commandName string
	logger      *StreamLogger
}

func (cmdLgr *CommandLogger) addCommandToMsgAndArgs(messageFmt string, args []interface{}) (string, []interface{}) {
	return "[%v] " + messageFmt, append([]interface{}{cmdLgr.commandName}, args...)
}

func (cmdLgr *CommandLogger) GetTaskLogWriter(level slogger.Level) io.Writer {
	return cmdLgr.logger.GetTaskLogWriter(level)
}

func (cmdLgr *CommandLogger) LogLocal(level slogger.Level, messageFmt string, args ...interface{}) {
	messageFmt, args = cmdLgr.addCommandToMsgAndArgs(messageFmt, args)
	cmdLgr.logger.LogLocal(level, messageFmt, args...)
}

func (cmdLgr *CommandLogger) LogExecution(level slogger.Level, messageFmt string, args ...interface{}) {
	messageFmt, args = cmdLgr.addCommandToMsgAndArgs(messageFmt, args)
	cmdLgr.logger.LogExecution(level, messageFmt, args...)
}

func (cmdLgr *CommandLogger) LogTask(level slogger.Level, messageFmt string, args ...interface{}) {
	messageFmt, args = cmdLgr.addCommandToMsgAndArgs(messageFmt, args)
	cmdLgr.logger.LogTask(level, messageFmt, args...)
}

func (cmdLgr *CommandLogger) LogSystem(level slogger.Level, messageFmt string, args ...interface{}) {
	messageFmt, args = cmdLgr.addCommandToMsgAndArgs(messageFmt, args)
	cmdLgr.logger.LogSystem(level, messageFmt, args...)
}

func (cmdLgr *CommandLogger) Flush() {
	cmdLgr.logger.Flush()
}

// NewStreamLogger creates a StreamLogger wrapper for the apiLogger with a given timeoutWatcher.
// Any logged messages on the StreamLogger will reset the TimeoutWatcher.
func NewStreamLogger(timeoutWatcher *TimeoutWatcher, apiLgr *APILogger, logFile string) (*StreamLogger, error) {
	localLogger := slogger.StdOutAppender()

	if logFile != "" {
		appendingFile, err := util.GetAppendingFile(logFile)
		if err != nil {
			return nil, err
		}
		localLogger = &slogger.FileAppender{appendingFile}
	}

	localLoggers := []slogger.Appender{localLogger}
	defaultLoggers := []slogger.Appender{localLogger, apiLgr}

	timeoutLogger := &TimeoutResetLogger{timeoutWatcher, apiLgr}

	return &StreamLogger{
		Local: &slogger.Logger{
			Prefix:    "local",
			Appenders: localLoggers,
		},

		System: &slogger.Logger{
			Prefix:    model.SystemLogPrefix,
			Appenders: defaultLoggers,
		},

		Task: &slogger.Logger{
			Prefix:    model.TaskLogPrefix,
			Appenders: []slogger.Appender{localLogger, timeoutLogger},
		},

		Execution: &slogger.Logger{
			Prefix:    model.AgentLogPrefix,
			Appenders: defaultLoggers,
		},
	}, nil
}

// TimeoutResetLogger wraps any slogger.Appender and resets a TimeoutWatcher
// each time any log message is appended to it.
type TimeoutResetLogger struct {
	*TimeoutWatcher
	slogger.Appender
}

// Append passes the message to the underlying appender, and resets the timeout
func (trLgr *TimeoutResetLogger) Append(log *slogger.Log) error {
	trLgr.TimeoutWatcher.CheckIn()
	return trLgr.Appender.Append(log)
}

// APILogger is a slogger.Appender which makes a call to the
// remote service's log endpoint after SendAfterLines messages have been
// received (or if set, after SendAfterDuration time has passed with no flush).
type APILogger struct {
	// An internal buffer of messages to send.
	messages []model.LogMessage

	// a mutex to ensure only one flush attempt is in progress at a time.
	flushLock sync.Mutex

	// mutex to protect appends to the buffer so that messages don't get lost
	// if both a flush/append happen concurrently
	appendLock sync.Mutex

	// last time flush actually flushed lines
	lastFlush time.Time

	// The number of log lines that the buffer must reach to trigger a flush
	SendAfterLines int

	// How long to wait without any flushes before triggering one automatically
	SendAfterDuration time.Duration

	// Timer to trigger autoflushing based on SendAfterDuration.
	// When set to nil, flushing will only happen when called directly
	// or the SendAfterLines threshold is reached.
	autoFlushTimer *time.Timer

	// Channel on which to send signals that must be propagated back to agent;
	// for instance, if the secret doesn't match what the server expects,
	// it must send IncorrectSecret on the channel.
	signalChan chan Signal

	// The mechanism for communicating with the remote endpoint.
	TaskCommunicator
}

// NewAPILogger creates an initialized logger around the given TaskCommunicator.
func NewAPILogger(tc TaskCommunicator) *APILogger {
	sendAfterDuration := 5 * time.Second
	return &APILogger{
		messages:          make([]model.LogMessage, 0, 100),
		flushLock:         sync.Mutex{},
		appendLock:        sync.Mutex{},
		SendAfterLines:    100,
		SendAfterDuration: sendAfterDuration,
		autoFlushTimer:    time.NewTimer(sendAfterDuration),
		TaskCommunicator:  tc,
	}
}

// Append (to satisfy the Appender interface) adds a log message to the internal
// buffer, and translates the log message into a format that is used by the
// remote endpoint.
func (apiLgr *APILogger) Append(log *slogger.Log) error {
	message := strings.TrimRight(log.Message(), "\r\n \t")

	// MCI-972: ensure message is valid UTF-8
	if !utf8.ValidString(message) {
		message = strconv.QuoteToASCII(message)
	}

	logMessage := &model.LogMessage{
		Timestamp: log.Timestamp,
		Severity:  levelToString(log.Level),
		Type:      log.Prefix,
		Version:   evergreen.LogmessageCurrentVersion,
		Message:   message,
	}

	apiLgr.appendLock.Lock()
	defer apiLgr.appendLock.Unlock()
	apiLgr.messages = append(apiLgr.messages, *logMessage)

	if len(apiLgr.messages) < apiLgr.SendAfterLines ||
		time.Since(apiLgr.lastFlush) < apiLgr.SendAfterDuration {
		return nil
	}

	apiLgr.flushInternal()

	return nil
}

func (apiLgr *APILogger) sendLogs(flushMsgs []model.LogMessage) int {
	apiLgr.flushLock.Lock()
	defer apiLgr.flushLock.Unlock()
	if len(flushMsgs) == 0 {
		return 0
	}
	apiLgr.TaskCommunicator.Log(flushMsgs)
	return len(flushMsgs)
}

func (apiLgr *APILogger) FlushAndWait() int {
	apiLgr.appendLock.Lock()
	defer apiLgr.appendLock.Unlock()

	apiLgr.lastFlush = time.Now()
	if len(apiLgr.messages) == 0 {
		return 0
	}

	err := apiLgr.sendLogs(apiLgr.messages)
	apiLgr.messages = make([]model.LogMessage, 0, apiLgr.SendAfterLines)
	return err
}

// This function assumes that the caller already holds apiLgr.appendLock
func (apiLgr *APILogger) flushInternal() {
	apiLgr.lastFlush = time.Now()

	messagesToSend := make([]model.LogMessage, len(apiLgr.messages))
	copy(messagesToSend, apiLgr.messages)
	apiLgr.messages = make([]model.LogMessage, 0, apiLgr.SendAfterLines)

	go apiLgr.sendLogs(messagesToSend)
}

func (apiLgr *APILogger) Flush() {
	apiLgr.appendLock.Lock()
	defer apiLgr.appendLock.Unlock()

	apiLgr.flushInternal()
}

func levelToString(level slogger.Level) string {
	switch level {
	case slogger.DEBUG:
		return model.LogDebugPrefix
	case slogger.INFO:
		return model.LogInfoPrefix
	case slogger.WARN:
		return model.LogWarnPrefix
	case slogger.ERROR:
		return model.LogErrorPrefix
	}
	return "UNKNOWN"
}

// NewTestLogger creates a logger for testing. This Logger
// stores everything in memory.
func NewTestLogger(appender slogger.Appender) *StreamLogger {
	return &StreamLogger{
		Local: &slogger.Logger{
			Prefix:    "local",
			Appenders: []slogger.Appender{appender},
		},

		System: &slogger.Logger{
			Prefix:    model.SystemLogPrefix,
			Appenders: []slogger.Appender{appender},
		},

		Task: &slogger.Logger{
			Prefix:    model.TaskLogPrefix,
			Appenders: []slogger.Appender{appender},
		},

		Execution: &slogger.Logger{
			Prefix:    model.AgentLogPrefix,
			Appenders: []slogger.Appender{appender},
		},
	}
}
