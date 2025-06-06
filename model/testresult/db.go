package testresult

import (
	"github.com/mongodb/anser/bsonutil"
)

const Collection = "testresults"

type DbTaskTestResults struct {
	ID      DbTaskTestResultsID  `bson:"_id"`
	Stats   TaskTestResultsStats `bson:"stats"`
	Results []TestResult         `bson:"results"`
}

type DbTaskTestResultsID struct {
	TaskID    string `bson:"task_id"`
	Execution int    `bson:"execution"`
}

var (
	IdKey      = bsonutil.MustHaveTag(DbTaskTestResults{}, "ID")
	StatsKey   = bsonutil.MustHaveTag(DbTaskTestResults{}, "Stats")
	ResultsKey = bsonutil.MustHaveTag(DbTaskTestResults{}, "Results")

	TaskIDKey    = bsonutil.MustHaveTag(DbTaskTestResultsID{}, "TaskID")
	ExecutionKey = bsonutil.MustHaveTag(DbTaskTestResultsID{}, "Execution")

	TotalCountKey  = bsonutil.MustHaveTag(TaskTestResultsStats{}, "TotalCount")
	FailedCountKey = bsonutil.MustHaveTag(TaskTestResultsStats{}, "FailedCount")
)
