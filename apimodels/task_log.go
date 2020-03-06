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

func GetBuildloggerLogs(ctx context.Context, buildloggerBaseURL, taskId, logType string, tail, execution int) (io.ReadCloser, error) {
	usr := gimlet.GetUser(ctx)
	fmt.Println(usr.GetAPIKey())
	taskId = "evergreen_lint_lint_service_d7550a92b28636350af9d375b0df7e341477751d_20_03_06_13_55_56"
	execution = 0
	buildloggerBaseURL = "cedar.mongodb.com"
	opts := fetcher.GetOptions{
		BaseURL:       fmt.Sprintf("https://%s", buildloggerBaseURL),
		UserKey:       "1dc6f93f6cde1ea0f4a41b90f36a25af",
		UserName:      "arjun.patel",
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
