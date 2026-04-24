package taskexec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

const (
	logBaseDir    = ".evergreen-local"
	logSubDir     = "logs"
	sessionSubDir = "session"
	setupSubDir   = "setup"
	outputLogFile = "output.log"

	setupLogPath = "/tmp/debug-setup.log"
)

// logFile manages writing log lines to a local file.
type logFile struct {
	mu   sync.Mutex
	file *os.File
}

// newLogFile creates or opens a log file at the given path.
func newLogFile(path string) (*logFile, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, errors.Wrapf(err, "creating log directory '%s'", dir)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, errors.Wrapf(err, "opening log file '%s'", path)
	}

	return &logFile{file: f}, nil
}

// WriteLogLine writes a timestamped, step-annotated line to the log file.
func (lf *logFile) WriteLogLine(step string, msg string) {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	lf.writeLogLine(step, msg)
}

func (lf *logFile) writeLogLine(step string, msg string) {
	if lf.file == nil {
		return
	}

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	line := fmt.Sprintf("[%s] [step:%s] %s\n", ts, step, msg)
	_, _ = lf.file.WriteString(line)
}

// WriteStepStart writes a step start delimiter.
func (lf *logFile) WriteStepStart(step string, displayName string, blockType string) {
	lf.mu.Lock()
	defer lf.mu.Unlock()

	if lf.file == nil {
		return
	}

	line := fmt.Sprintf("=== STEP %s START %s (%s) ===\n", step, displayName, blockType)
	_, _ = lf.file.WriteString(line)
}

// WriteStepEnd writes a step end delimiter.
func (lf *logFile) WriteStepEnd(step string, success bool, duration string) {
	lf.mu.Lock()
	defer lf.mu.Unlock()

	if lf.file == nil {
		return
	}

	line := fmt.Sprintf("=== STEP %s END success=%t duration=%s ===\n", step, success, duration)
	_, _ = lf.file.WriteString(line)
}

// Close closes the log file.
func (lf *logFile) Close() error {
	lf.mu.Lock()
	defer lf.mu.Unlock()

	if lf.file == nil {
		return nil
	}
	return lf.file.Close()
}

// logManager manages the log file for a debug session.
type logManager struct {
	outputLog    *logFile
	logDir       string
	isSetupPhase bool
}

// newLogManager creates a log manager for either session or setup logs.
func newLogManager(isSetupPhase bool) (*logManager, error) {
	homeDir, err := util.GetUserHome()
	if err != nil {
		return nil, errors.Wrap(err, "getting user home directory")
	}

	subDir := sessionSubDir
	if isSetupPhase {
		subDir = setupSubDir
	}
	logDir := filepath.Join(homeDir, logBaseDir, logSubDir, subDir)

	outputLog, err := newLogFile(filepath.Join(logDir, outputLogFile))
	if err != nil {
		return nil, errors.Wrap(err, "creating output log file")
	}

	return &logManager{
		outputLog:    outputLog,
		logDir:       logDir,
		isSetupPhase: isSetupPhase,
	}, nil
}

// LogFileHandle is an exported wrapper around logFile for cross-package use.
type LogFileHandle struct {
	inner *logFile
}

// LogHandle returns an exported handle to the output log file.
func (lm *logManager) LogHandle() *LogFileHandle {
	if lm.outputLog == nil {
		return nil
	}
	return &LogFileHandle{inner: lm.outputLog}
}

// LogFile returns the output log file.
func (lm *logManager) LogFile() *logFile {
	return lm.outputLog
}

// Close closes the log file.
func (lm *logManager) Close() error {
	return lm.outputLog.Close()
}

// ClearSessionLogs removes session log files (called when selecting a new task).
func ClearSessionLogs() error {
	homeDir, err := util.GetUserHome()
	if err != nil {
		return errors.Wrap(err, "getting user home directory")
	}
	logDir := filepath.Join(homeDir, logBaseDir, logSubDir, sessionSubDir)
	if err := os.RemoveAll(logDir); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "clearing session logs")
	}
	return nil
}

// ReadAllLogs reads all lines from the output log file. When isSetup is true,
// it reads from the setup script log at /tmp/debug-setup.log.
func ReadAllLogs(isSetup bool) ([]string, error) {
	var path string
	if isSetup {
		path = setupLogPath
	} else {
		homeDir, err := util.GetUserHome()
		if err != nil {
			return nil, errors.Wrap(err, "getting user home directory")
		}
		path = filepath.Join(homeDir, logBaseDir, logSubDir, sessionSubDir, outputLogFile)
	}

	lines, err := readLogFileLines(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "reading log file '%s'", path)
	}

	return lines, nil
}

func readLogFileLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	return lines, scanner.Err()
}

// FilterLogLinesByStep returns only log lines that belong to the given step.
// It matches lines containing "[step:S]" or "=== STEP S " markers.
func FilterLogLinesByStep(lines []string, step string) []string {
	stepTag := fmt.Sprintf("[step:%s]", step)
	stepDelimiter := fmt.Sprintf("=== STEP %s ", step)

	var filtered []string
	for _, line := range lines {
		if strings.Contains(line, stepTag) || strings.Contains(line, stepDelimiter) {
			filtered = append(filtered, line)
		}
	}
	return filtered
}
