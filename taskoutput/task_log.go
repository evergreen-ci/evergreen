package taskoutput

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/model/tasklog"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// TaskLogType represents the recognized types of task logs.
type TaskLogType string

const (
	TaskLogTypeAll    TaskLogType = "all_logs"
	TaskLogTypeAgent  TaskLogType = "agent_log"
	TaskLogTypeSystem TaskLogType = "system_log"
	TaskLogTypeTask   TaskLogType = "task_log"
)

func (t TaskLogType) validate() error {
	switch t {
	case TaskLogTypeAll, TaskLogTypeAgent, TaskLogTypeSystem, TaskLogTypeTask:
		return nil
	default:
		return errors.Errorf("unrecognized task log type '%s'", t)
	}
}

// TaskLogOutput is the versioned entry point for coordinating persistent
// storage of a task run's task log data.
type TaskLogOutput struct {
	Version      int                    `bson:"version" json:"version"`
	BucketConfig evergreen.BucketConfig `bson:"bucket_config" json:"bucket_config"`
}

// ID returns the unique identifier of the task log output type.
// Note that this is distinct from the task log output type subtype `task_log`.
func (TaskLogOutput) ID() string { return "task_logs" }

// TaskLogGetOptions represents the arguments for fetching task logs belonging
// to a task run.
type TaskLogGetOptions struct {
	// LogType is the type of task log to fetch.
	LogType TaskLogType
	// Start is the start time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Optional.
	Start int64
	// End is the end time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Optional.
	End int64
	// LineLimit limits the number of lines read from the log. Optional.
	LineLimit int
	// TailN is the number of lines to read from the tail of the log.
	// Optional.
	TailN int
}

// Get returns task logs belonging to the specified task run.
func (o TaskLogOutput) Get(ctx context.Context, env evergreen.Environment, taskOpts TaskOptions, getOpts TaskLogGetOptions) (log.LogIterator, error) {
	if err := getOpts.LogType.validate(); err != nil {
		return nil, err
	}

	if o.Version == 0 {
		return o.getBuildloggerLogs(ctx, env, taskOpts, getOpts)
	}

	svc, err := o.getLogService(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting log service")
	}

	return svc.Get(ctx, log.GetOptions{
		LogNames:  []string{o.getLogName(taskOpts, getOpts.LogType)},
		Start:     getOpts.Start,
		End:       getOpts.End,
		LineLimit: getOpts.LineLimit,
		TailN:     getOpts.TailN,
	})
}

func (o TaskLogOutput) getLogName(taskOpts TaskOptions, logType TaskLogType) string {
	prefix := fmt.Sprintf("%s/%s/%d/%s", taskOpts.ProjectID, taskOpts.TaskID, taskOpts.Execution, o.ID())

	var logTypePrefix string
	switch logType {
	case TaskLogTypeAgent:
		logTypePrefix = "agent"
	case TaskLogTypeSystem:
		logTypePrefix = "system"
	case TaskLogTypeTask:
		logTypePrefix = "task"
	default:
		return prefix
	}

	return fmt.Sprintf("%s/%s", prefix, logTypePrefix)
}

func (o TaskLogOutput) getLogService(ctx context.Context) (log.LogService, error) {
	b, err := newBucket(ctx, o.BucketConfig)
	if err != nil {
		return nil, err
	}

	return log.NewLogServiceV0(b), nil
}

// getBuildloggerLogs makes request to Cedar Buildlogger for logs.
func (o TaskLogOutput) getBuildloggerLogs(ctx context.Context, env evergreen.Environment, taskOpts TaskOptions, getOpts TaskLogGetOptions) (log.LogIterator, error) {
	opts := apimodels.GetBuildloggerLogsOptions{
		BaseURL:   env.Settings().Cedar.BaseURL,
		TaskID:    taskOpts.TaskID,
		Execution: utility.ToIntPtr(taskOpts.Execution),
		Start:     getOpts.Start,
		End:       getOpts.End,
		Limit:     getOpts.LineLimit,
		Tail:      getOpts.TailN,
	}
	if getOpts.LogType == TaskLogTypeAll {
		opts.Tags = []string{
			string(TaskLogTypeAgent),
			string(TaskLogTypeSystem),
			string(TaskLogTypeTask),
		}
	} else {
		opts.Tags = []string{string(getOpts.LogType)}
	}
	it, err := apimodels.GetBuildloggerLogs(ctx, opts)
	if err != nil {
		var logTypeFilter []string
		switch getOpts.LogType {
		case TaskLogTypeAgent:
			logTypeFilter = []string{apimodels.AgentLogPrefix}
		case TaskLogTypeTask:
			logTypeFilter = []string{apimodels.TaskLogPrefix}
		case TaskLogTypeSystem:
			logTypeFilter = []string{apimodels.SystemLogPrefix}
		}

		var messages []apimodels.LogMessage
		if getOpts.TailN > 0 {
			messages, err = tasklog.FindMostRecentLogMessages(taskOpts.TaskID, taskOpts.Execution, getOpts.TailN, nil, logTypeFilter)
			if err != nil {
				return nil, errors.Wrap(err, "getting most recent DB task logs")
			}
		} else {
			msgs, err := tasklog.GetRawTaskLogChannel(taskOpts.TaskID, taskOpts.Execution, nil, logTypeFilter)
			if err != nil {
				return nil, errors.Wrap(err, "getting DB task logs")
			}

			for msg := range msgs {
				messages = append(messages, msg)
			}
		}
		it = apimodels.NewLogMessageIterator(messages)
	}

	return it, nil
}
