package task

import (
	"context"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

// TestLogOutput is the versioned entry point for coordinating persistent
// storage of a task run's test log data.
type TestLogOutput struct {
	Version      int                    `bson:"version" json:"version"`
	BucketConfig evergreen.BucketConfig `bson:"bucket_config" json:"bucket_config"`

	AWSCredentials aws.CredentialsProvider `bson:"-" json:"-"`
}

// ID returns the unique identifier of the test log output type.
func (TestLogOutput) ID() string { return "test_logs" }

// TestLogGetOptions represents the arguments for fetching test logs belonging
// to a task run.
type TestLogGetOptions struct {
	// LogPaths are the paths of the logs to fetch and merge, prefixes may
	// be specified. At least one value must be specified.
	LogPaths []string
	// Start is the start time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Defaults to the
	// first timestamp of the first specified log.
	Start *int64
	// End is the end time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Defaults to the last
	// timestamp of the first specified log.
	End *int64
	// LineLimit limits the number of lines read from the log. Ignored if
	// less than or equal to 0.
	LineLimit int
	// TailN is the number of lines to read from the tail of the log.
	// Ignored if less than or equal to 0.
	TailN int
}

// NewTestLogSender returns a new test log sender for the given task run.
func NewTestLogSender(ctx context.Context, task Task, senderOpts EvergreenSenderOptions, logPath string, sequence int) (send.Sender, error) {
	output, ok := task.GetTaskOutputSafe()
	if !ok {
		// We know there task cannot have task output, likely because
		// it has not run yet. Return an empty iterator.
		return nil, nil
	}

	svc, err := getTestLogService(ctx, output.TestLogs)
	if err != nil {
		return nil, errors.Wrap(err, "getting log service")
	}

	senderOpts.appendLines = func(ctx context.Context, lines []log.LogLine) error {
		return svc.Append(ctx, getLogNames(task, []string{logPath}, output.TestLogs.ID())[0], sequence, lines)
	}

	return newEvergreenSender(ctx, fmt.Sprintf("%s-%s", task.Id, logPath), senderOpts)
}

// getTestLogs returns test logs belonging to the specified task run.
func getTestLogs(ctx context.Context, task Task, getOpts TestLogGetOptions) (log.LogIterator, error) {
	output, ok := task.GetTaskOutputSafe()
	if !ok {
		// We know there task cannot have task output, likely because
		// it has not run yet. Return an empty iterator.
		return log.EmptyIterator(), nil
	}

	// If the project is in the long retention list, override the bucket config
	env := evergreen.GetEnvironment()
	var testLogOutput TestLogOutput

	if env != nil && env.Settings() != nil && slices.Contains(env.Settings().Buckets.LongRetentionProjects, task.Project) {
		// Project is in long retention list, use current long retention bucket
		testLogOutput = TestLogOutput{
			Version:        output.TestLogs.Version,
			BucketConfig:   env.Settings().Buckets.LogBucketLongRetention,
			AWSCredentials: output.TestLogs.AWSCredentials,
		}
	} else {
		// Project is not in long retention list, use original bucket config
		testLogOutput = output.TestLogs
	}

	svc, err := getTestLogService(ctx, testLogOutput)
	if err != nil {
		return nil, errors.Wrap(err, "getting log service")
	}

	return svc.Get(ctx, log.GetOptions{
		LogNames:                   getLogNames(task, getOpts.LogPaths, output.TestLogs.ID()),
		Start:                      getOpts.Start,
		End:                        getOpts.End,
		DefaultTimeRangeOfFirstLog: true,
		LineLimit:                  getOpts.LineLimit,
		TailN:                      getOpts.TailN,
	})
}

func getLogNames(task Task, logPaths []string, id string) []string {
	prefix := fmt.Sprintf("%s/%s/%d/%s", task.Project, task.Id, task.Execution, id)

	logNames := make([]string, len(logPaths))
	for i, path := range logPaths {
		logNames[i] = fmt.Sprintf("%s/%s", prefix, path)
	}

	return logNames
}

func getTestLogService(ctx context.Context, o TestLogOutput) (log.LogService, error) {
	b, err := newBucket(ctx, o.BucketConfig, o.AWSCredentials)
	if err != nil {
		return nil, err
	}

	return log.NewLogServiceV0(b), nil
}
