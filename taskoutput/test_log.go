package taskoutput

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// TestLogOutput is the versioned entry point for coordinating persistent
// storage of a task run's test log data.
type TestLogOutput struct {
	Version    int    `bson:"version" json:"version"`
	BucketName string `bson:"bucket_name,omitempty" json:"bucket_name,omitempty"`
	BucketType string `bson:"bucket_type,omitempty" json:"bucket_type,omitempty"`
}

// ID returns the unique identifier of the test log output type.
func (TestLogOutput) ID() string { return "test_logs" }

// TestLogGetOptions represents the arguments for fetching test logs belonging
// to a task run.
type TestLogGetOptions struct {
	// LogPaths are the paths of the logs to fetch and merge, prefixes may
	// be specified. At least one name must be specified.
	LogPaths []string
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

// Get returns test logs belonging to the specified task run.
func (o TestLogOutput) Get(ctx context.Context, env evergreen.Environment, taskOpts TaskOptions, getOpts TestLogGetOptions) (log.LogIterator, error) {
	if o.Version == 0 {
		return o.getBuildloggerLogs(ctx, env, taskOpts, getOpts)
	}

	svc, err := o.getLogService(ctx, env)
	if err != nil {
		return nil, errors.Wrap(err, "getting log service")
	}

	return svc.Get(ctx, log.GetOptions{
		LogNames:  o.getLogNames(taskOpts, getOpts.LogPaths),
		Start:     getOpts.Start,
		End:       getOpts.End,
		LineLimit: getOpts.LineLimit,
		TailN:     getOpts.TailN,
	})
}

func (o TestLogOutput) getLogNames(taskOpts TaskOptions, logPaths []string) []string {
	prefix := fmt.Sprintf("%s/%s/%d/%s", taskOpts.ProjectID, taskOpts.TaskID, taskOpts.Execution, o.ID())

	logNames := make([]string, len(logPaths))
	for i, path := range logPaths {
		logNames[i] = fmt.Sprintf("%s/%s", prefix, path)
	}

	return logNames
}

func (o TestLogOutput) getLogService(ctx context.Context, env evergreen.Environment) (log.LogService, error) {
	b, err := newBucket(ctx, env, o.BucketName, o.BucketType)
	if err != nil {
		return nil, err
	}

	return log.NewLogServiceV0(b), nil
}

// getBuildloggerLogs makes request to Cedar Buildlogger for logs.
func (o TestLogOutput) getBuildloggerLogs(ctx context.Context, env evergreen.Environment, taskOpts TaskOptions, getOpts TestLogGetOptions) (log.LogIterator, error) {
	if len(getOpts.LogPaths) != 1 {
		return nil, errors.New("must request exactly one test log from Cedar Buildlogger")
	}

	opts := apimodels.GetBuildloggerLogsOptionsV2{
		BaseURL:   env.Settings().Cedar.BaseURL,
		TaskID:    taskOpts.TaskID,
		Execution: utility.ToIntPtr(taskOpts.Execution),
		TestName:  getOpts.LogPaths[0],
		Start:     getOpts.Start,
		End:       getOpts.End,
		Limit:     getOpts.LineLimit,
		Tail:      getOpts.TailN,
	}

	return apimodels.GetBuildloggerLogsV2(ctx, opts)
}
