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
	"github.com/evergreen-ci/evergreen/model/task"
	"gopkg.in/mgo.v2"

	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	hourlyTestStatsCollection  = "hourly_test_stats"
	dailyTestStatsCollection   = "daily_test_stats"
	dailyTaskStatsCollection   = "daily_task_stats"
	tasksCollection            = "tasks"
	testResultsCollection      = "testresults"
	oldTasksCollection         = "old_tasks"
	dailyStatsStatusCollection = "daily_stats_status"
	bulkSize                   = 1000
	nsInASecond                = 1000 * 1000 * 1000
)

// Convenient type to use for arrays in pipeline definitions.
type array []interface{}

//////////////////
// Stats Status //
//////////////////

// Returns a query to find a stats status document by project id.
func statsStatusQuery(projectId string) bson.M {
	return bson.M{"_id": projectId}
}

///////////////////////
// Hourly Test Stats //
///////////////////////

// Returns a pipeline aggregating task documents into hourly test stats.
func hourlyTestStatsPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time) []bson.M {
	return getHourlyTestStatsPipeline(projectId, requester, start, end, tasks, lastUpdate, false)
}

// Returns a pipeline aggregating old task documents into hourly test stats.
func hourlyTestStatsForOldTasksPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time) []bson.M {
	// Using the same pipeline as for the tasks collection as the base.
	var basePipeline = getHourlyTestStatsPipeline(projectId, requester, start, end, tasks, lastUpdate, true)
	// And the merge the documents with the existing ones.
	var mergePipeline = []bson.M{
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

// Internal helper function to create a pipeline aggregating task documents into hourly test stats.
func getHourlyTestStatsPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time, oldTasks bool) []bson.M {
	var matchExpr = bson.M{
		task.CreateTimeKey: bson.M{"$gte": start, "$lt": end},
		task.ProjectKey:    projectId,
		task.RequesterKey:  requester,
	}
	if len(tasks) > 0 {
		matchExpr[task.DisplayNameKey] = bson.M{"$in": tasks}
	}
	var taskIdExpr string
	var displayTaskLookupCollection string
	if oldTasks {
		taskIdExpr = "$old_task_id"
		displayTaskLookupCollection = oldTasksCollection
	} else {
		taskIdExpr = "$_id"
		displayTaskLookupCollection = tasksCollection
	}
	var pipeline = []bson.M{
		{"$match": matchExpr},
		{"$project": bson.M{
			"_id":       0,
			"task_id":   taskIdExpr,
			"execution": 1,
			"project":   "$branch",
			"task_name": "$display_name",
			"variant":   "$build_variant",
			"distro":    1,
			"requester": "$r"}},
		{"$lookup": bson.M{
			"from":         displayTaskLookupCollection,
			"localField":   "task_id",
			"foreignField": "execution_tasks",
			"as":           "display_task"}},
		{"$unwind": bson.M{
			"path":                       "$display_task",
			"preserveNullAndEmptyArrays": true}},
		{"$lookup": bson.M{
			"from": testResultsCollection,
			"let":  bson.M{"task_id": "$task_id", "execution": "$execution"},
			"pipeline": []bson.M{
				{"$match": bson.M{"$expr": bson.M{"$and": []bson.M{
					{"$eq": array{"$task_id", "$$task_id"}},
					{"$eq": array{"$task_execution", "$$execution"}}}}}},
				{"$project": bson.M{"_id": 0, "test_file": 1, "status": 1, "start": 1, "end": 1}}},
			"as": "testresults"}},
		{"$unwind": "$testresults"},
		{"$project": bson.M{
			"test_file": "$testresults.test_file",
			"task_name": bson.M{"$ifNull": array{"$display_task.display_name", "$task_name"}},
			"variant":   1,
			"distro":    1,
			"project":   1,
			"requester": 1,
			"status":    "$testresults.status",
			"duration":  bson.M{"$subtract": array{"$testresults.end", "$testresults.start"}}}},
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

// Returns a pipeline aggregating hourly test stats into daily test stats.
func dailyTestStatsFromHourlyPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time) []bson.M {
	var matchExpr = bson.M{
		"_id.project":   projectId,
		"_id.requester": requester,
		"_id.date":      bson.M{"$gte": start, "$lt": end},
	}
	if len(tasks) > 0 {
		matchExpr["_id.task_name"] = bson.M{"$in": tasks}
	}
	var pipeline = []bson.M{
		{"$match": matchExpr},
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

// Returns a pipeline aggregating task documents into daily task stats.
func dailyTaskStatsPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time) []bson.M {
	return getDailyTaskStatsPipeline(projectId, requester, start, end, tasks, lastUpdate, false)
}

// Returns a pipeline aggregating old task documents into daily task stats.
func dailyTaskStatsForOldTasksPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time) []bson.M {
	// Using the same pipeline as for the tasks collection as the base.
	var basePipeline = getDailyTaskStatsPipeline(projectId, requester, start, end, tasks, lastUpdate, true)
	// And the merge the documents with the existing ones.
	var mergePipeline = []bson.M{
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

// Internal helper function to create a pipeline aggregating task documents into daily task stats.
func getDailyTaskStatsPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string, lastUpdate time.Time, oldTasks bool) []bson.M {
	var matchExpr = bson.M{
		task.CreateTimeKey: bson.M{"$gte": start, "$lt": end},
		task.ProjectKey:    projectId,
		task.RequesterKey:  requester,
	}
	if len(tasks) > 0 {
		matchExpr[task.DisplayNameKey] = bson.M{"$in": tasks}
	}
	var taskIdExpr string
	var displayTaskLookupCollection string
	if oldTasks {
		taskIdExpr = "$old_task_id"
		displayTaskLookupCollection = oldTasksCollection
	} else {
		taskIdExpr = "$_id"
		displayTaskLookupCollection = tasksCollection
	}
	var pipeline = []bson.M{
		{"$match": matchExpr},
		{"$project": bson.M{
			"_id":        1,
			"task_id":    taskIdExpr,
			"execution":  1,
			"project":    "$branch",
			"task_name":  "$display_name",
			"variant":    "$build_variant",
			"distro":     1,
			"requester":  "$r",
			"status":     1,
			"details":    1,
			"time_taken": bson.M{"$divide": array{"$time_taken", nsInASecond}},
		}},
		{"$lookup": bson.M{
			"from":         displayTaskLookupCollection,
			"localField":   "task_id",
			"foreignField": "execution_tasks",
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

// Returns a pipeline aggregating task documents into documents describing tasks for which
// the stats need to be updated.
func statsToUpdatePipeline(projectId string, start time.Time, end time.Time) []bson.M {
	var pipeline = []bson.M{
		{"$match": bson.M{
			"branch":      projectId,
			"finish_time": bson.M{"$gte": start, "$lt": end},
		}},
		{"$project": bson.M{
			"_id":       0,
			"project":   "$branch",
			"requester": "$r",
			"date":      bson.M{"$dateToString": bson.M{"date": "$create_time", "format": "%Y-%m-%d %H"}},
			"day":       bson.M{"$dateToString": bson.M{"date": "$create_time", "format": "%Y-%m-%d"}},
			"task_name": "$display_name",
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
		{"$sort": bson.M{
			"project":   1,
			"date":      1,
			"requester": 1,
		}},
	}
	return pipeline
}

//////////////////////////////////////////////////////////////////
// Internal helpers for writing documents, running aggregations //
//////////////////////////////////////////////////////////////////

// A struct that can be used to write to a collection in bulk (unordered).
type documentWriter struct {
	collection    *mgo.Collection
	bulk          *mgo.Bulk
	numOperations int
}

// Creates a new documentWriter that can write to the specified collection.
func newDocumentWriter(collectionName string) (*documentWriter, error) {
	_, database, err := db.GetGlobalSessionFactory().GetSession()
	if err != nil {
		err = errors.Wrap(err, "Error establishing db connection")
		return nil, err
	}
	collection := database.C(collectionName)
	bulk := collection.Bulk()
	bulk.Unordered()
	writer := documentWriter{
		collection:    collection,
		bulk:          bulk,
		numOperations: 0,
	}
	return &writer, nil
}

// Adds a document to write to the queue and flush if necessary.
func (d *documentWriter) write(doc bson.RawD) error {
	d.bulk.Upsert(bson.M{"_id": getDocId(doc)}, doc)
	d.numOperations += 1

	if d.numOperations >= bulkSize {
		err := d.flush()
		if err != nil {
			return err
		}
	}
	return nil
}

// Flushes all pending operations.
func (d *documentWriter) flush() error {
	if d.numOperations > 0 {
		_, err := d.bulk.Run()
		if err != nil {
			return errors.Wrap(err, "Failed to run bulk write")
		}
		d.numOperations = 0
		d.bulk = d.collection.Bulk()
	}
	return nil
}

// Flushes all pending operations and closes the document writer.
// The document writer should not be used after this method is called.
func (d *documentWriter) close() error {
	defer d.collection.Database.Session.Close()
	err := d.flush()
	if err != nil {
		return errors.Wrap(err, "Failed to flush writes")
	}
	return nil
}

// Runs an aggregation pipeline on a collection and calls the provided callback for each output document.
func aggregateWithCallback(collection string, pipeline []bson.M, callback func(bson.RawD) error) error {
	session, database, err := db.GetGlobalSessionFactory().GetSession()
	if err != nil {
		err = errors.Wrap(err, "Error establishing db connection")
		return err
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
				err = errors.Wrap(err, "Error during aggregation")
				return err
			}
			break
		}
	}
	return nil
}

// Runs an aggregation pipeline on a collection and bulk upserts all the documents into the target collection.
func aggregateIntoCollection(collection string, pipeline []bson.M, outputCollection string) error {
	writer, err := newDocumentWriter(outputCollection)
	if err != nil {
		return errors.Wrap(err, "Failed to initialize document writer")
	}
	err = aggregateWithCallback(collection, pipeline, writer.write)
	if err != nil {
		return errors.Wrap(err, "Failed to aggregate with document writer callback")
	}
	err = writer.close()
	if err != nil {
		return errors.Wrap(err, "Failed to close document writer")
	}
	return nil
}

// Internal function that extracts the _id field from a raw bson document.
func getDocId(rawDoc bson.RawD) interface{} {
	for _, raw := range rawDoc {
		if raw.Name == "_id" {
			return raw.Value
		}
	}
	return nil
}

// Internal function that creates a conditional $sum expression.
func makeSum(condition bson.M) bson.M {
	return bson.M{"$sum": bson.M{"$cond": bson.M{"if": condition, "then": 1, "else": 0}}}
}
