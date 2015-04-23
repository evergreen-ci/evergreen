package agent

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"10gen.com/mci/util"
	"bytes"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

//LineBufferedLogWriter is an implementation of io.Writer that sends the newline
//delimited subslices of its inputs to a wrapped io.Writer, buffering as needed.
type LineBufferingWriter struct {
	buf           []byte
	WrappedWriter io.Writer
	bufferLock    sync.Mutex
}

func NewLineBufferingWriter(wrapped io.Writer) *LineBufferingWriter {
	return &LineBufferingWriter{
		buf:           []byte{},
		WrappedWriter: wrapped,
	}
}

// writes whatever is in the buffer out using the wrapped io.Writer
func (lbw *LineBufferingWriter) Flush() error {
	lbw.bufferLock.Lock()
	defer lbw.bufferLock.Unlock()
	if len(lbw.buf) == 0 {
		return nil
	}

	_, err := lbw.WrappedWriter.Write(lbw.buf)
	if err != nil {
		return err
	}
	lbw.buf = []byte{}
	return nil
}

// use the wrapped io.Writer to write anything that is delimited with a newline
func (lbw *LineBufferingWriter) Write(p []byte) (n int, err error) {
	lbw.bufferLock.Lock()
	defer lbw.bufferLock.Unlock()

	// Check to see if sum of two buffers is greater than 4K bytes
	if len(p)+len(lbw.buf) >= (4*1024) && len(lbw.buf) > 0 {
		_, err := lbw.WrappedWriter.Write(lbw.buf)
		if err != nil {
			return 0, err
		}
		lbw.buf = []byte{}
	}

	fullBuffer := append(lbw.buf, p...)
	lines := bytes.Split(fullBuffer, []byte{'\n'})
	for idx, val := range lines {
		if idx == len(lines)-1 {
			lbw.buf = val
		} else {
			_, err := lbw.WrappedWriter.Write(val)
			if err != nil {
				return 0, err
			}
		}
	}

	return len(p), nil
}

type AgentLogger struct {
	LocalLogger    *slogger.Logger
	SystemLogger   *slogger.Logger
	TaskLogger     *slogger.Logger
	ExecLogger     *slogger.Logger
	remoteAppender TaskCommunicatorAppender
}

func (self *AgentLogger) GetTaskLogWriter(level slogger.Level) io.Writer {
	return &mci.LoggingWriter{
		Logger:   self.TaskLogger,
		Severity: level,
	}
}

func (self *AgentLogger) LogLocal(level slogger.Level, messageFmt string, args ...interface{}) {
	self.LocalLogger.Logf(level, messageFmt, args...)
}

func (self *AgentLogger) LogExecution(level slogger.Level, messageFmt string, args ...interface{}) {
	self.ExecLogger.Logf(level, messageFmt, args...)
}

func (self *AgentLogger) LogTask(level slogger.Level, messageFmt string, args ...interface{}) {
	self.TaskLogger.Logf(level, messageFmt, args...)
}

func (self *AgentLogger) LogSystem(level slogger.Level, messageFmt string, args ...interface{}) {
	self.SystemLogger.Logf(level, messageFmt, args...)
}

func (self *AgentLogger) Flush() {
	self.remoteAppender.Flush()
}

func (self *AgentLogger) FlushAndWait() int {
	return self.remoteAppender.FlushAndWait()
}

// Wraps an AgentLogger, with additional context about which command is
// currently being run
type AgentCommandLogger struct {
	commandName string
	agentLogger *AgentLogger
}

func (self *AgentCommandLogger) addCommandToMsgAndArgs(messageFmt string,
	args []interface{}) (string, []interface{}) {
	return "[%v] " + messageFmt, append([]interface{}{self.commandName}, args...)
}

func (self *AgentCommandLogger) GetTaskLogWriter(level slogger.Level) io.Writer {
	return self.agentLogger.GetTaskLogWriter(level)
}

func (self *AgentCommandLogger) LogLocal(level slogger.Level,
	messageFmt string, args ...interface{}) {
	messageFmt, args = self.addCommandToMsgAndArgs(messageFmt, args)
	self.agentLogger.LogLocal(level, messageFmt, args...)
}

func (self *AgentCommandLogger) LogExecution(level slogger.Level,
	messageFmt string, args ...interface{}) {
	messageFmt, args = self.addCommandToMsgAndArgs(messageFmt, args)
	self.agentLogger.LogExecution(level, messageFmt, args...)
}

func (self *AgentCommandLogger) LogTask(level slogger.Level,
	messageFmt string, args ...interface{}) {
	messageFmt, args = self.addCommandToMsgAndArgs(messageFmt, args)
	self.agentLogger.LogTask(level, messageFmt, args...)
}

func (self *AgentCommandLogger) LogSystem(level slogger.Level,
	messageFmt string, args ...interface{}) {
	messageFmt, args = self.addCommandToMsgAndArgs(messageFmt, args)
	self.agentLogger.LogSystem(level, messageFmt, args...)
}

func (self *AgentCommandLogger) Flush() {
	self.agentLogger.Flush()
}

func initLocalLogger() (slogger.Appender, error) {
	mciHome := mci.RemoteShell
	logDir := filepath.Join(mciHome, "logs")
	exists, err := util.FileExists(logDir)
	if err != nil {
		return nil, err
	}

	if !exists {
		cmd := exec.Command("mkdir", "-p", logDir)
		cmd.Env = os.Environ()
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("cannot create path %v: %v", logDir, err)
		}
	}

	logFilePath := filepath.Join(logDir, "agent.log")
	logFile, err := util.GetAppendingFile(logFilePath)
	if err != nil {
		return nil, err
	}
	return &slogger.FileAppender{logFile}, nil
}

func NewAgentLogger(idleWatcher *TimeoutWatcher, remoteAppender *TaskCommunicatorAppender, useStdout bool) (*AgentLogger, error) {
	localAppender, err := initLocalLogger()
	if err != nil {
		return nil, err
	}

	localAppenders := []slogger.Appender{localAppender}
	defaultAppenders := []slogger.Appender{localAppender, remoteAppender}
	if useStdout {
		defaultAppenders = append(defaultAppenders, slogger.StdOutAppender())
		localAppenders = append(localAppenders, slogger.StdOutAppender())
	}

	timeoutResetAppender := &TimeoutResetAppender{
		idleWatcher,
		remoteAppender,
	}
	return &AgentLogger{
		LocalLogger: &slogger.Logger{
			Prefix:    "local",
			Appenders: localAppenders,
		},

		SystemLogger: &slogger.Logger{
			Prefix:    model.SystemLogPrefix,
			Appenders: defaultAppenders,
		},

		TaskLogger: &slogger.Logger{
			Prefix:    model.TaskLogPrefix,
			Appenders: []slogger.Appender{localAppender, timeoutResetAppender},
		},

		ExecLogger: &slogger.Logger{
			Prefix:    model.AgentLogPrefix,
			Appenders: defaultAppenders,
		},
	}, nil
}

//IdleWatchingAppender wraps any slogger.Appender and resets a TimeoutWatcher
//each time any log message is appended to it.
type TimeoutResetAppender struct {
	*TimeoutWatcher
	slogger.Appender
}

//Append passes the message to the underlying appender, and resets the timeout
func (self *TimeoutResetAppender) Append(log *slogger.Log) error {
	self.TimeoutWatcher.CheckIn()
	return self.Appender.Append(log)
}

//TaskCommunicatorAppender is a slogger.Appender which makes a call to the
//remote service's log endpoint after SendAfterLines messages have been
//received (or if set, after SendAfterDuration time has passed with no flush).
type TaskCommunicatorAppender struct {
	//An internal buffer of messages to send.
	messages []model.LogMessage

	//a mutex to ensure only one flush attempt is in progress at a time.
	flushLock sync.Mutex

	//mutex to protect appends to the buffer so that messages don't get lost
	//if both a flush/append happen concurrently
	appendLock sync.Mutex

	//last time flush actually flushed lines
	lastFlush time.Time

	//The number of log lines that the buffer must reach to trigger a flush
	SendAfterLines int

	//How long to wait without any flushes before triggering one automatically
	SendAfterDuration time.Duration

	//Timer to trigger autoflushing based on SendAfterDuration.
	//When set to nil, flushing will only happen when called directly
	//or the SendAfterLines threshold is reached.
	autoFlushTimer *time.Timer

	//Channel on which to send signals that must be propagated back to agent;
	//for instance, if the secret doesn't match what the server expects,
	//it must send IncorrectSecret on the channel.
	signalChan chan AgentSignal

	//The mechanism for communicating with the remote endpoint.
	TaskCommunicator
}

func NewTaskCommunicatorAppender(tc TaskCommunicator, signalChan chan AgentSignal) *TaskCommunicatorAppender {
	sendAfterDuration := 5 * time.Second
	return &TaskCommunicatorAppender{
		messages:          make([]model.LogMessage, 0, 100),
		flushLock:         sync.Mutex{},
		appendLock:        sync.Mutex{},
		SendAfterLines:    100,
		SendAfterDuration: sendAfterDuration,
		autoFlushTimer:    time.NewTimer(sendAfterDuration),
		TaskCommunicator:  tc,
	}
}

//Append (to satisfy the Appender interface) adds a log message to the internal
//buffer, and translates the log message into a format that is used by the
//remote endpoint.
func (self *TaskCommunicatorAppender) Append(log *slogger.Log) error {
	message := strings.TrimRight(log.Message(), "\r\n \t")

	// MCI-972: ensure message is valid UTF-8
	if !utf8.ValidString(message) {
		message = strconv.QuoteToASCII(message)
	}

	logMessage := &model.LogMessage{
		Timestamp: log.Timestamp,
		Severity:  levelToString(log.Level),
		Type:      log.Prefix,
		Version:   mci.LogmessageCurrentVersion,
		Message:   message,
	}

	self.appendLock.Lock()
	defer self.appendLock.Unlock()
	self.messages = append(self.messages, *logMessage)

	if len(self.messages) < self.SendAfterLines ||
		time.Since(self.lastFlush) < self.SendAfterDuration {
		return nil
	}

	self.flushInternal()

	return nil
}

func (self *TaskCommunicatorAppender) sendLogs(flushMsgs []model.LogMessage) int {
	self.flushLock.Lock()
	defer self.flushLock.Unlock()
	if len(flushMsgs) == 0 {
		return 0
	}
	self.TaskCommunicator.Log(flushMsgs)
	return len(flushMsgs)
}

func (self *TaskCommunicatorAppender) FlushAndWait() int {
	self.appendLock.Lock()
	defer self.appendLock.Unlock()

	self.lastFlush = time.Now()
	if len(self.messages) == 0 {
		return 0
	}

	err := self.sendLogs(self.messages)
	self.messages = make([]model.LogMessage, 0, self.SendAfterLines)
	return err
}

// This function assumes that the caller already holds self.appendLock
func (self *TaskCommunicatorAppender) flushInternal() {
	self.lastFlush = time.Now()

	messagesToSend := make([]model.LogMessage, len(self.messages))
	copy(messagesToSend, self.messages)
	self.messages = make([]model.LogMessage, 0, self.SendAfterLines)

	go self.sendLogs(messagesToSend)
}

func (self *TaskCommunicatorAppender) Flush() {
	self.appendLock.Lock()
	defer self.appendLock.Unlock()

	self.flushInternal()
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

func NewTestAgentLogger(appender slogger.Appender) *AgentLogger {
	return &AgentLogger{
		LocalLogger: &slogger.Logger{
			Prefix:    "local",
			Appenders: []slogger.Appender{appender},
		},

		SystemLogger: &slogger.Logger{
			Prefix:    model.SystemLogPrefix,
			Appenders: []slogger.Appender{appender},
		},

		TaskLogger: &slogger.Logger{
			Prefix:    model.TaskLogPrefix,
			Appenders: []slogger.Appender{appender},
		},

		ExecLogger: &slogger.Logger{
			Prefix:    model.AgentLogPrefix,
			Appenders: []slogger.Appender{appender},
		},
	}
}
