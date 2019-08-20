package stats

// This file provides database layer logic for pre-computed test ant task execution statistics.
// The database schema is the following:
// *daily_stats_status*
// {
//   "_id": <Project Id (string)>,
//   "last_job_run": <Date of the last successful job run that updated the project stats (date)>
//   "processed_tasks_until": <Date before which finished tasks have been processed by a successful job (date)>
// }
// *hourly_test_stats*
// {
//   "_id": {
//     "test_file": <Test file (string)>,
//     "task_name": <Task display name (string)>,
//     "variant": <Build variant (string)>,
//     "distro": <Distro (string)>,
//     "project": <Project Id (string)>,
//     "date": <UTC hour period this document covers (date)>,
//   },
//   "num_pass": <Number of times the test passed (int)>,
//   "num_fail": <Number of times the test failed (int)>,
//   "avg_duration_pass": <Average duration in seconds of the passing tests (double)>,
//   "last_update": <Date of the job run that last updated this document (date)>
// }
// *daily_test_stats*
// {
//   "_id": {
//     "test_file": <Test file (string)>,
//     "task_name": <Task display name (string)>,
//     "variant": <Build variant (string)>,
//     "distro": <Distro (string)>,
//     "project": <Project Id (string)>,
//     "date": <UTC day period this document covers (date)>,
//   },
//   "num_pass": <Number of times the test passed (int)>,
//   "num_fail": <Number of times the test failed (int)>,
//   "avg_duration_pass": <Average duration in seconds of the passing tests (double)>,
//   "last_update": <Date of the job run that last updated this document (date)>
// }
// *daily_task_stats*
// {
//   "_id": {
//     "task_name": <Task display name (string)>,
//     "variant": <Build variant (string)>,
//     "distro": <Distro (string)>,
//     "project": <Project Id (string)>,
//     "date": <UTC day period this document covers (date)>,
//   },
//   "num_success": <Number of times the task was successful (int)>,
//   "num_failed": <Number of times the task failed (int)>,
//   "num_test_failed": <Number of times the task failed with a details type of 'test' (int)>,
//   "num_setup_failed": <Number of times the task failed with a details type of 'setup' (int)>,
//   "num_system_failed": <Number of times the task failed with a details type of 'system' (int)>,
//   "num_timeout": <Number of times the task failed with a timeout (int)>,
//   "avg_duration_success": <Average duration in seconds of the successful tasks (double)>,
//   "last_update": <Date of the job run that last updated this document (date)>
// }

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	hourlyTestStatsCollection  = "hourly_test_stats"
	dailyTestStatsCollection   = "daily_test_stats"
	DailyTaskStatsCollection   = "daily_task_stats"
	dailyStatsStatusCollection = "daily_stats_status"
	bulkSize                   = 1000
	nsInASecond                = time.Second / time.Nanosecond
)

var (
	// $ references to the BSON fields of tasks.
	taskIdKeyRef           = "$" + task.IdKey
	taskExecutionKeyRef    = "$" + task.ExecutionKey
	taskProjectKeyRef      = "$" + task.ProjectKey
	taskDisplayNameKeyRef  = "$" + task.DisplayNameKey
	taskCreateTimeKeyRef   = "$" + task.CreateTimeKey
	taskBuildVariantKeyRef = "$" + task.BuildVariantKey
	taskRequesterKeyRef    = "$" + task.RequesterKey
	taskDistroIdKeyRef     = "$" + task.DistroIdKey
	taskStatusKeyRef       = "$" + task.StatusKey
	taskDetailsKeyRef      = "$" + task.DetailsKey
	taskTimeTakenKeyRef    = "$" + task.TimeTakenKey
	testResultTaskIdKeyRef = "$" + testresult.TaskIDKey
	testResultExecutionRef = "$" + testresult.ExecutionKey
)

// Convenient type to use for arrays in pipeline definitions.
type Array []interface{}

//////////////////
// Stats Status //
//////////////////

// statsStatusQuery returns a query to find a stats status document by project id.
func statsStatusQuery(projectId string) bson.M {
	return bson.M{"_id": projectId}
}

///////////////////////
// Hourly Test Stats //
///////////////////////

// DbTestStatsId represents the _id field for hourly_test_stats and daily_test_stats documents.
type DbTestStatsId struct {
	TestFile     string    `bson:"test_file"`
	TaskName     string    `bson:"task_name"`
	BuildVariant string    `bson:"variant"`
	Distro       string    `bson:"distro"`
	Project      string    `bson:"project"`
	Requester    string    `bson:"requester"`
	Date         time.Time `bson:"date"`
}

// dbTestStats represents the hourly_test_stats and daily_test_stats documents.
type dbTestStats struct {
	Id              DbTestStatsId `bson:"_id"`
	NumPass         int           `bson:"num_pass"`
	NumFail         int           `bson:"num_fail"`
	AvgDurationPass float64       `bson:"avg_duration_pass"`
	LastUpdate      time.Time     `bson:"last_update"`
}

func (d *dbTestStats) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(d) }
func (d *dbTestStats) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, d) }

var (
	// BSON fields for the test stats id struct
	dbTestStatsIdTestFileKey     = bsonutil.MustHaveTag(DbTestStatsId{}, "TestFile")
	dbTestStatsIdTaskNameKey     = bsonutil.MustHaveTag(DbTestStatsId{}, "TaskName")
	dbTestStatsIdBuildVariantKey = bsonutil.MustHaveTag(DbTestStatsId{}, "BuildVariant")
	dbTestStatsIdDistroKey       = bsonutil.MustHaveTag(DbTestStatsId{}, "Distro")
	dbTestStatsIdProjectKey      = bsonutil.MustHaveTag(DbTestStatsId{}, "Project")
	dbTestStatsIdRequesterKey    = bsonutil.MustHaveTag(DbTestStatsId{}, "Requester")
	dbTestStatsIdDateKey         = bsonutil.MustHaveTag(DbTestStatsId{}, "Date")

	// BSON fields for the test stats struct
	dbTestStatsIdKey              = bsonutil.MustHaveTag(dbTestStats{}, "Id")
	dbTestStatsNumPassKey         = bsonutil.MustHaveTag(dbTestStats{}, "NumPass")
	dbTestStatsNumFailKey         = bsonutil.MustHaveTag(dbTestStats{}, "NumFail")
	dbTestStatsAvgDurationPassKey = bsonutil.MustHaveTag(dbTestStats{}, "AvgDurationPass")
	dbTestStatsLastUpdateKey      = bsonutil.MustHaveTag(dbTestStats{}, "LastUpdate")

	// BSON dotted field names for test stats id elements
	DbTestStatsIdTestFileKeyFull     = bsonutil.GetDottedKeyName(dbTestStatsIdKey, dbTestStatsIdTestFileKey)
	DbTestStatsIdTaskNameKeyFull     = bsonutil.GetDottedKeyName(dbTestStatsIdKey, dbTestStatsIdTaskNameKey)
	DbTestStatsIdBuildVariantKeyFull = bsonutil.GetDottedKeyName(dbTestStatsIdKey, dbTestStatsIdBuildVariantKey)
	DbTestStatsIdDistroKeyFull       = bsonutil.GetDottedKeyName(dbTestStatsIdKey, dbTestStatsIdDistroKey)
	DbTestStatsIdProjectKeyFull      = bsonutil.GetDottedKeyName(dbTestStatsIdKey, dbTestStatsIdProjectKey)
	DbTestStatsIdRequesterKeyFull    = bsonutil.GetDottedKeyName(dbTestStatsIdKey, dbTestStatsIdRequesterKey)
	DbTestStatsIdDateKeyFull         = bsonutil.GetDottedKeyName(dbTestStatsIdKey, dbTestStatsIdDateKey)
)

// hourlyTestStatsPipeline returns a pipeline aggregating task documents into hourly test stats.
func hourlyTestStatsPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time) []bson.M {
	return getHourlyTestStatsPipeline(projectId, requester, start, end, tasks, lastUpdate)
}

// getHourlyTestStatsPipeline is an internal helper function to create a pipeline aggregating task
// documents into hourly test stats.
func getHourlyTestStatsPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time) []bson.M {
	var taskIdExpr string
	var displayTaskLookupCollection string
	taskIdExpr = taskIdKeyRef
	displayTaskLookupCollection = task.Collection
	pipeline := []bson.M{
		{"$match": bson.M{
			task.ProjectKey:     projectId,
			task.RequesterKey:   requester,
			task.CreateTimeKey:  bson.M{"$gte": start, "$lt": end},
			task.DisplayNameKey: bson.M{"$in": tasks},
		}},
		{"$project": bson.M{
			task.IdKey:                   0,
			"task_id":                    taskIdExpr,
			"execution":                  taskExecutionKeyRef,
			dbTestStatsIdProjectKey:      taskProjectKeyRef,
			dbTestStatsIdTaskNameKey:     taskDisplayNameKeyRef,
			dbTestStatsIdBuildVariantKey: taskBuildVariantKeyRef,
			dbTestStatsIdDistroKey:       taskDistroIdKeyRef,
			dbTestStatsIdRequesterKey:    taskRequesterKeyRef}},
		{"$lookup": bson.M{
			"from":         displayTaskLookupCollection,
			"localField":   "task_id",
			"foreignField": task.ExecutionTasksKey,
			"as":           "display_task"}},
		{"$unwind": bson.M{
			"path":                       "$display_task",
			"preserveNullAndEmptyArrays": true}},
		{"$lookup": bson.M{
			"from": testresult.Collection,
			"let":  bson.M{"task_id": "$task_id", "execution": "$execution"},
			"pipeline": []bson.M{
				{"$match": bson.M{"$expr": bson.M{"$and": []bson.M{
					{"$eq": Array{testResultTaskIdKeyRef, "$$task_id"}},
					{"$eq": Array{testResultExecutionRef, "$$execution"}}}}}},
				{"$project": bson.M{
					testresult.IDKey:        0,
					testresult.TestFileKey:  1,
					testresult.StatusKey:    1,
					testresult.StartTimeKey: 1,
					testresult.EndTimeKey:   1}}},
			"as": "testresults"}},
		{"$unwind": "$testresults"},
		{"$project": bson.M{
			dbTestStatsIdTestFileKey: "$testresults." + testresult.TestFileKey,
			// We use the name of the display task if there is one.
			dbTestStatsIdTaskNameKey:     bson.M{"$ifNull": Array{"$display_task." + task.DisplayNameKey, "$task_name"}},
			dbTestStatsIdBuildVariantKey: 1,
			dbTestStatsIdDistroKey:       1,
			dbTestStatsIdProjectKey:      1,
			dbTestStatsIdRequesterKey:    1,
			"status":                     "$testresults." + task.StatusKey,
			"duration":                   bson.M{"$subtract": Array{"$testresults." + testresult.EndTimeKey, "$testresults." + testresult.StartTimeKey}}}},
		{"$group": bson.M{
			"_id": bson.D{
				{Key: dbTestStatsIdTestFileKey, Value: "$" + dbTestStatsIdTestFileKey},
				{Key: dbTestStatsIdTaskNameKey, Value: "$" + dbTestStatsIdTaskNameKey},
				{Key: dbTestStatsIdBuildVariantKey, Value: "$" + dbTestStatsIdBuildVariantKey},
				{Key: dbTestStatsIdDistroKey, Value: "$" + dbTestStatsIdDistroKey},
				{Key: dbTestStatsIdProjectKey, Value: "$" + dbTestStatsIdProjectKey},
				{Key: dbTestStatsIdRequesterKey, Value: "$" + dbTestStatsIdRequesterKey},
			},
			dbTestStatsNumPassKey: makeSum(bson.M{"$eq": Array{"$status", evergreen.TestSucceededStatus}}),
			dbTestStatsNumFailKey: makeSum(bson.M{"$in": Array{"$status", Array{evergreen.TestFailedStatus, evergreen.TestSilentlyFailedStatus}}}),
			// "IGNORE" is not a special value, setting the value to something that is not a number will cause $avg to ignore it
			dbTestStatsAvgDurationPassKey: bson.M{"$avg": bson.M{"$cond": bson.M{
				"if":   bson.M{"$eq": Array{"$status", evergreen.TestSucceededStatus}},
				"then": "$duration",
				"else": "IGNORE"}}}}},
		{"$addFields": bson.M{
			"_id." + dbTestStatsIdDateKey: start,
			dbTestStatsLastUpdateKey:      lastUpdate,
		}},
	}
	return pipeline
}

//////////////////////
// Daily Test Stats //
//////////////////////

// dailyTestStatsFromHourlyPipeline returns a pipeline aggregating hourly test stats into daily test stats.
func dailyTestStatsFromHourlyPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time) []bson.M {
	pipeline := []bson.M{
		{"$match": bson.M{
			DbTestStatsIdProjectKeyFull:   projectId,
			DbTestStatsIdRequesterKeyFull: requester,
			DbTestStatsIdDateKeyFull:      bson.M{"$gte": start, "$lt": end},
			DbTestStatsIdTaskNameKeyFull:  bson.M{"$in": tasks},
		}},
		{
			"$group": bson.M{
				"_id": bson.D{
					{Key: dbTestStatsIdTestFileKey, Value: "$" + DbTestStatsIdTestFileKeyFull},
					{Key: dbTestStatsIdTaskNameKey, Value: "$" + DbTestStatsIdTaskNameKeyFull},
					{Key: dbTestStatsIdBuildVariantKey, Value: "$" + DbTestStatsIdBuildVariantKeyFull},
					{Key: dbTestStatsIdDistroKey, Value: "$" + DbTestStatsIdDistroKeyFull},
					{Key: dbTestStatsIdProjectKey, Value: "$" + DbTestStatsIdProjectKeyFull},
					{Key: dbTestStatsIdRequesterKey, Value: "$" + DbTestStatsIdRequesterKeyFull},
				},
				dbTestStatsNumPassKey: bson.M{"$sum": "$" + dbTestStatsNumPassKey},
				dbTestStatsNumFailKey: bson.M{"$sum": "$" + dbTestStatsNumFailKey},
				"total_duration_pass": bson.M{"$sum": bson.M{"$multiply": Array{"$num_pass", "$" + dbTestStatsAvgDurationPassKey}}},
			},
		},
		{
			"$project": bson.M{
				"_id":                 1,
				dbTestStatsNumPassKey: 1,
				dbTestStatsNumFailKey: 1,
				dbTestStatsAvgDurationPassKey: bson.M{"$cond": bson.M{"if": bson.M{"$ne": Array{"$" + dbTestStatsNumPassKey, 0}},
					"then": bson.M{"$divide": Array{"$total_duration_pass", "$" + dbTestStatsNumPassKey}},
					"else": nil}},
			},
		},
		{"$addFields": bson.M{
			"_id." + dbTestStatsIdDateKey: start,
			dbTestStatsLastUpdateKey:      lastUpdate,
		}},
	}
	return pipeline
}

//////////////////////
// Daily Task Stats //
//////////////////////

// DbTaskStatsId represents the _id field for daily_task_stats documents.
type DbTaskStatsId struct {
	TaskName     string    `bson:"task_name"`
	BuildVariant string    `bson:"variant"`
	Distro       string    `bson:"distro"`
	Project      string    `bson:"project"`
	Requester    string    `bson:"requester"`
	Date         time.Time `bson:"date"`
}

// dbTaskStats represents the daily_task_stats documents.
type dbTaskStats struct {
	Id                 DbTaskStatsId `bson:"_id"`
	NumSuccess         int           `bson:"num_success"`
	NumFailed          int           `bson:"num_failed"`
	NumTimeout         int           `bson:"num_timeout"`
	NumTestFailed      int           `bson:"num_test_failed"`
	NumSystemFailed    int           `bson:"num_system_failed"`
	NumSetupFailed     int           `bson:"num_setup_failed"`
	AvgDurationSuccess float64       `bson:"avg_duration_success"`
	LastUpdate         time.Time     `bson:"last_update"`
}

func (d *dbTaskStats) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(d) }
func (d *dbTaskStats) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, d) }

var (
	// BSON fields for the task stats id struct
	DbTaskStatsIdTaskNameKey     = bsonutil.MustHaveTag(DbTaskStatsId{}, "TaskName")
	DbTaskStatsIdBuildVariantKey = bsonutil.MustHaveTag(DbTaskStatsId{}, "BuildVariant")
	DbTaskStatsIdDistroKey       = bsonutil.MustHaveTag(DbTaskStatsId{}, "Distro")
	DbTaskStatsIdProjectKey      = bsonutil.MustHaveTag(DbTaskStatsId{}, "Project")
	DbTaskStatsIdRequesterKey    = bsonutil.MustHaveTag(DbTaskStatsId{}, "Requester")
	DbTaskStatsIdDateKey         = bsonutil.MustHaveTag(DbTaskStatsId{}, "Date")

	// BSON fields for the test stats struct
	DbTaskStatsIdKey                 = bsonutil.MustHaveTag(dbTaskStats{}, "Id")
	DbTaskStatsNumSuccessKey         = bsonutil.MustHaveTag(dbTaskStats{}, "NumSuccess")
	DbTaskStatsNumFailedKey          = bsonutil.MustHaveTag(dbTaskStats{}, "NumFailed")
	DbTaskStatsNumTestFailedKey      = bsonutil.MustHaveTag(dbTaskStats{}, "NumTestFailed")
	DbTaskStatsNumSetupFailedKey     = bsonutil.MustHaveTag(dbTaskStats{}, "NumSetupFailed")
	DbTaskStatsNumSystemFailedKey    = bsonutil.MustHaveTag(dbTaskStats{}, "NumSystemFailed")
	DbTaskStatsNumTimeoutKey         = bsonutil.MustHaveTag(dbTaskStats{}, "NumTimeout")
	DbTaskStatsAvgDurationSuccessKey = bsonutil.MustHaveTag(dbTaskStats{}, "AvgDurationSuccess")
	DbTaskStatsLastUpdateKey         = bsonutil.MustHaveTag(dbTaskStats{}, "LastUpdate")

	// BSON dotted field names for task stats id elements
	DbTaskStatsIdTaskNameKeyFull     = bsonutil.GetDottedKeyName(DbTaskStatsIdKey, DbTaskStatsIdTaskNameKey)
	DbTaskStatsIdBuildVariantKeyFull = bsonutil.GetDottedKeyName(DbTaskStatsIdKey, DbTaskStatsIdBuildVariantKey)
	DbTaskStatsIdDistroKeyFull       = bsonutil.GetDottedKeyName(DbTaskStatsIdKey, DbTaskStatsIdDistroKey)
	DbTaskStatsIdDateKeyFull         = bsonutil.GetDottedKeyName(DbTaskStatsIdKey, DbTaskStatsIdDateKey)
)

// dailyTaskStatsPipeline returns a pipeline aggregating task documents into daily task stats.
func dailyTaskStatsPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time) []bson.M {
	return getDailyTaskStatsPipeline(projectId, requester, start, end, tasks, lastUpdate)
}

// getDailyTaskStatsPipeline is an internal helper function to create a pipeline aggregating task
// documents into daily task stats.
func getDailyTaskStatsPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time) []bson.M {
	var taskIdExpr string
	var displayTaskLookupCollection string
	taskIdExpr = taskIdKeyRef
	displayTaskLookupCollection = task.Collection
	pipeline := []bson.M{
		{"$match": bson.M{
			task.ProjectKey:     projectId,
			task.RequesterKey:   requester,
			task.CreateTimeKey:  bson.M{"$gte": start, "$lt": end},
			task.DisplayNameKey: bson.M{"$in": tasks},
		}},
		{"$project": bson.M{
			task.IdKey:                   0,
			"task_id":                    taskIdExpr,
			"execution":                  taskExecutionKeyRef,
			DbTaskStatsIdProjectKey:      taskProjectKeyRef,
			DbTaskStatsIdTaskNameKey:     taskDisplayNameKeyRef,
			DbTaskStatsIdBuildVariantKey: taskBuildVariantKeyRef,
			DbTaskStatsIdDistroKey:       taskDistroIdKeyRef,
			DbTaskStatsIdRequesterKey:    taskRequesterKeyRef,
			task.StatusKey:               1,
			task.DetailsKey:              1,
			"time_taken":                 bson.M{"$divide": Array{taskTimeTakenKeyRef, nsInASecond}},
		}},
		{"$lookup": bson.M{
			"from":         displayTaskLookupCollection,
			"localField":   "task_id",
			"foreignField": task.ExecutionTasksKey,
			"as":           "display_task",
		}},
		{"$match": bson.M{"display_task": Array{}}}, // Excluding the execution tasks
		{"$group": bson.M{
			"_id": bson.D{
				{Key: DbTaskStatsIdTaskNameKey, Value: "$" + DbTaskStatsIdTaskNameKey},
				{Key: DbTaskStatsIdBuildVariantKey, Value: "$" + DbTaskStatsIdBuildVariantKey},
				{Key: DbTaskStatsIdDistroKey, Value: "$" + DbTaskStatsIdDistroKey},
				{Key: DbTaskStatsIdProjectKey, Value: "$" + DbTaskStatsIdProjectKey},
				{Key: DbTaskStatsIdRequesterKey, Value: "$" + DbTaskStatsIdRequesterKey}},
			DbTaskStatsNumSuccessKey: makeSum(bson.M{"$eq": Array{"$status", "success"}}),
			DbTaskStatsNumFailedKey:  makeSum(bson.M{"$eq": Array{"$status", "failed"}}),
			DbTaskStatsNumTimeoutKey: makeSum(bson.M{"$and": Array{
				bson.M{"$eq": Array{taskStatusKeyRef, "failed"}},
				bson.M{"$eq": Array{bsonutil.GetDottedKeyName(taskDetailsKeyRef, task.TaskEndDetailTimedOut), true}}}}),
			DbTaskStatsNumTestFailedKey: makeSum(bson.M{"$and": Array{
				bson.M{"$eq": Array{taskStatusKeyRef, "failed"}},
				bson.M{"$eq": Array{bsonutil.GetDottedKeyName(taskDetailsKeyRef, task.TaskEndDetailType), "test"}},
				bson.M{"$ne": Array{bsonutil.GetDottedKeyName(taskDetailsKeyRef, task.TaskEndDetailTimedOut), true}}}}),
			DbTaskStatsNumSystemFailedKey: makeSum(bson.M{"$and": Array{
				bson.M{"$eq": Array{taskStatusKeyRef, "failed"}},
				bson.M{"$eq": Array{bsonutil.GetDottedKeyName(taskDetailsKeyRef, task.TaskEndDetailType), "system"}},
				bson.M{"$ne": Array{bsonutil.GetDottedKeyName(taskDetailsKeyRef, task.TaskEndDetailTimedOut), true}}}}),
			DbTaskStatsNumSetupFailedKey: makeSum(bson.M{"$and": Array{
				bson.M{"$eq": Array{taskStatusKeyRef, "failed"}},
				bson.M{"$eq": Array{bsonutil.GetDottedKeyName(taskDetailsKeyRef, task.TaskEndDetailType), "setup"}},
				bson.M{"$ne": Array{bsonutil.GetDottedKeyName(taskDetailsKeyRef, task.TaskEndDetailTimedOut), true}}}}),
			DbTaskStatsAvgDurationSuccessKey: bson.M{"$avg": bson.M{"$cond": bson.M{"if": bson.M{"$eq": Array{taskStatusKeyRef, "success"}},
				"then": "$time_taken", "else": "IGNORE"}}}}},
		{"$addFields": bson.M{
			"_id." + DbTaskStatsIdDateKey: start,
			DbTaskStatsLastUpdateKey:      lastUpdate,
		}},
	}
	return pipeline
}

var (
	statsToUpdateProjectKey   = bsonutil.MustHaveTag(StatsToUpdate{}, "ProjectId")
	statsToUpdateRequesterKey = bsonutil.MustHaveTag(StatsToUpdate{}, "Requester")
	statsToUpdateDayKey       = bsonutil.MustHaveTag(StatsToUpdate{}, "Day")
	statsToUpdateHourKey      = bsonutil.MustHaveTag(StatsToUpdate{}, "Hour")
	statsToUpdateTasksKey     = bsonutil.MustHaveTag(StatsToUpdate{}, "Tasks")
)

// statsToUpdatePipeline returns a pipeline aggregating task documents into documents describing tasks for which
// the stats need to be updated.
func statsToUpdatePipeline(projectID string, requester []string, start, end time.Time) []bson.M {
	match := bson.M{
		task.ProjectKey:    projectID,
		task.FinishTimeKey: bson.M{"$gte": start, "$lt": end},
	}

	if len(requester) == 1 {
		match[task.RequesterKey] = requester[0]
	} else if len(requester) > 1 {
		match[task.RequesterKey] = bson.M{"$in": requester}
	}

	return []bson.M{
		{"$match": match},
		{"$project": bson.M{
			task.IdKey:                0,
			statsToUpdateProjectKey:   taskProjectKeyRef,
			statsToUpdateRequesterKey: taskRequesterKeyRef,
			statsToUpdateHourKey:      bson.M{"$dateToString": bson.M{"date": taskCreateTimeKeyRef, "format": "%Y-%m-%d %H"}},
			statsToUpdateDayKey:       bson.M{"$dateToString": bson.M{"date": taskCreateTimeKeyRef, "format": "%Y-%m-%d"}},
			"task_name":               taskDisplayNameKeyRef,
		}},
		{"$group": bson.M{
			"_id": bson.M{
				statsToUpdateProjectKey:   "$" + statsToUpdateProjectKey,
				statsToUpdateRequesterKey: "$" + statsToUpdateRequesterKey,
				statsToUpdateHourKey:      "$" + statsToUpdateHourKey,
				statsToUpdateDayKey:       "$" + statsToUpdateDayKey,
			},
			statsToUpdateTasksKey: bson.M{"$addToSet": "$task_name"},
		}},
		{"$project": bson.M{
			"_id":                     0,
			statsToUpdateProjectKey:   "$_id." + statsToUpdateProjectKey,
			statsToUpdateRequesterKey: "$_id." + statsToUpdateRequesterKey,
			statsToUpdateHourKey:      bson.M{"$dateFromString": bson.M{"dateString": "$_id." + statsToUpdateHourKey, "format": "%Y-%m-%d %H"}},
			statsToUpdateDayKey:       bson.M{"$dateFromString": bson.M{"dateString": "$_id." + statsToUpdateDayKey, "format": "%Y-%m-%d"}},
			statsToUpdateTasksKey:     1,
		}},
		{"$sort": bson.D{
			{Key: statsToUpdateProjectKey, Value: 1},
			{Key: statsToUpdateHourKey, Value: 1},
			{Key: statsToUpdateRequesterKey, Value: 1},
		}},
	}
}

///////////////////////////////////////////
// Queries on the precomputed statistics //
///////////////////////////////////////////

var (
	// BSON fields for the test stats struct
	TestStatsTestFileKey        = bsonutil.MustHaveTag(TestStats{}, "TestFile")
	TestStatsTaskNameKey        = bsonutil.MustHaveTag(TestStats{}, "TaskName")
	TestStatsBuildVariantKey    = bsonutil.MustHaveTag(TestStats{}, "BuildVariant")
	TestStatsDistroKey          = bsonutil.MustHaveTag(TestStats{}, "Distro")
	TestStatsDateKey            = bsonutil.MustHaveTag(TestStats{}, "Date")
	TestStatsNumPassKey         = bsonutil.MustHaveTag(TestStats{}, "NumPass")
	TestStatsNumFailKey         = bsonutil.MustHaveTag(TestStats{}, "NumFail")
	TestStatsAvgDurationPassKey = bsonutil.MustHaveTag(TestStats{}, "AvgDurationPass")
	TestStatsLastUpdateKey      = bsonutil.MustHaveTag(TestStats{}, "LastUpdate")

	// BSON fields for the task stats struct
	TaskStatsTaskNameKey           = bsonutil.MustHaveTag(TaskStats{}, "TaskName")
	TaskStatsBuildVariantKey       = bsonutil.MustHaveTag(TaskStats{}, "BuildVariant")
	TaskStatsDistroKey             = bsonutil.MustHaveTag(TaskStats{}, "Distro")
	TaskStatsDateKey               = bsonutil.MustHaveTag(TaskStats{}, "Date")
	TaskStatsNumSuccessKey         = bsonutil.MustHaveTag(TaskStats{}, "NumSuccess")
	TaskStatsNumFailedKey          = bsonutil.MustHaveTag(TaskStats{}, "NumFailed")
	TaskStatsNumTotalKey           = bsonutil.MustHaveTag(TaskStats{}, "NumTotal")
	TaskStatsNumTestFailedKey      = bsonutil.MustHaveTag(TaskStats{}, "NumTestFailed")
	TaskStatsNumSetupFailedKey     = bsonutil.MustHaveTag(TaskStats{}, "NumSetupFailed")
	TaskStatsNumSystemFailedKey    = bsonutil.MustHaveTag(TaskStats{}, "NumSystemFailed")
	TaskStatsNumTimeoutKey         = bsonutil.MustHaveTag(TaskStats{}, "NumTimeout")
	TaskStatsAvgDurationSuccessKey = bsonutil.MustHaveTag(TaskStats{}, "AvgDurationSuccess")
	TaskStatsLastUpdateKey         = bsonutil.MustHaveTag(TaskStats{}, "LastUpdate")
)

// testStatsQueryPipeline creates an aggregation pipeline to query test statistics.
func (filter StatsFilter) testStatsQueryPipeline() []bson.M {
	matchExpr := filter.buildMatchStageForTest()

	return []bson.M{
		matchExpr,
		buildAddFieldsDateStage(TestStatsDateKey, DbTestStatsIdDateKeyFull, filter.AfterDate, filter.BeforeDate, filter.GroupNumDays),
		{"$group": bson.M{
			"_id":                 buildGroupId(filter.GroupBy),
			TestStatsNumPassKey:   bson.M{"$sum": "$" + dbTestStatsNumPassKey},
			TestStatsNumFailKey:   bson.M{"$sum": "$" + dbTestStatsNumFailKey},
			"total_duration_pass": bson.M{"$sum": bson.M{"$multiply": Array{"$" + dbTestStatsNumPassKey, "$" + dbTestStatsAvgDurationPassKey}}},
		}},
		{"$project": bson.M{
			TestStatsTestFileKey:     "$_id." + TestStatsTestFileKey,
			TestStatsTaskNameKey:     "$_id." + TestStatsTaskNameKey,
			TestStatsBuildVariantKey: "$_id." + TestStatsBuildVariantKey,
			TestStatsDistroKey:       "$_id." + TestStatsDistroKey,
			TestStatsDateKey:         "$_id." + TestStatsDateKey,
			TestStatsNumPassKey:      1,
			TestStatsNumFailKey:      1,
			TestStatsAvgDurationPassKey: bson.M{"$cond": bson.M{"if": bson.M{"$ne": Array{"$" + TestStatsNumPassKey, 0}},
				"then": bson.M{"$divide": Array{"$total_duration_pass", "$" + TestStatsNumPassKey}},
				"else": nil}},
		}},
		{"$sort": bson.D{
			{Key: TestStatsDateKey, Value: SortDateOrder(filter.Sort)},
			{Key: TestStatsBuildVariantKey, Value: 1},
			{Key: TestStatsTaskNameKey, Value: 1},
			{Key: TestStatsTestFileKey, Value: 1},
			{Key: TestStatsDistroKey, Value: 1},
		}},
		{"$limit": filter.Limit},
	}
}

// buildMatchStageForTest builds the match stage of the test query pipeline based on the filter options.
func (filter StatsFilter) buildMatchStageForTest() bson.M {
	match := bson.M{
		DbTestStatsIdDateKeyFull: bson.M{
			"$gte": filter.AfterDate,
			"$lt":  filter.BeforeDate,
		},
		DbTestStatsIdProjectKeyFull:   filter.Project,
		DbTestStatsIdRequesterKeyFull: bson.M{"$in": filter.Requesters},
	}
	if len(filter.Tests) > 0 {
		match[DbTestStatsIdTestFileKeyFull] = BuildMatchArrayExpression(filter.Tests)
	}
	if len(filter.Tasks) > 0 {
		match[DbTestStatsIdTaskNameKeyFull] = BuildMatchArrayExpression(filter.Tasks)
	}
	if len(filter.BuildVariants) > 0 {
		match[DbTestStatsIdBuildVariantKeyFull] = BuildMatchArrayExpression(filter.BuildVariants)
	}
	if len(filter.Distros) > 0 {
		match[DbTestStatsIdDistroKeyFull] = BuildMatchArrayExpression(filter.Distros)
	}

	if filter.StartAt != nil {
		match["$or"] = filter.buildTestPaginationOrBranches()
	}

	return bson.M{"$match": match}
}

// buildAddFieldsDateStage builds the $addFields stage that sets the start date of the grouped
// period the stats document belongs in.
func buildAddFieldsDateStage(fieldName string, inputDateFieldName string, start time.Time, end time.Time, numDays int) bson.M {
	inputDateFieldRef := "$" + inputDateFieldName
	if numDays <= 1 {
		return bson.M{"$addFields": bson.M{fieldName: inputDateFieldRef}}
	}
	boundaries := dateBoundaries(start, end, numDays)
	branches := make([]bson.M, len(boundaries))
	for i := 0; i < len(boundaries)-1; i++ {
		branches[i] = bson.M{
			"case": bson.M{"$and": Array{
				bson.M{"$gte": Array{inputDateFieldRef, boundaries[i]}},
				bson.M{"$lt": Array{inputDateFieldRef, boundaries[i+1]}},
			}},
			"then": boundaries[i],
		}
	}
	lastIndex := len(boundaries) - 1
	branches[lastIndex] = bson.M{
		"case": bson.M{"$gte": Array{inputDateFieldRef, boundaries[lastIndex]}},
		"then": boundaries[lastIndex],
	}
	return bson.M{"$addFields": bson.M{fieldName: bson.M{"$switch": bson.M{"branches": branches}}}}
}

// buildGroupId builds the _id field for the $group stage corresponding to the GroupBy value.
func buildGroupId(groupBy GroupBy) bson.M {
	id := bson.M{TestStatsDateKey: "$" + TestStatsDateKey}
	switch groupBy {
	case GroupByDistro:
		id[TestStatsDistroKey] = "$" + DbTestStatsIdDistroKeyFull
		fallthrough
	case GroupByVariant:
		id[TestStatsBuildVariantKey] = "$" + DbTestStatsIdBuildVariantKeyFull
		fallthrough
	case GroupByTask:
		id[TestStatsTaskNameKey] = "$" + DbTestStatsIdTaskNameKeyFull
		fallthrough
	case GroupByTest:
		id[TestStatsTestFileKey] = "$" + DbTestStatsIdTestFileKeyFull
	}
	return id
}

// BuildMatchArrayExpression builds an expression to match any of the values in the array argument.
func BuildMatchArrayExpression(values []string) interface{} {
	if len(values) == 1 {
		return values[0]
	} else if len(values) > 1 {
		return bson.M{"$in": values}
	}
	return nil
}

// getNextDate returns the date of the grouping period following the one specified in startAt.
func (filter StatsFilter) getNextDate() time.Time {
	numDays := time.Duration(filter.GroupNumDays) * 24 * time.Hour
	if filter.Sort == SortLatestFirst {
		return filter.StartAt.Date.Add(-numDays)
	} else {
		return filter.StartAt.Date.Add(numDays)
	}
}

// buildTestPaginationOrBranches builds an expression for the conditions imposed by the filter StartAt field.
func (filter StatsFilter) buildTestPaginationOrBranches() []bson.M {
	var dateDescending = filter.Sort == SortLatestFirst
	var nextDate interface{}

	if filter.GroupNumDays > 1 {
		nextDate = filter.getNextDate()
	}

	var fields []paginationField

	switch filter.GroupBy {
	case GroupByTest:
		fields = []paginationField{
			{Field: DbTestStatsIdDateKeyFull, Descending: dateDescending, Strict: true, Value: filter.StartAt.Date, NextValue: nextDate},
			{Field: DbTestStatsIdTestFileKeyFull, Strict: false, Value: filter.StartAt.Test},
		}
	case GroupByTask:
		fields = []paginationField{
			{Field: DbTestStatsIdDateKeyFull, Descending: dateDescending, Strict: true, Value: filter.StartAt.Date, NextValue: nextDate},
			{Field: DbTestStatsIdTaskNameKeyFull, Strict: true, Value: filter.StartAt.Task},
			{Field: DbTestStatsIdTestFileKeyFull, Strict: false, Value: filter.StartAt.Test},
		}
	case GroupByVariant:
		fields = []paginationField{
			{Field: DbTestStatsIdDateKeyFull, Descending: dateDescending, Strict: true, Value: filter.StartAt.Date, NextValue: nextDate},
			{Field: DbTestStatsIdBuildVariantKeyFull, Strict: true, Value: filter.StartAt.BuildVariant},
			{Field: DbTestStatsIdTaskNameKeyFull, Strict: true, Value: filter.StartAt.Task},
			{Field: DbTestStatsIdTestFileKeyFull, Strict: false, Value: filter.StartAt.Test},
		}
	case GroupByDistro:
		fields = []paginationField{
			{Field: DbTestStatsIdDateKeyFull, Descending: dateDescending, Strict: true, Value: filter.StartAt.Date, NextValue: nextDate},
			{Field: DbTestStatsIdBuildVariantKeyFull, Strict: true, Value: filter.StartAt.BuildVariant},
			{Field: DbTestStatsIdTaskNameKeyFull, Strict: true, Value: filter.StartAt.Task},
			{Field: DbTestStatsIdTestFileKeyFull, Strict: true, Value: filter.StartAt.Test},
			{Field: DbTestStatsIdDistroKeyFull, Strict: false, Value: filter.StartAt.Distro},
		}
	}

	return buildPaginationOrBranches(fields)
}

// TaskStatsQueryPipeline creates an aggregation pipeline to query task statistics.
func (filter StatsFilter) BuildTaskStatsQueryGroupStage() bson.M {
	return bson.M{
		"$group": bson.M{
			"_id":                       buildGroupId(filter.GroupBy),
			TaskStatsNumSuccessKey:      bson.M{"$sum": "$" + DbTaskStatsNumSuccessKey},
			TaskStatsNumFailedKey:       bson.M{"$sum": "$" + DbTaskStatsNumFailedKey},
			TaskStatsNumTimeoutKey:      bson.M{"$sum": "$" + DbTaskStatsNumTimeoutKey},
			TaskStatsNumTestFailedKey:   bson.M{"$sum": "$" + DbTaskStatsNumTestFailedKey},
			TaskStatsNumSystemFailedKey: bson.M{"$sum": "$" + DbTaskStatsNumSystemFailedKey},
			TaskStatsNumSetupFailedKey:  bson.M{"$sum": "$" + DbTaskStatsNumSetupFailedKey},
			"total_duration_success":    bson.M{"$sum": bson.M{"$multiply": Array{"$" + DbTaskStatsNumSuccessKey, "$" + DbTaskStatsAvgDurationSuccessKey}}},
		}}
}

// BuildTaskStatsQuerySortStage creates an aggregation sort stage to query task statistics.
func (filter StatsFilter) BuildTaskStatsQuerySortStage() bson.M {
	return bson.M{"$sort": bson.D{
		{Key: TaskStatsDateKey, Value: SortDateOrder(filter.Sort)},
		{Key: TaskStatsBuildVariantKey, Value: 1},
		{Key: TaskStatsTaskNameKey, Value: 1},
		{Key: TaskStatsDistroKey, Value: 1},
	}}
}

// BuildTaskStatsQueryProjectStage creates an aggregation project stage to query task statistics.
func (filter StatsFilter) BuildTaskStatsQueryProjectStage() bson.M {
	return bson.M{"$project": bson.M{
		TaskStatsTaskNameKey:        "$" + DbTaskStatsIdTaskNameKeyFull,
		TaskStatsBuildVariantKey:    "$" + DbTaskStatsIdBuildVariantKeyFull,
		TaskStatsDistroKey:          "$" + DbTaskStatsIdDistroKeyFull,
		TaskStatsDateKey:            "$" + DbTaskStatsIdDateKeyFull,
		TaskStatsNumSuccessKey:      1,
		TaskStatsNumFailedKey:       1,
		TaskStatsNumTotalKey:        bson.M{"$add": Array{"$" + TaskStatsNumSuccessKey, "$" + TaskStatsNumFailedKey}},
		TaskStatsNumTimeoutKey:      1,
		TaskStatsNumTestFailedKey:   1,
		TaskStatsNumSystemFailedKey: 1,
		TaskStatsNumSetupFailedKey:  1,
		TaskStatsAvgDurationSuccessKey: bson.M{"$cond": bson.M{"if": bson.M{"$ne": Array{"$" + TaskStatsNumSuccessKey, 0}},
			"then": bson.M{"$divide": Array{"$total_duration_success", "$" + TaskStatsNumSuccessKey}},
			"else": nil}},
	}}
}

// TaskStatsQueryPipeline creates an aggregation pipeline to query task statistics.
func (filter StatsFilter) TaskStatsQueryPipeline() []bson.M {

	return []bson.M{
		filter.BuildMatchStageForTask(),
		buildAddFieldsDateStage("date", DbTaskStatsIdDateKeyFull, filter.AfterDate, filter.BeforeDate, filter.GroupNumDays),
		filter.BuildTaskStatsQueryGroupStage(),
		filter.BuildTaskStatsQueryProjectStage(),
		filter.BuildTaskStatsQuerySortStage(),
		{"$limit": filter.Limit},
	}
}

// BuildMatchStageForTask builds the match stage of the task query pipeline based on the filter options.
func (filter StatsFilter) BuildMatchStageForTask() bson.M {
	match := bson.M{
		DbTaskStatsIdDateKeyFull: bson.M{
			"$gte": filter.AfterDate,
			"$lt":  filter.BeforeDate,
		},
		DbTestStatsIdProjectKeyFull:   filter.Project,
		DbTestStatsIdRequesterKeyFull: bson.M{"$in": filter.Requesters},
	}
	if len(filter.Tasks) > 0 {
		match[DbTaskStatsIdTaskNameKeyFull] = BuildMatchArrayExpression(filter.Tasks)
	}
	if len(filter.BuildVariants) > 0 {
		match[DbTaskStatsIdBuildVariantKeyFull] = BuildMatchArrayExpression(filter.BuildVariants)
	}
	if len(filter.Distros) > 0 {
		match[DbTaskStatsIdDistroKeyFull] = BuildMatchArrayExpression(filter.Distros)
	}

	if filter.StartAt != nil {
		match["$or"] = filter.BuildTaskPaginationOrBranches()
	}

	return bson.M{"$match": match}
}

// BuildTaskPaginationOrBranches builds an expression for the conditions imposed by the filter StartAt field.
func (filter StatsFilter) BuildTaskPaginationOrBranches() []bson.M {
	var dateDescending = filter.Sort == SortLatestFirst
	var nextDate interface{}

	if filter.GroupNumDays > 1 {
		nextDate = filter.getNextDate()
	}

	var fields []paginationField

	switch filter.GroupBy {
	case GroupByTask:
		fields = []paginationField{
			{Field: DbTaskStatsIdDateKeyFull, Descending: dateDescending, Strict: true, Value: filter.StartAt.Date, NextValue: nextDate},
			{Field: DbTaskStatsIdTaskNameKeyFull, Strict: false, Value: filter.StartAt.Task},
		}
	case GroupByVariant:
		fields = []paginationField{
			{Field: DbTaskStatsIdDateKeyFull, Descending: dateDescending, Strict: true, Value: filter.StartAt.Date, NextValue: nextDate},
			{Field: DbTaskStatsIdBuildVariantKeyFull, Strict: true, Value: filter.StartAt.BuildVariant},
			{Field: DbTaskStatsIdTaskNameKeyFull, Strict: false, Value: filter.StartAt.Task},
		}
	case GroupByDistro:
		fields = []paginationField{
			{Field: DbTaskStatsIdDateKeyFull, Descending: dateDescending, Strict: true, Value: filter.StartAt.Date, NextValue: nextDate},
			{Field: DbTaskStatsIdBuildVariantKeyFull, Strict: true, Value: filter.StartAt.BuildVariant},
			{Field: DbTaskStatsIdTaskNameKeyFull, Strict: true, Value: filter.StartAt.Task},
			{Field: DbTaskStatsIdDistroKeyFull, Strict: false, Value: filter.StartAt.Distro},
		}
	}

	return buildPaginationOrBranches(fields)
}

// buildPaginationOrBranches builds and returns the $or branches of the pagination constraints.
// fields is an array of field names, they must be in the same order as the sort order.
// operators is a list of MongoDB comparison operators ("$gte", "$gt", "$lte", "$lt") for the fields.
// values is a list of values for the fields.
func buildPaginationOrBranches(fields []paginationField) []bson.M {
	baseConstraints := bson.M{}
	branches := []bson.M{}

	for _, field := range fields {
		branch := bson.M{}
		for k, v := range baseConstraints {
			branch[k] = v
		}
		branch[field.Field] = field.getNextExpression()
		branches = append(branches, branch)
		baseConstraints[field.Field] = field.getEqExpression()
	}
	return branches
}

// dateBoundaries returns the date boundaries when splitting the period between 'start' and 'end' in groups of 'numDays' days.
// The boundaries are the start dates of the periods of 'numDays' (or less for the last period), starting with 'start'.
func dateBoundaries(start time.Time, end time.Time, numDays int) []time.Time {
	if numDays <= 0 {
		numDays = 1
	}

	start = util.GetUTCDay(start)
	end = util.GetUTCDay(end)
	duration := 24 * time.Hour * time.Duration(numDays)
	boundary := start
	boundaries := []time.Time{}

	for boundary.Before(end) {
		boundaries = append(boundaries, boundary)
		boundary = boundary.Add(duration)
	}
	return boundaries
}

// SortDateOrder returns the sort order specification (1, -1) for the date field corresponding to the Sort value.
func SortDateOrder(sort Sort) int {
	if sort == SortLatestFirst {
		return -1
	} else {
		return 1
	}
}

// paginationField represents a statistics document field that is used to determine where to resume during pagination.
type paginationField struct {
	Field      string
	Descending bool
	Strict     bool
	Value      interface{}
	NextValue  interface{}
}

// getEqExpression returns an expression that can be used to match the documents which have the same field value or
// are in the same range as this paginationField.
func (pf paginationField) getEqExpression() interface{} {
	if pf.NextValue == nil {
		return pf.Value
	}
	if pf.Descending {
		return bson.M{
			"$lte": pf.Value,
			"$gt":  pf.NextValue,
		}
	} else {
		return bson.M{
			"$gte": pf.Value,
			"$lt":  pf.NextValue,
		}
	}
}

// getNextExpression returns an expression that can be used to match the documents which have a field value
// greater or smaller than the this paginationField.
func (pf paginationField) getNextExpression() bson.M {
	var operator string
	var value interface{}
	var strict bool

	if pf.NextValue != nil {
		value = pf.NextValue
		strict = false
	} else {
		value = pf.Value
		strict = pf.Strict
	}

	if pf.Descending {
		if strict {
			operator = "$lt"
		} else {
			operator = "$lte"
		}
	} else {
		if strict {
			operator = "$gt"
		} else {
			operator = "$gte"
		}
	}
	return bson.M{operator: value}
}

//////////////////////////////////////////////////////////////////
// Internal helpers for writing documents, running aggregations //
//////////////////////////////////////////////////////////////////

// aggregateIntoCollection runs an aggregation pipeline on a collection and bulk upserts all the documents
// into the target collection.
func aggregateIntoCollection(collection string, pipeline []bson.M, outputCollection string) error {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()

	opts := adb.BufferedWriteOptions{
		DB:         env.Settings().Database.DB,
		Collection: outputCollection,
		Count:      bulkSize,
		Duration:   10 * time.Second,
	}

	writer, err := adb.NewBufferedUpsertByID(ctx, env.Session().DB(opts.DB), opts)
	if err != nil {
		return errors.Wrap(err, "Failed to initialize document writer")
	}

	cursor, err := env.DB().Collection(collection, options.Collection().SetReadPreference(readpref.SecondaryPreferred())).Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return errors.Wrap(err, "problem running aggregation")
	}

	for cursor.Next(ctx) {
		doc := make(bson.Raw, len(cursor.Current))
		copy(doc, cursor.Current)

		if err = writer.Append(doc); err != nil {
			return errors.Wrap(err, "problem with bulk insert")
		}
	}

	if err = cursor.Err(); err != nil {
		return errors.Wrap(err, "problem running aggregation")
	}

	if err = cursor.Close(ctx); err != nil {
		return errors.Wrap(err, "problem closing cursor")
	}

	if err = writer.Close(); err != nil {
		return errors.Wrap(err, "problem flushing to new collection")
	}
	return nil
}

// makeSum is an internal function that creates a conditional $sum expression.
func makeSum(condition bson.M) bson.M {
	return bson.M{"$sum": bson.M{"$cond": bson.M{"if": condition, "then": 1, "else": 0}}}
}

///////////////////////////////////////////////////////////////////
// Functions to access pre-computed stats documents for testing. //
///////////////////////////////////////////////////////////////////

func GetDailyTestDoc(id DbTestStatsId) (*dbTestStats, error) {
	doc := dbTestStats{}
	err := db.FindOne(dailyTestStatsCollection, bson.M{"_id": id}, db.NoProjection, db.NoSort, &doc)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return &doc, err
}

func GetHourlyTestDoc(id DbTestStatsId) (*dbTestStats, error) {
	doc := dbTestStats{}
	err := db.FindOne(hourlyTestStatsCollection, bson.M{"_id": id}, db.NoProjection, db.NoSort, &doc)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return &doc, err
}

func GetDailyTaskDoc(id DbTaskStatsId) (*dbTaskStats, error) {
	doc := dbTaskStats{}
	err := db.FindOne(DailyTaskStatsCollection, bson.M{"_id": id}, db.NoProjection, db.NoSort, &doc)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return &doc, err
}
