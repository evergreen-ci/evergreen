package task

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/model/s3usage"
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
func NewTaskLogSender(ctx context.Context, task *Task, senderOpts EvergreenSenderOptions, logType TaskLogType) (send.Sender, error) {
	if task == nil {
		return nil, nil
	}

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

	logName := getLogName(*task, logType, output.TaskLogs.ID())
	senderOpts.appendLines = func(ctx context.Context, lines []log.LogLine) (int64, int, error) {
		return svc.Append(ctx, logName, 0, lines)
	}
	senderOpts.LogType = string(logType)
	senderOpts.LogKey = logName

	return newEvergreenSender(ctx, fmt.Sprintf("%s-%s", task.Id, logType), senderOpts)
}

// AppendTaskLogs appends log lines to the specified task log for the given task run.
func AppendTaskLogs(ctx context.Context, task *Task, logType TaskLogType, lines []log.LogLine) error {
	if task == nil {
		return nil
	}

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

	logName := getLogName(*task, logType, output.TaskLogs.ID())
	uploadBytes, puts, err := svc.Append(ctx, logName, 0, lines)
	if puts > 0 {
		task.S3Usage.IncrementLogs(puts, uploadBytes, string(logType), logName)
	}
	if err != nil {
		return err
	}

	return nil
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

	// Get the appropriate bucket config for this project
	bucketConfig := getBucketConfigForProject(task.Project, output.TaskLogs.BucketConfig)

	taskLogOutput := TaskLogOutput{
		Version:        output.TaskLogs.Version,
		BucketConfig:   bucketConfig,
		AWSCredentials: output.TaskLogs.AWSCredentials,
	}

	svc, err := getLogService(ctx, taskLogOutput)
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

// getBucketConfigForProject returns the appropriate bucket config for a project, using long
// retention bucket if the project is in the long retention list. It returns a boolean indicating if the original bucket is being used.
func getBucketConfigForProject(project string, originalBucketConfig evergreen.BucketConfig) evergreen.BucketConfig {
	env := evergreen.GetEnvironment()
	if env != nil && env.Settings() != nil && slices.Contains(env.Settings().Buckets.LongRetentionProjects, project) {
		return env.Settings().Buckets.LogBucketLongRetention
	}
	return originalBucketConfig
}

// GetS3LogUsageFromS3 reconstructs log S3 usage for the crash path, when the agent never
// reached teardown. Chunk count approximates PUT count (exact under 10MB, lower bound otherwise).
func (t *Task) GetS3LogUsageFromS3(ctx context.Context) (s3usage.LogMetrics, error) {
	output, ok := t.GetTaskOutputSafe()
	if !ok {
		return s3usage.LogMetrics{}, nil
	}

	bucketConfig := getBucketConfigForProject(t.Project, output.TaskLogs.BucketConfig)
	bucket, err := newBucket(ctx, bucketConfig, nil)
	if err != nil {
		return s3usage.LogMetrics{}, errors.Wrap(err, "creating log bucket")
	}

	id := output.TaskLogs.ID()
	// The LCP of the agent, system, and task log type prefixes covers all three in a single ListObjectsV2 call.
	prefix := fmt.Sprintf("%s/%s/%d/%s", t.Project, t.Id, t.Execution, id)
	it, err := bucket.List(ctx, prefix)
	if err != nil {
		return s3usage.LogMetrics{}, errors.Wrap(err, "listing log chunks")
	}

	agentLogName := getLogName(*t, TaskLogTypeAgent, id)
	systemLogName := getLogName(*t, TaskLogTypeSystem, id)
	taskLogName := getLogName(*t, TaskLogTypeTask, id)

	var usage s3usage.LogMetrics
	for it.Next(ctx) {
		item := it.Item()
		size := item.Size()
		usage.PutRequests++
		usage.UploadBytes += size

		name := item.Name()
		switch {
		case strings.HasPrefix(name, agentLogName+"/"):
			usage.Agent.Bytes += size
			usage.Agent.PutRequests++
			if usage.Agent.LogKey == "" {
				usage.Agent.LogKey = agentLogName
			}
		case strings.HasPrefix(name, systemLogName+"/"):
			usage.System.Bytes += size
			usage.System.PutRequests++
			if usage.System.LogKey == "" {
				usage.System.LogKey = systemLogName
			}
		case strings.HasPrefix(name, taskLogName+"/"):
			usage.Task.Bytes += size
			usage.Task.PutRequests++
			if usage.Task.LogKey == "" {
				usage.Task.LogKey = taskLogName
			}
		}
	}
	if err := it.Err(); err != nil {
		return s3usage.LogMetrics{}, errors.Wrap(err, "iterating log chunks")
	}

	// Test logs may be in a different bucket than task/agent/system logs.
	if output.TestLogs.BucketConfig.Name != "" {
		testBucketConfig := getBucketConfigForProject(t.Project, output.TestLogs.BucketConfig)
		testBucket, err := newBucket(ctx, testBucketConfig, nil)
		if err != nil {
			return usage, errors.Wrap(err, "creating test log bucket")
		}
		testPrefix := fmt.Sprintf("%s/%s/%d/%s", t.Project, t.Id, t.Execution, output.TestLogs.ID())
		testIt, err := testBucket.List(ctx, testPrefix)
		if err != nil {
			return usage, errors.Wrap(err, "listing test log chunks")
		}
		for testIt.Next(ctx) {
			item := testIt.Item()
			size := item.Size()
			usage.PutRequests++
			usage.UploadBytes += size
			usage.Test.Bytes += size
			usage.Test.PutRequests++
			if usage.Test.LogKey == "" {
				usage.Test.LogKey = testPrefix
			}
		}
		if err := testIt.Err(); err != nil {
			return usage, errors.Wrap(err, "iterating test log chunks")
		}
	}

	return usage, nil
}

// LogBucketName returns the S3 bucket name for this task's logs, applying the
// long-retention redirect if the project is in the long-retention list.
func (t *Task) LogBucketName() string {
	output, ok := t.GetTaskOutputSafe()
	if !ok {
		return ""
	}
	return getBucketConfigForProject(t.Project, output.TaskLogs.BucketConfig).Name
}
