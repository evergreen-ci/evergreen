package apimodels

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/timber/fetcher"
	"github.com/mongodb/grip/level"
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

// BuildloggerLogLine represents a cedar buildlogger log line, with the
// severity extracted.
type BuildloggerLogLine struct {
	Message  string `bson:"m" json:"m"`
	Severity string `bson:"s" json:"s"`
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
