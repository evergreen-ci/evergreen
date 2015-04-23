package gologger

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	TIME_FORMAT = "2006-01-02T15:04:05.000"
	DEBUG       = iota
	INFO
	WARN
	ERROR
)

var (
	LogColors = map[int]int{
		DEBUG: 102,
		INFO:  28,
		WARN:  214,
		ERROR: 196,
	}
	LogPrefixes = map[int]string{
		DEBUG: "DEBUG",
		INFO:  "INFO ",
		WARN:  "WARN ",
		ERROR: "ERROR",
	}
	logger *Logger
)

func Colorize(c int, s string) (r string) {
	return fmt.Sprintf("\033[38;5;%dm%s\033[0m", c, s)
}

type Logger struct {
	LogLevel int
	Started  time.Time
	Prefix   string
	prefixes []string
	Colored  bool
	Caller   bool
}

func currentLogger() *Logger {
	if logger == nil {
		logger = New()
		if os.Getenv("DEBUG") == "true" {
			logger.Caller = true
			logger.LogLevel = DEBUG
		}
	}
	return logger
}

func New() *Logger {
	return &Logger{Colored: true, LogLevel: INFO}
}

func NewFromEnv() *Logger {
	logger = New()
	if os.Getenv("DEBUG") == "true" {
		logger.Caller = true
		logger.LogLevel = DEBUG
	}
	return logger
}

func DeferBenchmark() func() {
	return currentLogger().DeferBenchmark()
}

func (logger *Logger) DeferBenchmark() func() {
	logger.Start()
	return func() { logger.Stop() }
}

func DeferRestoreLogLevel(level int) func() {
	return currentLogger().DeferRestoreLogLevel(level)
}

func (logger *Logger) DeferRestoreLogLevel(level int) func() {
	oldLevel := logger.LogLevel
	logger.LogLevel = level
	return func() { logger.LogLevel = oldLevel }
}

func DeferRestoreLogPrefix(newPrefix string) func() {
	return currentLogger().DeferRestoreLogPrefix(newPrefix)
}

func (logger *Logger) DeferRestoreLogPrefix(newPrefix string) func() {
	oldPrefix := logger.Prefix
	logger.Prefix = newPrefix
	return func() { logger.Prefix = oldPrefix }
}

func Start() {
	currentLogger().Start()
}

func (self *Logger) Start() {
	self.Started = time.Now()
}

func Stop() {
	currentLogger().Stop()
}

func (self *Logger) Stop() {
	self.Started = time.Unix(0, 0)
}

func Inspect(i interface{}) {
	currentLogger().Info(i)
}

func (self *Logger) Inspect(i interface{}) {
	self.Debugf("%+v", i)
}

func Debugf(format string, n ...interface{}) {
	currentLogger().Debugf(format, n...)
}

func (l *Logger) Debugf(format string, n ...interface{}) {
	l.logf(DEBUG, format, n...)
}

func Infof(format string, n ...interface{}) {
	currentLogger().Infof(format, n...)
}

func (l *Logger) Infof(format string, n ...interface{}) {
	l.logf(INFO, format, n...)
}

func Warnf(format string, n ...interface{}) {
	currentLogger().Warnf(format, n...)
}

func (l *Logger) Warnf(format string, n ...interface{}) {
	l.logf(WARN, format, n...)
}

func Errorf(format string, n ...interface{}) {
	currentLogger().Errorf(format, n...)
}

func (l *Logger) Errorf(format string, n ...interface{}) {
	l.logf(ERROR, format, n...)
}

func Debug(n ...interface{}) {
	currentLogger().Debug(n...)
}

func (l *Logger) Debug(n ...interface{}) {
	l.log(DEBUG, n...)
}

func Info(n ...interface{}) {
	currentLogger().Info(n...)
}

func (l *Logger) Info(n ...interface{}) {
	l.log(INFO, n...)
}

func Warn(n ...interface{}) {
	currentLogger().Warn(n...)
}

func (l *Logger) Warn(n ...interface{}) {
	l.log(WARN, n...)
}

func Error(n ...interface{}) {
	currentLogger().Error(n...)
}

func (l *Logger) Error(n ...interface{}) {
	l.log(ERROR, n...)
}

func (l *Logger) logf(level int, s string, n ...interface{}) {
	if level >= l.LogLevel {
		l.write(l.logPrefix(level), fmt.Sprintf(s, n...))
	}
}

func (l *Logger) log(level int, n ...interface{}) {
	if level >= l.LogLevel {
		all := append([]interface{}{l.logPrefix(level)}, n...)
		l.write(all...)
	}
}

func (l *Logger) PushPrefix(prefix string) {
	l.prefixes = append(l.prefixes, prefix)
}

func (l *Logger) PopPrefix() string {
	popped := ""
	pLen := len(l.prefixes)
	if pLen > 0 {
		popped = l.prefixes[pLen-1]
		l.prefixes = l.prefixes[:pLen-1]
	}
	return popped
}

func (l *Logger) logPrefix(i int) (s string) {
	s = time.Now().Format(TIME_FORMAT)
	if l.Started.Unix() > 0 {
		time := fmt.Sprintf("%.3f", time.Now().Sub(l.Started).Seconds())
		s += fmt.Sprintf(" [%8s]", time)
	}
	s = s + " " + l.LogLevelPrefix(i) + " "
	if l.Prefix != "" {
		s = s + "[" + l.Prefix + "]"
		if len(l.prefixes) == 0 {
			s += " "
		}
	}
	if len(l.prefixes) > 0 {
		for i := range l.prefixes {
			s = s + "[" + l.prefixes[i] + "]"
		}
		s += " "
	}
	if l.Caller {
		_, file, line, ok := runtime.Caller(3)
		if ok {
			s += fmt.Sprintf("[%s:%d] ", filepath.Base(file), line)
		}
	}
	return s
}

func (l *Logger) LogLevelPrefix(level int) (s string) {
	prefix := LogPrefixes[level]
	if l.Colored {
		color := LogColors[level]
		return Colorize(color, prefix)
	}
	return prefix
}

func (self *Logger) write(n ...interface{}) {
	fmt.Fprintln(os.Stderr, n...)
}

func (logger *Logger) Write(p []byte) (n int, e error) {
	q := p
	n = len(q)
	logger.Infof(string(q))
	return n, e
}
