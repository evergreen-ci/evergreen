package apimodels

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/timber/fetcher"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// for the different types of remote logging
const (
	SystemLogPrefix = "S"
	AgentLogPrefix  = "E"
	TaskLogPrefix   = "T"

	LogErrorPrefix = "E"
	LogWarnPrefix  = "W"
	LogDebugPrefix = "D"
	LogInfoPrefix  = "I"
)

// Also used in the task_logg collection in the database.
// The LogMessage type is used by the models package and is stored in
// the database (inside in the model.TaskLog structure.)
type LogMessage struct {
	Type      string    `bson:"t" json:"t"`
	Severity  string    `bson:"s" json:"s"`
	Message   string    `bson:"m" json:"m"`
	Timestamp time.Time `bson:"ts" json:"ts"`
	Version   int       `bson:"v" json:"v"`
}

// TaskLog is a group of LogMessages, and mirrors the model.TaskLog
// type, sans the ObjectID field.
type TaskLog struct {
	TaskId       string       `json:"t_id"`
	Execution    int          `json:"e"`
	Timestamp    time.Time    `json:"ts"`
	MessageCount int          `json:"c"`
	Messages     []LogMessage `json:"m"`
}

func GetSeverityMapping(s int) string {
	switch {
	case s >= int(level.Error):
		return LogErrorPrefix
	case s >= int(level.Warning):
		return LogWarnPrefix
	case s >= int(level.Info):
		return LogInfoPrefix
	case s < int(level.Info):
		return LogDebugPrefix
	default:
		return LogInfoPrefix
	}
}

// GetBuildloggerLogs makes request to cedar for a specifc log and returns a ReadCloser
func GetBuildloggerLogs(ctx context.Context, buildloggerBaseURL, taskId, logType string, tail, execution int) (io.ReadCloser, error) {
	usr := gimlet.GetUser(ctx)
	opts := fetcher.GetOptions{
		BaseURL:       fmt.Sprintf("https://%s", buildloggerBaseURL),
		UserKey:       usr.GetAPIKey(),
		UserName:      usr.Username(),
		TaskID:        taskId,
		Execution:     execution,
		PrintTime:     true,
		PrintPriority: true,
		Tail:          tail,
	}
	switch logType {
	case TaskLogPrefix:
		opts.ProcessName = evergreen.LogTypeTask
	case SystemLogPrefix:
		opts.ProcessName = evergreen.LogTypeSystem
	case AgentLogPrefix:
		opts.ProcessName = evergreen.LogTypeAgent
	}

	logReader, err := fetcher.Logs(ctx, opts)
	return logReader, errors.Wrapf(err, "failed to get logs for '%s' from buildlogger, using evergreen logger", taskId)
}

// ReadBuildloggerToChan parses cedar log lines by message and severity and reads into channel
func ReadBuildloggerToChan(ctx context.Context, taskID string, r io.ReadCloser, lines chan<- LogMessage) {
	var (
		line string
		err  error
	)

	defer close(lines)
	if r == nil {
		return
	}

	reader := bufio.NewReader(r)
	for err == nil {
		line, err = reader.ReadString('\n')
		if err != nil && err != io.EOF {
			grip.Warning(message.WrapError(err, message.Fields{
				"task_id": taskID,
				"message": "problem reading buildlogger log lines",
			}))
			return
		}

		severity := int(level.Info)
		if strings.HasPrefix(line, "[P: ") {
			severity, err = strconv.Atoi(strings.TrimSpace(line[3:6]))
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"task_id": taskID,
					"message": "problem reading buildlogger log line severity",
				}))
				err = nil
			}
		}

		select {
		case <-ctx.Done():
			grip.Error(message.WrapError(ctx.Err(), message.Fields{
				"task_id": taskID,
				"message": "context error while reading buildlogger log lines",
			}))
		case lines <- LogMessage{
			Message:  strings.TrimSuffix(line, "\n"),
			Severity: GetSeverityMapping(severity),
		}:
		}
	}
}

// ReadBuildloggerToSlice returns a slice of LogMessages from a ReadCloser
func ReadBuildloggerToSlice(ctx context.Context, taskID string, r io.ReadCloser) []LogMessage {
	lines := []LogMessage{}
	lineChan := make(chan LogMessage, 1024)
	go ReadBuildloggerToChan(ctx, taskID, r, lineChan)

	for {
		line, more := <-lineChan
		if !more {
			break
		}

		lines = append(lines, line)
	}

	return lines
}
