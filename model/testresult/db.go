package testresult

import (
	"crypto/sha1"
	"fmt"
	"io"
	"time"

	"github.com/mongodb/anser/bsonutil"
)

const (
	Collection        = "test_results"
	parquetDateFormat = "2006-01-02"
)

type DbTaskTestResults struct {
	ID          string               `bson:"_id"`
	Stats       TaskTestResultsStats `bson:"stats"`
	Info        TestResultsInfo      `bson:"info"`
	CreatedAt   time.Time            `bson:"created_at"`
	CompletedAt time.Time            `bson:"completed_at"`
	// FailedTestsSample is the first X failing tests of the test Results.
	// This is an optimization for Evergreen's UI features that display a
	// limited number of failing tests for a task.
	FailedTestsSample []string     `bson:"failed_tests_sample"`
	Results           []TestResult `bson:"-"`
}

// ID creates a unique hash for a TestResultsInfo.
func (t *TestResultsInfo) ID() string {
	hash := sha1.New()
	_, _ = io.WriteString(hash, t.Project)
	_, _ = io.WriteString(hash, t.Version)
	_, _ = io.WriteString(hash, t.Variant)
	_, _ = io.WriteString(hash, t.TaskName)
	_, _ = io.WriteString(hash, t.DisplayTaskName)
	_, _ = io.WriteString(hash, t.TaskID)
	_, _ = io.WriteString(hash, t.DisplayTaskID)
	_, _ = io.WriteString(hash, fmt.Sprint(t.Execution))
	_, _ = io.WriteString(hash, t.Requester)

	return fmt.Sprintf("%x", hash.Sum(nil))
}

// PartitionKey returns the partition key for the S3 bucket in Presto.
func PartitionKey(createdAt time.Time, project string, id string) string {
	return fmt.Sprintf("task_create_iso=%s/project=%s/%s", createdAt.UTC().Format(parquetDateFormat), project, id)
}

var (
	IdKey                           = bsonutil.MustHaveTag(DbTaskTestResults{}, "ID")
	StatsKey                        = bsonutil.MustHaveTag(DbTaskTestResults{}, "Stats")
	TotalCountKey                   = bsonutil.MustHaveTag(TaskTestResultsStats{}, "TotalCount")
	FailedCountKey                  = bsonutil.MustHaveTag(TaskTestResultsStats{}, "FailedCount")
	TestResultsInfoKey              = bsonutil.MustHaveTag(DbTaskTestResults{}, "Info")
	TestResultsFailedTestsSampleKey = bsonutil.MustHaveTag(DbTaskTestResults{}, "FailedTestsSample")
	TestResultsInfoTaskIDKey        = bsonutil.MustHaveTag(TestResultsInfo{}, "TaskID")
	TestResultsInfoExecutionKey     = bsonutil.MustHaveTag(TestResultsInfo{}, "Execution")
)
