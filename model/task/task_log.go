package task

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/send"
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

func (t TaskLogType) Validate(writing bool) error {
	switch t {
	case TaskLogTypeAll, TaskLogTypeAgent, TaskLogTypeSystem, TaskLogTypeTask:
	default:
		return errors.Errorf("unrecognized task log type '%s'", t)
	}

	if writing && t == TaskLogTypeAll {
		return errors.Errorf("cannot persist task log type '%s'", TaskLogTypeAll)
	}

	return nil
}

// TaskLogOutput is the versioned entry point for coordinating persistent
// storage of a task run's task log data.
type TaskLogOutput struct {
	Version      int                    `bson:"version" json:"version"`
	BucketConfig evergreen.BucketConfig `bson:"bucket_config" json:"bucket_config"`

	AWSCredentials aws.CredentialsProvider `bson:"-" json:"-"`
}

// ID returns the unique identifier of the task log output type.
// Note that this is distinct from the task log output type subtype `task_log`.
func (TaskLogOutput) ID() string { return "task_logs" }

// TaskLogGetOptions represents the arguments for fetching task logs belonging
// to a task run.
type TaskLogGetOptions struct {
	// LogType is the type of task log to fetch. Must be a valid task log
	// type.
	LogType TaskLogType
	// Start is the start time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Defaults to
	// unbounded.
	Start *int64
	// End is the end time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Defaults to
	// unbounded.
	End *int64
	// LineLimit limits the number of lines read from the log. Ignored if
	// less than or equal to 0.
	LineLimit int
	// TailN is the number of lines to read from the tail of the log.
	// Ignored if less than or equal to 0.
	TailN int
}

// NewTaskLogSender returns a new task log sender for the given task run.
func NewTaskLogSender(ctx context.Context, task Task, senderOpts EvergreenSenderOptions, logType TaskLogType) (send.Sender, error) {
	output, ok := task.GetTaskOutputSafe()
	if !ok {
		// We know there task cannot have task output, likely because
		// it has not run yet. Return an empty iterator.
		return nil, nil
	}

	if err := logType.Validate(true); err != nil {
		return nil, err
	}

	svc, err := getLogService(ctx, output.TaskLogs)
	if err != nil {
		return nil, errors.Wrap(err, "getting log service")
	}

	senderOpts.appendLines = func(ctx context.Context, lines []log.LogLine) error {
		return svc.Append(ctx, getLogName(task, logType, output.TaskLogs.ID()), 0, lines)
	}

	return newEvergreenSender(ctx, fmt.Sprintf("%s-%s", task.Id, logType), senderOpts)
}

// AppendTaskLogs appends log lines to the specified task log for the given task run.
func AppendTaskLogs(ctx context.Context, task Task, logType TaskLogType, lines []log.LogLine) error {
	output, ok := task.GetTaskOutputSafe()
	if !ok {
		// We know there task cannot have task output, likely because
		// it has not run yet. Return an empty iterator.
		return nil
	}

	if err := logType.Validate(true); err != nil {
		return err
	}

	svc, err := getLogService(ctx, output.TaskLogs)
	if err != nil {
		return errors.Wrap(err, "getting log service")
	}

	return svc.Append(ctx, getLogName(task, logType, output.TaskLogs.ID()), 0, lines)
}

// getTaskLogs returns task logs belonging to the specified task run.
func getTaskLogs(ctx context.Context, task Task, getOpts TaskLogGetOptions) (log.LogIterator, error) {
	output, ok := task.GetTaskOutputSafe()
	if !ok {
		// We know there task cannot have task output, likely because
		// it has not run yet. Return an empty iterator.
		return log.EmptyIterator(), nil
	}

	if err := getOpts.LogType.Validate(false); err != nil {
		return nil, err
	}

	if output.TaskLogs.Version == 0 {
		return getBuildloggerLogs(ctx, task, getOpts)
	}

	svc, err := getLogService(ctx, output.TaskLogs)
	if err != nil {
		return nil, errors.Wrap(err, "getting log service")
	}

	return svc.Get(ctx, log.GetOptions{
		LogNames:  []string{getLogName(task, getOpts.LogType, output.TaskLogs.ID())},
		Start:     getOpts.Start,
		End:       getOpts.End,
		LineLimit: getOpts.LineLimit,
		TailN:     getOpts.TailN,
	})
}

func getLogName(task Task, logType TaskLogType, id string) string {
	prefix := fmt.Sprintf("%s/%s/%d/%s", task.Project, task.Id, task.Execution, id)

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

func getLogService(ctx context.Context, o TaskLogOutput) (log.LogService, error) {
	b, err := newBucket(ctx, o.BucketConfig, o.AWSCredentials)
	if err != nil {
		return nil, err
	}

	return log.NewLogServiceV0(b), nil
}

// getBuildloggerLogs makes request to Cedar Buildlogger for logs.
func getBuildloggerLogs(ctx context.Context, task Task, getOpts TaskLogGetOptions) (log.LogIterator, error) {
	opts := apimodels.GetBuildloggerLogsOptions{
		BaseURL:   evergreen.GetEnvironment().Settings().Cedar.BaseURL,
		TaskID:    task.Id,
		Execution: utility.ToIntPtr(task.Execution),
		Start:     utility.FromInt64Ptr(getOpts.Start),
		End:       utility.FromInt64Ptr(getOpts.End),
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

	return apimodels.GetBuildloggerLogs(ctx, opts)
}
