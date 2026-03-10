package taskexec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

const (
	logBaseDir       = ".evergreen-local"
	logSubDir        = "logs"
	sessionSubDir    = "session"
	setupSubDir      = "setup"
	taskLogFile      = "task.log"
	executionLogFile = "execution.log"
	systemLogFile    = "system.log"
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
func (lf *logFile) WriteLogLine(step int, msg string) {
	lf.mu.Lock()
	defer lf.mu.Unlock()

	if lf.file == nil {
		return
	}

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	line := fmt.Sprintf("[%s] [step:%d] %s\n", ts, step, msg)
	_, _ = lf.file.WriteString(line)
}

// WriteStepStart writes a step start delimiter.
func (lf *logFile) WriteStepStart(step int, displayName string, blockType string) {
	lf.mu.Lock()
	defer lf.mu.Unlock()

	if lf.file == nil {
		return
	}

	line := fmt.Sprintf("=== STEP %d START %s (%s) ===\n", step, displayName, blockType)
	_, _ = lf.file.WriteString(line)
}

// WriteStepEnd writes a step end delimiter.
func (lf *logFile) WriteStepEnd(step int, success bool, duration string) {
	lf.mu.Lock()
	defer lf.mu.Unlock()

	if lf.file == nil {
		return
	}

	line := fmt.Sprintf("=== STEP %d END success=%t duration=%s ===\n", step, success, duration)
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

// logManager manages the set of log files for a debug session.
type logManager struct {
	taskLog      *logFile
	execLog      *logFile
	systemLog    *logFile
	logDir       string
	isSetupPhase bool
}

// newLogManager creates a log manager for either session or setup logs.
func newLogManager(isSetupPhase bool) (*logManager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.Wrap(err, "getting user home directory")
	}

	subDir := sessionSubDir
	if isSetupPhase {
		subDir = setupSubDir
	}
	logDir := filepath.Join(homeDir, logBaseDir, logSubDir, subDir)

	taskLog, err := newLogFile(filepath.Join(logDir, taskLogFile))
	if err != nil {
		return nil, errors.Wrap(err, "creating task log file")
	}

	execLog, err := newLogFile(filepath.Join(logDir, executionLogFile))
	if err != nil {
		taskLog.Close()
		return nil, errors.Wrap(err, "creating execution log file")
	}

	systemLog, err := newLogFile(filepath.Join(logDir, systemLogFile))
	if err != nil {
		taskLog.Close()
		execLog.Close()
		return nil, errors.Wrap(err, "creating system log file")
	}

	return &logManager{
		taskLog:      taskLog,
		execLog:      execLog,
		systemLog:    systemLog,
		logDir:       logDir,
		isSetupPhase: isSetupPhase,
	}, nil
}

// LogFileHandle is an exported wrapper around logFile for cross-package use.
type LogFileHandle struct {
	inner *logFile
}

// TaskLogHandle returns an exported handle to the task log file.
func (lm *logManager) TaskLogHandle() *LogFileHandle {
	if lm.taskLog == nil {
		return nil
	}
	return &LogFileHandle{inner: lm.taskLog}
}

// LogForChannel returns the log file for the given channel.
func (lm *logManager) LogForChannel(ch StreamChannel) *logFile {
	switch ch {
	case TaskChannel:
		return lm.taskLog
	case ExecChannel:
		return lm.execLog
	default:
		return lm.taskLog
	}
}

// Close closes all log files.
func (lm *logManager) Close() error {
	errs := []error{}
	if err := lm.taskLog.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := lm.execLog.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := lm.systemLog.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// ClearSessionLogs removes session log files (called when selecting a new task).
func ClearSessionLogs() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return errors.Wrap(err, "getting user home directory")
	}
	logDir := filepath.Join(homeDir, logBaseDir, logSubDir, sessionSubDir)
	if err := os.RemoveAll(logDir); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "clearing session logs")
	}
	return nil
}

type LogFilterOptions struct {
	Step    int
	HasStep bool
	Tail    int
}

// ReadFilteredLogs reads log files from the given directory with optional filtering.
func ReadFilteredLogs(isSetup bool, channel string, opts LogFilterOptions) ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.Wrap(err, "getting user home directory")
	}

	subDir := sessionSubDir
	if isSetup {
		subDir = setupSubDir
	}
	logDir := filepath.Join(homeDir, logBaseDir, logSubDir, subDir)

	var filenames []string
	switch channel {
	case "task":
		filenames = []string{taskLogFile}
	case "exec", "execution":
		filenames = []string{executionLogFile}
	case "system":
		filenames = []string{systemLogFile}
	default:
		filenames = []string{taskLogFile, executionLogFile, systemLogFile}
	}

	var allLines []string
	for _, filename := range filenames {
		path := filepath.Join(logDir, filename)
		lines, err := readLogFileLines(path, opts)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, errors.Wrapf(err, "reading log file '%s'", path)
		}
		allLines = append(allLines, lines...)
	}

	if opts.Tail > 0 && len(allLines) > opts.Tail {
		allLines = allLines[len(allLines)-opts.Tail:]
	}

	return allLines, nil
}

func readLogFileLines(path string, opts LogFilterOptions) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	inTargetStep := !opts.HasStep

	for scanner.Scan() {
		line := scanner.Text()

		if opts.HasStep {
			if strings.HasPrefix(line, fmt.Sprintf("=== STEP %d START", opts.Step)) {
				inTargetStep = true
			} else if strings.HasPrefix(line, "=== STEP") && strings.Contains(line, "START") && !strings.HasPrefix(line, fmt.Sprintf("=== STEP %d ", opts.Step)) {
				inTargetStep = false
			} else if strings.HasPrefix(line, fmt.Sprintf("=== STEP %d END", opts.Step)) {
				lines = append(lines, line)
				inTargetStep = false
				continue
			}
		}

		if inTargetStep {
			lines = append(lines, line)
		}
	}

	return lines, scanner.Err()
}
