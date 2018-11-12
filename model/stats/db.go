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
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/util"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	hourlyTestStatsCollection  = "hourly_test_stats"
	dailyTestStatsCollection   = "daily_test_stats"
	dailyTaskStatsCollection   = "daily_task_stats"
	dailyStatsStatusCollection = "daily_stats_status"
	bulkSize                   = 1000
	nsInASecond                = time.Second / time.Nanosecond
)

var (
	taskExecutionRef       = "$" + task.ExecutionKey
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
type array []interface{}

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

// hourlyTestStatsPipeline returns a pipeline aggregating task documents into hourly test stats.
func hourlyTestStatsPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time) []bson.M {
	return getHourlyTestStatsPipeline(projectId, requester, start, end, tasks, lastUpdate, false)
}

// hourlyTestStatsForOldTasksPipeline returns a pipeline aggregating old task documents into hourly test stats.
func hourlyTestStatsForOldTasksPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time) []bson.M {
	// Using the same pipeline as for the tasks collection as the base.
	basePipeline := getHourlyTestStatsPipeline(projectId, requester, start, end, tasks, lastUpdate, true)
	// And the merge the documents with the existing ones.
	mergePipeline := []bson.M{
		{"$lookup": bson.M{
			"from":         hourlyTestStatsCollection,
			"localField":   "_id",
			"foreignField": "_id",
			"as":           "existing",
		}},
		{"$unwind": bson.M{
			"path":                       "$existing",
			"preserveNullAndEmptyArrays": true,
		}},
		{"$project": bson.M{
			"_id":      1,
			"num_pass": bson.M{"$add": array{"$num_pass", "$existing.num_pass"}},
			"num_fail": bson.M{"$add": array{"$num_fail", "$existing.num_fail"}},
			"total_duration_pass": bson.M{"$add": array{
				bson.M{"$ifNull": array{bson.M{"$multiply": array{"$num_pass", "$avg_duration_pass"}}, 0}},
				bson.M{"$ifNull": array{bson.M{"$multiply": array{"$existing.num_pass", "$existing.avg_duration_pass"}}, 0}},
			}},
			"last_update": 1,
		}},
		{"$project": bson.M{
			"_id":      1,
			"num_pass": 1,
			"num_fail": 1,
			"avg_duration_pass": bson.M{"$cond": bson.M{"if": bson.M{"$ne": array{"$num_pass", 0}},
				"then": bson.M{"$divide": array{"$total_duration_pass", "$num_pass"}},
				"else": nil}},
			"last_update": 1,
		}},
	}
	return append(basePipeline, mergePipeline...)
}

// getHourlyTestStatsPipeline is an internal helper function to create a pipeline aggregating task
// documents into hourly test stats.
func getHourlyTestStatsPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time, oldTasks bool) []bson.M {
	var taskIdExpr string
	var displayTaskLookupCollection string
	if oldTasks {
		taskIdExpr = "$old_task_id"
		displayTaskLookupCollection = task.OldCollection
	} else {
		taskIdExpr = "$_id"
		displayTaskLookupCollection = task.Collection
	}
	pipeline := []bson.M{
		{"$match": bson.M{
			task.ProjectKey:     projectId,
			task.RequesterKey:   requester,
			task.CreateTimeKey:  bson.M{"$gte": start, "$lt": end},
			task.DisplayNameKey: bson.M{"$in": tasks},
		}},
		{"$project": bson.M{
			task.IdKey:  0,
			"task_id":   taskIdExpr,
			"execution": taskExecutionRef,
			"project":   taskProjectKeyRef,
			"task_name": taskDisplayNameKeyRef,
			"variant":   taskBuildVariantKeyRef,
			"distro":    taskDistroIdKeyRef,
			"requester": taskRequesterKeyRef}},
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
					{"$eq": array{testResultTaskIdKeyRef, "$$task_id"}},
					{"$eq": array{testResultExecutionRef, "$$execution"}}}}}},
				{"$project": bson.M{
					testresult.IDKey:        0,
					testresult.TestFileKey:  1,
					testresult.StatusKey:    1,
					testresult.StartTimeKey: 1,
					testresult.EndTimeKey:   1}}},
			"as": "testresults"}},
		{"$unwind": "$testresults"},
		{"$project": bson.M{
			"test_file": "$testresults." + testresult.TestFileKey,
			// We use the name of the display task if there is one.
			"task_name": bson.M{"$ifNull": array{"$display_task." + task.DisplayNameKey, "$task_name"}},
			"variant":   1,
			"distro":    1,
			"project":   1,
			"requester": 1,
			"status":    "$testresults." + task.StatusKey,
			"duration":  bson.M{"$subtract": array{"$testresults." + testresult.EndTimeKey, "$testresults." + testresult.StartTimeKey}}}},
		{"$group": bson.M{
			"_id": bson.D{
				{Name: "test_file", Value: "$test_file"},
				{Name: "task_name", Value: "$task_name"},
				{Name: "variant", Value: "$variant"},
				{Name: "distro", Value: "$distro"},
				{Name: "project", Value: "$project"},
				{Name: "requester", Value: "$requester"},
			},
			"num_pass": makeSum(bson.M{"$eq": array{"$status", "pass"}}),
			"num_fail": makeSum(bson.M{"$ne": array{"$status", "pass"}}),
			// "IGNORE" is not a special value, setting the value to something that is not a number will cause $avg to ignore it
			"avg_duration_pass": bson.M{"$avg": bson.M{"$cond": bson.M{"if": bson.M{"$eq": array{"$status", "pass"}}, "then": "$duration", "else": "IGNORE"}}}}},
		{"$addFields": bson.M{
			"_id.date":    start,
			"last_update": lastUpdate,
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
			"_id.project":   projectId,
			"_id.requester": requester,
			"_id.date":      bson.M{"$gte": start, "$lt": end},
			"_id.task_name": bson.M{"$in": tasks},
		}},
		{
			"$group": bson.M{
				"_id": bson.D{
					{Name: "test_file", Value: "$_id.test_file"},
					{Name: "task_name", Value: "$_id.task_name"},
					{Name: "variant", Value: "$_id.variant"},
					{Name: "distro", Value: "$_id.distro"},
					{Name: "project", Value: "$_id.project"},
					{Name: "requester", Value: "$_id.requester"},
				},
				"num_pass":            bson.M{"$sum": "$num_pass"},
				"num_fail":            bson.M{"$sum": "$num_fail"},
				"total_duration_pass": bson.M{"$sum": bson.M{"$multiply": array{"$num_pass", "$avg_duration_pass"}}},
			},
		},
		{
			"$project": bson.M{
				"_id":      1,
				"num_pass": 1,
				"num_fail": 1,
				"avg_duration_pass": bson.M{"$cond": bson.M{"if": bson.M{"$ne": array{"$num_pass", 0}},
					"then": bson.M{"$divide": array{"$total_duration_pass", "$num_pass"}},
					"else": nil}},
			},
		},
		{"$addFields": bson.M{
			"_id.date":    start,
			"last_update": lastUpdate,
		}},
	}
	return pipeline
}

//////////////////////
// Daily Task Stats //
//////////////////////

// dailyTaskStatsPipeline returns a pipeline aggregating task documents into daily task stats.
func dailyTaskStatsPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time) []bson.M {
	return getDailyTaskStatsPipeline(projectId, requester, start, end, tasks, lastUpdate, false)
}

// dailyTaskStatsForOldTasksPipeline returns a pipeline aggregating old task documents into daily task stats.
func dailyTaskStatsForOldTasksPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time) []bson.M {
	// Using the same pipeline as for the tasks collection as the base.
	basePipeline := getDailyTaskStatsPipeline(projectId, requester, start, end, tasks, lastUpdate, true)
	// And the merge the documents with the existing ones.
	mergePipeline := []bson.M{
		{"$lookup": bson.M{
			"from":         dailyTaskStatsCollection,
			"localField":   "_id",
			"foreignField": "_id",
			"as":           "existing",
		}},
		{"$unwind": bson.M{
			"path":                       "$existing",
			"preserveNullAndEmptyArrays": true,
		}},
		{"$project": bson.M{
			"_id":               1,
			"num_success":       bson.M{"$add": array{"$num_success", "$existing.num_success"}},
			"num_failed":        bson.M{"$add": array{"$num_failed", "$existing.num_failed"}},
			"num_test_failed":   bson.M{"$add": array{"$num_test_failed", "$existing.num_test_failed"}},
			"num_setup_failed":  bson.M{"$add": array{"$num_setup_failed", "$existing.num_setup_failed"}},
			"num_system_failed": bson.M{"$add": array{"$num_system_failed", "$existing.num_system_failed"}},
			"num_timeout":       bson.M{"$add": array{"$num_timeout", "$existing.num_timeout"}},
			"total_duration_success": bson.M{"$add": array{
				bson.M{"$ifNull": array{bson.M{"$multiply": array{"$num_success", "$avg_duration_success"}}, 0}},
				bson.M{"$ifNull": array{bson.M{"$multiply": array{"$existing.num_success", "$existing.avg_duration_success"}}, 0}},
			}},
			"last_update": 1,
		}},
		{"$project": bson.M{
			"_id":               1,
			"num_success":       1,
			"num_failed":        1,
			"num_test_failed":   1,
			"num_setup_failed":  1,
			"num_system_failed": 1,
			"num_timeout":       1,
			"avg_duration_success": bson.M{"$cond": bson.M{"if": bson.M{"$ne": array{"$num_success", 0}},
				"then": bson.M{"$divide": array{"$total_duration_success", "$num_success"}},
				"else": nil}},
			"last_update": 1,
		}},
	}
	return append(basePipeline, mergePipeline...)

}

// getDailyTaskStatsPipeline is an internal helper function to create a pipeline aggregating task
// documents into daily task stats.
func getDailyTaskStatsPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time, oldTasks bool) []bson.M {
	var taskIdExpr string
	var displayTaskLookupCollection string
	if oldTasks {
		taskIdExpr = "$old_task_id"
		displayTaskLookupCollection = task.OldCollection
	} else {
		taskIdExpr = "$_id"
		displayTaskLookupCollection = task.Collection
	}
	pipeline := []bson.M{
		{"$match": bson.M{
			task.ProjectKey:     projectId,
			task.RequesterKey:   requester,
			task.CreateTimeKey:  bson.M{"$gte": start, "$lt": end},
			task.DisplayNameKey: bson.M{"$in": tasks},
		}},
		{"$project": bson.M{
			task.IdKey:   0,
			"task_id":    taskIdExpr,
			"execution":  taskExecutionRef,
			"project":    taskProjectKeyRef,
			"task_name":  taskDisplayNameKeyRef,
			"variant":    taskBuildVariantKeyRef,
			"distro":     taskDistroIdKeyRef,
			"requester":  taskRequesterKeyRef,
			"status":     taskStatusKeyRef,
			"details":    taskDetailsKeyRef,
			"time_taken": bson.M{"$divide": array{taskTimeTakenKeyRef, nsInASecond}},
		}},
		{"$lookup": bson.M{
			"from":         displayTaskLookupCollection,
			"localField":   "task_id",
			"foreignField": task.ExecutionTasksKey,
			"as":           "display_task",
		}},
		{"$match": bson.M{"display_task": array{}}}, // Excluding the execution tasks
		{"$group": bson.M{
			"_id": bson.D{
				{Name: "task_name", Value: "$task_name"},
				{Name: "variant", Value: "$variant"},
				{Name: "distro", Value: "$distro"},
				{Name: "project", Value: "$project"},
				{Name: "requester", Value: "$requester"}},
			"num_success": makeSum(bson.M{"$eq": array{"$status", "success"}}),
			"num_failed":  makeSum(bson.M{"$eq": array{"$status", "failed"}}),
			"num_timeout": makeSum(bson.M{"$and": array{
				bson.M{"$eq": array{"$status", "failed"}},
				bson.M{"$eq": array{"$details.timed_out", true}}}}),
			"num_test_failed": makeSum(bson.M{"$and": array{
				bson.M{"$eq": array{"$status", "failed"}},
				bson.M{"$eq": array{"$details.type", "test"}},
				bson.M{"$ne": array{"$details.timed_out", true}}}}),
			"num_system_failed": makeSum(bson.M{"$and": array{
				bson.M{"$eq": array{"$status", "failed"}},
				bson.M{"$eq": array{"$details.type", "system"}},
				bson.M{"$ne": array{"$details.timed_out", true}}}}),
			"num_setup_failed": makeSum(bson.M{"$and": array{
				bson.M{"$eq": array{"$status", "failed"}},
				bson.M{"$eq": array{"$details.type", "setup"}},
				bson.M{"$ne": array{"$details.timed_out", true}}}}),
			"avg_duration_success": bson.M{"$avg": bson.M{"$cond": bson.M{"if": bson.M{"$eq": array{"$status", "success"}},
				"then": "$time_taken", "else": "IGNORE"}}}}},
		{"$addFields": bson.M{
			"_id.date":    start,
			"last_update": lastUpdate,
		}},
	}
	return pipeline
}

// statsToUpdatePipeline returns a pipeline aggregating task documents into documents describing tasks for which
// the stats need to be updated.
func statsToUpdatePipeline(projectId string, start time.Time, end time.Time) []bson.M {
	pipeline := []bson.M{
		{"$match": bson.M{
			task.ProjectKey:    projectId,
			task.FinishTimeKey: bson.M{"$gte": start, "$lt": end},
		}},
		{"$project": bson.M{
			task.IdKey:  0,
			"project":   taskProjectKeyRef,
			"requester": taskRequesterKeyRef,
			"date":      bson.M{"$dateToString": bson.M{"date": taskCreateTimeKeyRef, "format": "%Y-%m-%d %H"}},
			"day":       bson.M{"$dateToString": bson.M{"date": taskCreateTimeKeyRef, "format": "%Y-%m-%d"}},
			"task_name": taskDisplayNameKeyRef,
		}},
		{"$group": bson.M{
			"_id": bson.M{
				"project":   "$project",
				"requester": "$requester",
				"date":      "$date",
				"day":       "$day",
			},
			"task_names": bson.M{"$addToSet": "$task_name"},
		}},
		{"$project": bson.M{
			"_id":        0,
			"project":    "$_id.project",
			"requester":  "$_id.requester",
			"date":       bson.M{"$dateFromString": bson.M{"dateString": "$_id.date", "format": "%Y-%m-%d %H"}},
			"day":        bson.M{"$dateFromString": bson.M{"dateString": "$_id.day", "format": "%Y-%m-%d"}},
			"task_names": 1,
		}},
		{"$sort": bson.D{
			{Name: "project", Value: 1},
			{Name: "date", Value: 1},
			{Name: "requester", Value: 1},
		}},
	}
	return pipeline
}

///////////////////////////////////////////
// Queries on the precomputed statistics //
///////////////////////////////////////////

// testStatsQueryPipeline creates an aggregation pipeline to query test statistics.
func testStatsQueryPipeline(filter *StatsFilter) []bson.M {
	matchExpr := buildMatchStageForTest(filter)

	return []bson.M{
		matchExpr,
		buildAddFieldsDateStage("date", filter.AfterDate, filter.BeforeDate, filter.GroupNumDays),
		{"$group": bson.M{
			"_id":                 buildGroupId(filter.GroupBy),
			"num_pass":            bson.M{"$sum": "$num_pass"},
			"num_fail":            bson.M{"$sum": "$num_fail"},
			"total_duration_pass": bson.M{"$sum": bson.M{"$multiply": array{"$num_pass", "$avg_duration_pass"}}},
		}},
		{"$project": bson.M{
			"test_file": "$_id.test_file",
			"task_name": "$_id.task_name",
			"variant":   "$_id.variant",
			"distro":    "$_id.distro",
			"date":      "$_id.date",
			"num_pass":  1,
			"num_fail":  1,
			"avg_duration_pass": bson.M{"$cond": bson.M{"if": bson.M{"$ne": array{"$num_pass", 0}},
				"then": bson.M{"$divide": array{"$total_duration_pass", "$num_pass"}},
				"else": nil}},
		}},
		{"$sort": bson.D{
			{Name: "date", Value: sortDateOrder(filter.Sort)},
			{Name: "variant", Value: 1},
			{Name: "task_name", Value: 1},
			{Name: "test_file", Value: 1},
			{Name: "distro", Value: 1},
		}},
		{"$limit": filter.Limit},
	}
}

// buildMatchStageForTest builds the match stage of the test query pipeline based on the filter options.
func buildMatchStageForTest(filter *StatsFilter) bson.M {
	match := bson.M{
		"_id.date": bson.M{
			"$gte": filter.AfterDate,
			"$lt":  filter.BeforeDate,
		},
		"_id.project":   filter.Project,
		"_id.requester": bson.M{"$in": filter.Requesters},
	}
	if len(filter.Tests) > 0 {
		match["_id.test_file"] = buildMatchArrayExpression(filter.Tests)
	}
	if len(filter.Tasks) > 0 {
		match["_id.task_name"] = buildMatchArrayExpression(filter.Tasks)
	}
	if len(filter.BuildVariants) > 0 {
		match["_id.variant"] = buildMatchArrayExpression(filter.BuildVariants)
	}
	if len(filter.Distros) > 0 {
		match["_id.distro"] = buildMatchArrayExpression(filter.Distros)
	}

	if filter.StartAt != nil {
		match["$or"] = buildTestPaginationOrBranches(filter)
	}

	return bson.M{"$match": match}
}

// buildAddFieldsDateStage builds the $addFields stage that sets the start date of the grouped
// period the stats document belongs in.
func buildAddFieldsDateStage(fieldName string, start time.Time, end time.Time, numDays int) bson.M {
	if numDays <= 1 {
		return bson.M{"$addFields": bson.M{fieldName: "$_id.date"}}
	}
	boundaries := dateBoundaries(start, end, numDays)
	branches := make([]bson.M, len(boundaries))
	for i := 0; i < len(boundaries)-1; i++ {
		branches[i] = bson.M{
			"case": bson.M{"$and": array{
				bson.M{"$gte": array{"$_id.date", boundaries[i]}},
				bson.M{"$lt": array{"$_id.date", boundaries[i+1]}},
			}},
			"then": boundaries[i],
		}
	}
	lastIndex := len(boundaries) - 1
	branches[lastIndex] = bson.M{
		"case": bson.M{"$gte": array{"$_id.date", boundaries[lastIndex]}},
		"then": boundaries[lastIndex],
	}
	return bson.M{"$addFields": bson.M{fieldName: bson.M{"$switch": bson.M{"branches": branches}}}}
}

// buildGroupId builds the _id field for the $group stage corresponding to the GroupBy value.
func buildGroupId(groupBy GroupBy) bson.M {
	id := bson.M{"date": "$date"}
	switch groupBy {
	case GroupByDistro:
		id["distro"] = "$_id.distro"
		fallthrough
	case GroupByVariant:
		id["variant"] = "$_id.variant"
		fallthrough
	case GroupByTask:
		id["task_name"] = "$_id.task_name"
		fallthrough
	case GroupByTest:
		id["test_file"] = "$_id.test_file"
	}
	return id
}

// buildMatchArrayExpression builds an expression to match any of the values in the array argument.
func buildMatchArrayExpression(values []string) interface{} {
	if len(values) == 1 {
		return values[0]
	} else if len(values) > 1 {
		return bson.M{"$in": values}
	}
	return nil
}

// buildTestPaginationOrBranches builds an expression for the conditions imposed by the filter StartAt field.
func buildTestPaginationOrBranches(filter *StatsFilter) []bson.M {
	var dateOperator string
	if filter.Sort == SortLatestFirst {
		dateOperator = "$lt"
	} else {
		dateOperator = "$gt"
	}

	var fields []paginationField

	switch filter.GroupBy {
	case GroupByTest:
		fields = []paginationField{
			{Field: "_id.date", Operator: dateOperator, Value: filter.StartAt.Date},
			{Field: "_id.test_file", Operator: "$gt", Value: filter.StartAt.Test},
		}
	case GroupByTask:
		fields = []paginationField{
			{Field: "_id.date", Operator: dateOperator, Value: filter.StartAt.Date},
			{Field: "_id.task_name", Operator: "$gt", Value: filter.StartAt.Task},
			{Field: "_id.test_file", Operator: "$gt", Value: filter.StartAt.Test},
		}
	case GroupByVariant:
		fields = []paginationField{
			{Field: "_id.date", Operator: dateOperator, Value: filter.StartAt.Date},
			{Field: "_id.variant", Operator: "$gt", Value: filter.StartAt.BuildVariant},
			{Field: "_id.task_name", Operator: "$gt", Value: filter.StartAt.Task},
			{Field: "_id.test_file", Operator: "$gt", Value: filter.StartAt.Test},
		}
	case GroupByDistro:
		fields = []paginationField{
			{Field: "_id.date", Operator: dateOperator, Value: filter.StartAt.Date},
			{Field: "_id.variant", Operator: "$gt", Value: filter.StartAt.BuildVariant},
			{Field: "_id.task_name", Operator: "$gt", Value: filter.StartAt.Task},
			{Field: "_id.test_file", Operator: "$gt", Value: filter.StartAt.Test},
			{Field: "_id.distro", Operator: "$gt", Value: filter.StartAt.Distro},
		}
	}

	return buildPaginationOrBranches(fields)
}

// taskStatsQueryPipeline creates an aggregation pipeline to query task statistics.
func taskStatsQueryPipeline(filter *StatsFilter) []bson.M {
	matchExpr := buildMatchStageForTask(filter)

	return []bson.M{
		matchExpr,
		buildAddFieldsDateStage("date", filter.AfterDate, filter.BeforeDate, filter.GroupNumDays),
		{"$group": bson.M{
			"_id":                    buildGroupId(filter.GroupBy),
			"num_success":            bson.M{"$sum": "$num_success"},
			"num_failed":             bson.M{"$sum": "$num_failed"},
			"num_timeout":            bson.M{"$sum": "$num_timeout"},
			"num_test_failed":        bson.M{"$sum": "$num_test_failed"},
			"num_system_failed":      bson.M{"$sum": "$num_system_failed"},
			"num_setup_failed":       bson.M{"$sum": "$num_setup_failed"},
			"total_duration_success": bson.M{"$sum": bson.M{"$multiply": array{"$num_success", "$avg_duration_success"}}},
		}},
		{"$project": bson.M{
			"task_name":         "$_id.task_name",
			"variant":           "$_id.variant",
			"distro":            "$_id.distro",
			"date":              "$_id.date",
			"num_success":       1,
			"num_failed":        1,
			"num_total":         bson.M{"$add": array{"$num_success", "$num_failed"}},
			"num_timeout":       1,
			"num_test_failed":   1,
			"num_system_failed": 1,
			"num_setup_failed":  1,
			"avg_duration_success": bson.M{"$cond": bson.M{"if": bson.M{"$ne": array{"$num_success", 0}},
				"then": bson.M{"$divide": array{"$total_duration_success", "$num_success"}},
				"else": nil}},
		}},
		{"$sort": bson.D{
			{Name: "date", Value: sortDateOrder(filter.Sort)},
			{Name: "variant", Value: 1},
			{Name: "task_name", Value: 1},
			{Name: "distro", Value: 1},
		}},
		{"$limit": filter.Limit},
	}
}

// buildMatchStageForTask builds the match stage of the task query pipeline based on the filter options.
func buildMatchStageForTask(filter *StatsFilter) bson.M {
	match := bson.M{
		"_id.date": bson.M{
			"$gte": filter.AfterDate,
			"$lt":  filter.BeforeDate,
		},
		"_id.project":   filter.Project,
		"_id.requester": bson.M{"$in": filter.Requesters},
	}
	if len(filter.Tasks) > 0 {
		match["_id.task_name"] = buildMatchArrayExpression(filter.Tasks)
	}
	if len(filter.BuildVariants) > 0 {
		match["_id.variant"] = buildMatchArrayExpression(filter.BuildVariants)
	}
	if len(filter.Distros) > 0 {
		match["_id.distro"] = buildMatchArrayExpression(filter.Distros)
	}

	if filter.StartAt != nil {
		match["$or"] = buildTaskPaginationOrBranches(filter)
	}

	return bson.M{"$match": match}
}

// buildTaskPaginationOrBranches builds an expression for the conditions imposed by the filter StartAt field.
func buildTaskPaginationOrBranches(filter *StatsFilter) []bson.M {
	var dateOperator string
	if filter.Sort == SortLatestFirst {
		dateOperator = "$lt"
	} else {
		dateOperator = "$gt"
	}

	var fields []paginationField

	switch filter.GroupBy {
	case GroupByTask:
		fields = []paginationField{
			{Field: "_id.date", Operator: dateOperator, Value: filter.StartAt.Date},
			{Field: "_id.task_name", Operator: "$gt", Value: filter.StartAt.Task},
		}
	case GroupByVariant:
		fields = []paginationField{
			{Field: "_id.date", Operator: dateOperator, Value: filter.StartAt.Date},
			{Field: "_id.variant", Operator: "$gt", Value: filter.StartAt.BuildVariant},
			{Field: "_id.task_name", Operator: "$gt", Value: filter.StartAt.Task},
		}
	case GroupByDistro:
		fields = []paginationField{
			{Field: "_id.date", Operator: dateOperator, Value: filter.StartAt.Date},
			{Field: "_id.variant", Operator: "$gt", Value: filter.StartAt.BuildVariant},
			{Field: "_id.task_name", Operator: "$gt", Value: filter.StartAt.Task},
			{Field: "_id.distro", Operator: "$gt", Value: filter.StartAt.Distro},
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
		branch[field.Field] = bson.M{field.Operator: field.Value}
		branches = append(branches, branch)
		baseConstraints[field.Field] = field.Value
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
	duration := time.Duration(numDays * 24 * int(time.Hour))
	boundary := start
	boundaries := []time.Time{}

	for boundary.Before(end) {
		boundaries = append(boundaries, boundary)
		boundary = boundary.Add(duration)
	}
	return boundaries
}

// sortDateOrder returns the sort order specification (1, -1) for the date field corresponding to the Sort value.
func sortDateOrder(sort Sort) int {
	if sort == SortLatestFirst {
		return -1
	} else {
		return 1
	}
}

type paginationField struct {
	Field    string
	Operator string
	Value    interface{}
}

//////////////////////////////////////////////////////////////////
// Internal helpers for writing documents, running aggregations //
//////////////////////////////////////////////////////////////////

// aggregateWithCallback runs an aggregation pipeline on a collection and calls the provided callback for each output document.
func aggregateWithCallback(collection string, pipeline []bson.M, callback func(interface{}) error) error {
	session, database, err := db.GetGlobalSessionFactory().GetSession()
	if err != nil {
		return errors.Wrap(err, "Error establishing db connection")
	}
	defer session.Close()

	session.SetSocketTimeout(0)
	pipe := database.C(collection).Pipe(pipeline).AllowDiskUse()
	iter := pipe.Iter()
	for {
		raw := bson.RawD{}
		if iter.Next(&raw) {
			err = callback(raw)
			if err != nil {
				return errors.Wrap(err, "A callback call failed")
			}
		} else {
			err = iter.Err()
			if err != nil {
				return errors.Wrap(err, "Error during aggregation")
			}
			break
		}
	}
	return nil
}

// aggregateIntoCollection runs an aggregation pipeline on a collection and bulk upserts all the documents
// into the target collection.
func aggregateIntoCollection(collection string, pipeline []bson.M, outputCollection string) error {
	session, database, err := db.GetGlobalSessionFactory().GetSession()
	if err != nil {
		err = errors.Wrap(err, "Error establishing db connection")
		return err
	}
	defer session.Close()

	ctx := context.TODO()

	opts := adb.BufferedWriteOptions{
		DB:         database.Name,
		Collection: outputCollection,
		Count:      bulkSize,
		Duration:   10 * time.Second,
	}

	writer, err := adb.NewBufferedSessionUpsertByID(ctx, session, opts)
	if err != nil {
		return errors.Wrap(err, "Failed to initialize document writer")
	}
	err = aggregateWithCallback(collection, pipeline, writer.Append)
	if err != nil {
		return errors.Wrap(err, "Failed to aggregate with document writer callback")
	}
	err = writer.Close()
	if err != nil {
		return errors.Wrap(err, "Failed to flush document writer")
	}
	return nil
}

// makeSume is an internal function that creates a conditional $sum expression.
func makeSum(condition bson.M) bson.M {
	return bson.M{"$sum": bson.M{"$cond": bson.M{"if": condition, "then": 1, "else": 0}}}
}
