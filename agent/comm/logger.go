package comm

import (
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/grip/slogger"
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

// GetTaskLogWriter returns an io.Writer of the given level that writes to the task log stream.
func (lgr *StreamLogger) GetTaskLogWriter(level slogger.Level) io.Writer {
	return &evergreen.LoggingWriter{
		Logger:   lgr.Task,
		Severity: level.Priority(),
	}
}

// GetSystemLogWriter returns an io.Writer of the given level that writes to the system log stream.
func (lgr *StreamLogger) GetSystemLogWriter(level slogger.Level) io.Writer {
	return &evergreen.LoggingWriter{
		Logger:   lgr.System,
		Severity: level.Priority(),
	}
}

// appendMessages sends a log message to every component sender in a slogger.Logger
func appendMessages(logger *slogger.Logger, msg message.Composer) {
	for _, sender := range logger.Appenders {
		sender.Send(msg)
	}
}

// LogLocal logs a message to the agent logs on the machine's local file system.
//
// Anything logged by this method will not be sent to the server, so only use it
// to log information that would only be useful when debugging locally.
func (lgr *StreamLogger) LogLocal(level slogger.Level, messageFmt string, args ...interface{}) {
	msg := slogger.NewPrefixedLog(lgr.Local.Name, message.NewFormattedMessage(level.Priority(), messageFmt, args...))
	appendMessages(lgr.Local, msg)
}

// LogExecution logs a message related to the agent's internal workings.
//
// Internally this is used to log things like heartbeats and command internals that
// would pollute the regular task test output.
func (lgr *StreamLogger) LogExecution(level slogger.Level, messageFmt string, args ...interface{}) {
	msg := slogger.NewPrefixedLog(lgr.Execution.Name, message.NewFormattedMessage(level.Priority(), messageFmt, args...))
	appendMessages(lgr.Execution, msg)
}

// LogTask logs a message to the task's logs.
//
// This log type is for main task input and output. LogTask should be used for logging
// first-class information like test results and shell script output.
func (lgr *StreamLogger) LogTask(level slogger.Level, messageFmt string, args ...interface{}) {
	msg := slogger.NewPrefixedLog(lgr.Task.Name, message.NewFormattedMessage(level.Priority(), messageFmt, args...))
	appendMessages(lgr.Task, msg)
}

// LogSystem logs passive system messages.
//
// Internally this is used for periodically logging process information and CPU usage.
func (lgr *StreamLogger) LogSystem(level slogger.Level, messageFmt string, args ...interface{}) {
	msg := slogger.NewPrefixedLog(lgr.System.Name, message.NewFormattedMessage(level.Priority(), messageFmt, args...))
	appendMessages(lgr.System, msg)
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

func NewCommandLogger(name string, logger *StreamLogger) *CommandLogger {
	return &CommandLogger{name, logger}
}

func (cmdLgr *CommandLogger) addCommandToMsgAndArgs(messageFmt string, args []interface{}) (string, []interface{}) {
	return "[%v] " + messageFmt, append([]interface{}{cmdLgr.commandName}, args...)
}

func (cmdLgr *CommandLogger) GetTaskLogWriter(level slogger.Level) io.Writer {
	return cmdLgr.logger.GetTaskLogWriter(level)
}

func (cmdLgr *CommandLogger) GetSystemLogWriter(level slogger.Level) io.Writer {
	return cmdLgr.logger.GetSystemLogWriter(level)
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
func NewStreamLogger(timeoutWatcher *TimeoutWatcher, apiLgr *APILogger) (*StreamLogger, error) {
	defaultLoggers := []send.Sender{slogger.WrapAppender(apiLgr), grip.GetSender()}
	timeoutLogger := slogger.WrapAppender(&TimeoutResetLogger{timeoutWatcher, apiLgr})

	return &StreamLogger{
		Local: &slogger.Logger{
			Name:      "local",
			Appenders: []send.Sender{grip.GetSender()},
		},

		System: &slogger.Logger{
			Name:      model.SystemLogPrefix,
			Appenders: defaultLoggers,
		},

		Task: &slogger.Logger{
			Name:      model.TaskLogPrefix,
			Appenders: []send.Sender{timeoutLogger, grip.GetSender()},
		},

		Execution: &slogger.Logger{
			Name:      model.AgentLogPrefix,
			Appenders: defaultLoggers,
		},
	}, nil
}

// TimeoutResetLogger wraps any slogger.Appender and resets a TimeoutWatcher
// each time any log message is appended to it.
type TimeoutResetLogger struct {
	*TimeoutWatcher
	*APILogger
}

// Append passes the message to the underlying appender, and resets the timeout
func (trLgr *TimeoutResetLogger) Append(log *slogger.Log) error {
	trLgr.TimeoutWatcher.CheckIn()

	return trLgr.APILogger.Append(log)
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
	start := time.Now()
	apiLgr.flushLock.Lock()
	defer apiLgr.flushLock.Unlock()
	if len(flushMsgs) == 0 {
		return 0
	}

	grip.CatchError(apiLgr.TaskCommunicator.Log(flushMsgs))
	grip.Infof("sent %d log messages to api server, in %s",
		len(flushMsgs), time.Since(start))

	return len(flushMsgs)
}

func (apiLgr *APILogger) FlushAndWait() int {
	apiLgr.appendLock.Lock()
	defer apiLgr.appendLock.Unlock()

	apiLgr.lastFlush = time.Now()
	if len(apiLgr.messages) == 0 {
		return 0
	}

	numMessages := apiLgr.sendLogs(apiLgr.messages)
	apiLgr.messages = make([]model.LogMessage, 0, apiLgr.SendAfterLines)

	return numMessages
}

// flushInternal assumes that the caller already holds apiLgr.appendLock
func (apiLgr *APILogger) flushInternal() {
	apiLgr.lastFlush = time.Now()

	messagesToSend := make([]model.LogMessage, len(apiLgr.messages))
	copy(messagesToSend, apiLgr.messages)
	apiLgr.messages = make([]model.LogMessage, 0, apiLgr.SendAfterLines)

	go apiLgr.sendLogs(messagesToSend)
}

// Flush pushes log messages (asynchronously, without waiting for messages to send.)
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
