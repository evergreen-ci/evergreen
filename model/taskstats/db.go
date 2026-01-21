package taskstats

// This file provides database layer logic for pre-computed task execution
// statistics.
// The database schema is the following:
// *daily_stats_status*
// {
//   "_id": <Project Id (string)>,
//   "last_job_run": <Date of the last successful job run that updated the project stats (date)>
//   "processed_tasks_until": <Date before which finished tasks have been processed by a successful job (date)>
//   "runtime": <Duration (ns) of the last successful job run that updated the project stats>
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
	"math/rand"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const (
	DailyTaskStatsCollection   = "daily_task_stats"
	DailyStatsStatusCollection = "daily_stats_status"
	bulkSize                   = 1000
	nsInASecond                = time.Second / time.Nanosecond
)

var (
	// '$' references to the BSON fields of tasks.
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
)

// Convenient type to use for arrays in pipeline definitions.
type Array []any

//////////////////
// Stats Status //
//////////////////

// statsStatusQuery returns a query to find a stats status document by project id.
func statsStatusQuery(projectId string) bson.M {
	return bson.M{"_id": projectId}
}

//////////////////////
// Daily Task Stats //
//////////////////////

// DBTaskStatsID represents the _id field for daily_task_stats documents.
type DBTaskStatsID struct {
	TaskName     string    `bson:"task_name"`
	BuildVariant string    `bson:"variant"`
	Distro       string    `bson:"distro"`
	Project      string    `bson:"project"`
	Requester    string    `bson:"requester"`
	Date         time.Time `bson:"date"`
}

// DBTaskStats represents the daily_task_stats documents.
type DBTaskStats struct {
	Id                 DBTaskStatsID `bson:"_id"`
	NumSuccess         int           `bson:"num_success"`
	NumFailed          int           `bson:"num_failed"`
	NumTimeout         int           `bson:"num_timeout"`
	NumTestFailed      int           `bson:"num_test_failed"`
	NumSystemFailed    int           `bson:"num_system_failed"`
	NumSetupFailed     int           `bson:"num_setup_failed"`
	AvgDurationSuccess float64       `bson:"avg_duration_success"`
	LastUpdate         time.Time     `bson:"last_update"`
}

func (d *DBTaskStats) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(d) }
func (d *DBTaskStats) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, d) }

var (
	// BSON fields for the task stats ID struct.
	DBTaskStatsIDTaskNameKey     = bsonutil.MustHaveTag(DBTaskStatsID{}, "TaskName")
	DBTaskStatsIDBuildVariantKey = bsonutil.MustHaveTag(DBTaskStatsID{}, "BuildVariant")
	DBTaskStatsIDDistroKey       = bsonutil.MustHaveTag(DBTaskStatsID{}, "Distro")
	DBTaskStatsIDProjectKey      = bsonutil.MustHaveTag(DBTaskStatsID{}, "Project")
	DBTaskStatsIDRequesterKey    = bsonutil.MustHaveTag(DBTaskStatsID{}, "Requester")
	DBTaskStatsIDDateKey         = bsonutil.MustHaveTag(DBTaskStatsID{}, "Date")

	// BSON fields for the task stats struct.
	DBTaskStatsIDKey                 = bsonutil.MustHaveTag(DBTaskStats{}, "Id")
	DBTaskStatsNumSuccessKey         = bsonutil.MustHaveTag(DBTaskStats{}, "NumSuccess")
	DBTaskStatsNumFailedKey          = bsonutil.MustHaveTag(DBTaskStats{}, "NumFailed")
	DBTaskStatsNumTestFailedKey      = bsonutil.MustHaveTag(DBTaskStats{}, "NumTestFailed")
	DBTaskStatsNumSetupFailedKey     = bsonutil.MustHaveTag(DBTaskStats{}, "NumSetupFailed")
	DBTaskStatsNumSystemFailedKey    = bsonutil.MustHaveTag(DBTaskStats{}, "NumSystemFailed")
	DBTaskStatsNumTimeoutKey         = bsonutil.MustHaveTag(DBTaskStats{}, "NumTimeout")
	DBTaskStatsAvgDurationSuccessKey = bsonutil.MustHaveTag(DBTaskStats{}, "AvgDurationSuccess")
	DBTaskStatsLastUpdateKey         = bsonutil.MustHaveTag(DBTaskStats{}, "LastUpdate")

	// BSON dotted field names for task stats ID elements.
	DBTaskStatsIDTaskNameKeyFull     = bsonutil.GetDottedKeyName(DBTaskStatsIDKey, DBTaskStatsIDTaskNameKey)
	DBTaskStatsIDBuildVariantKeyFull = bsonutil.GetDottedKeyName(DBTaskStatsIDKey, DBTaskStatsIDBuildVariantKey)
	DBTaskStatsIDDistroKeyFull       = bsonutil.GetDottedKeyName(DBTaskStatsIDKey, DBTaskStatsIDDistroKey)
	DBTaskStatsIDProjectKeyFull      = bsonutil.GetDottedKeyName(DBTaskStatsIDKey, DBTaskStatsIDProjectKey)
	DBTaskStatsIDRequesterKeyFull    = bsonutil.GetDottedKeyName(DBTaskStatsIDKey, DBTaskStatsIDRequesterKey)
	DBTaskStatsIDDateKeyFull         = bsonutil.GetDottedKeyName(DBTaskStatsIDKey, DBTaskStatsIDDateKey)
)

// statsPipeline returns a pipeline aggregating task documents into daily task
// stats.
func statsPipeline(projectId string, requester string, start time.Time, end time.Time, tasks []string) []bson.M {
	return []bson.M{
		{"$match": bson.M{
			task.ProjectKey:     projectId,
			task.RequesterKey:   requester,
			task.CreateTimeKey:  bson.M{"$gte": start, "$lt": end},
			task.DisplayNameKey: bson.M{"$in": tasks},
			"$or": []bson.M{
				{task.DisplayTaskIdKey: bson.M{"$exists": false}},
				{task.DisplayTaskIdKey: ""},
			},
		}},
		{"$project": bson.M{
			task.IdKey:                   0,
			"execution":                  taskExecutionKeyRef,
			DBTaskStatsIDProjectKey:      taskProjectKeyRef,
			DBTaskStatsIDTaskNameKey:     taskDisplayNameKeyRef,
			DBTaskStatsIDBuildVariantKey: taskBuildVariantKeyRef,
			DBTaskStatsIDDistroKey:       taskDistroIdKeyRef,
			DBTaskStatsIDRequesterKey:    taskRequesterKeyRef,
			task.StatusKey:               1,
			task.DetailsKey:              1,
			"time_taken":                 bson.M{"$divide": Array{taskTimeTakenKeyRef, nsInASecond}},
		}},
		{"$group": bson.M{
			"_id": bson.D{
				{Key: DBTaskStatsIDTaskNameKey, Value: "$" + DBTaskStatsIDTaskNameKey},
				{Key: DBTaskStatsIDBuildVariantKey, Value: "$" + DBTaskStatsIDBuildVariantKey},
				{Key: DBTaskStatsIDDistroKey, Value: "$" + DBTaskStatsIDDistroKey},
				{Key: DBTaskStatsIDProjectKey, Value: "$" + DBTaskStatsIDProjectKey},
				{Key: DBTaskStatsIDRequesterKey, Value: "$" + DBTaskStatsIDRequesterKey}},
			DBTaskStatsNumSuccessKey: makeSum(bson.M{"$eq": Array{"$status", "success"}}),
			DBTaskStatsNumFailedKey:  makeSum(bson.M{"$eq": Array{"$status", "failed"}}),
			DBTaskStatsNumTimeoutKey: makeSum(bson.M{"$and": Array{
				bson.M{"$eq": Array{taskStatusKeyRef, "failed"}},
				bson.M{"$eq": Array{bsonutil.GetDottedKeyName(taskDetailsKeyRef, task.TaskEndDetailTimedOut), true}}}}),
			DBTaskStatsNumTestFailedKey: makeSum(bson.M{"$and": Array{
				bson.M{"$eq": Array{taskStatusKeyRef, "failed"}},
				bson.M{"$eq": Array{bsonutil.GetDottedKeyName(taskDetailsKeyRef, task.TaskEndDetailType), "test"}},
				bson.M{"$ne": Array{bsonutil.GetDottedKeyName(taskDetailsKeyRef, task.TaskEndDetailTimedOut), true}}}}),
			DBTaskStatsNumSystemFailedKey: makeSum(bson.M{"$and": Array{
				bson.M{"$eq": Array{taskStatusKeyRef, "failed"}},
				bson.M{"$eq": Array{bsonutil.GetDottedKeyName(taskDetailsKeyRef, task.TaskEndDetailType), evergreen.CommandTypeSystem}},
				bson.M{"$ne": Array{bsonutil.GetDottedKeyName(taskDetailsKeyRef, task.TaskEndDetailTimedOut), true}}}}),
			DBTaskStatsNumSetupFailedKey: makeSum(bson.M{"$and": Array{
				bson.M{"$eq": Array{taskStatusKeyRef, "failed"}},
				bson.M{"$eq": Array{bsonutil.GetDottedKeyName(taskDetailsKeyRef, task.TaskEndDetailType), evergreen.CommandTypeSetup}},
				bson.M{"$ne": Array{bsonutil.GetDottedKeyName(taskDetailsKeyRef, task.TaskEndDetailTimedOut), true}}}}),
			DBTaskStatsAvgDurationSuccessKey: bson.M{"$avg": bson.M{"$cond": bson.M{"if": bson.M{"$eq": Array{taskStatusKeyRef, "success"}},
				"then": "$time_taken", "else": "IGNORE"}}}}},
		{"$addFields": bson.M{
			"_id." + DBTaskStatsIDDateKey: start,
			DBTaskStatsLastUpdateKey:      time.Now(),
		}},
	}
}

// statsToUpdatePipeline returns a pipeline aggregating task documents into
// documents describing tasks for which the stats need to be updated.
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
			statsToUpdateRequesterKey: taskRequesterKeyRef,
			statsToUpdateDayKey:       bson.M{"$dateToString": bson.M{"date": taskCreateTimeKeyRef, "format": "%Y-%m-%d"}},
			"task_name":               taskDisplayNameKeyRef,
		}},
		{"$group": bson.M{
			"_id": bson.M{
				statsToUpdateRequesterKey: "$" + statsToUpdateRequesterKey,
				statsToUpdateDayKey:       "$" + statsToUpdateDayKey,
			},
			statsToUpdateTasksKey: bson.M{"$addToSet": "$task_name"},
		}},
		{"$project": bson.M{
			"_id":                     0,
			statsToUpdateRequesterKey: "$_id." + statsToUpdateRequesterKey,
			statsToUpdateDayKey:       bson.M{"$dateFromString": bson.M{"dateString": "$_id." + statsToUpdateDayKey, "format": "%Y-%m-%d"}},
			statsToUpdateTasksKey:     1,
		}},
	}
}

var (
	statsToUpdateRequesterKey = bsonutil.MustHaveTag(StatsToUpdate{}, "Requester")
	statsToUpdateDayKey       = bsonutil.MustHaveTag(StatsToUpdate{}, "Day")
	statsToUpdateTasksKey     = bsonutil.MustHaveTag(StatsToUpdate{}, "Tasks")
)

///////////////////////////////////////////
// Queries on the precomputed statistics //
///////////////////////////////////////////

var (
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

// buildGroupId builds the _id field for the $group stage corresponding to the
// GroupBy value.
func buildGroupId(groupBy GroupBy) bson.M {
	id := bson.M{TaskStatsDateKey: "$" + TaskStatsDateKey}
	switch groupBy {
	case GroupByDistro:
		id[TaskStatsDistroKey] = "$" + DBTaskStatsIDDistroKeyFull
		fallthrough
	case GroupByVariant:
		id[TaskStatsBuildVariantKey] = "$" + DBTaskStatsIDBuildVariantKeyFull
		fallthrough
	case GroupByTask:
		id[TaskStatsTaskNameKey] = "$" + DBTaskStatsIDTaskNameKeyFull
	}

	return id
}

// BuildMatchArrayExpression builds an expression to match any of the values in the array argument.
func BuildMatchArrayExpression(values []string) any {
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

// TaskStatsQueryPipeline creates an aggregation pipeline to query task statistics.
func (filter StatsFilter) BuildTaskStatsQueryGroupStage() bson.M {
	return bson.M{
		"$group": bson.M{
			"_id":                       buildGroupId(filter.GroupBy),
			TaskStatsNumSuccessKey:      bson.M{"$sum": "$" + DBTaskStatsNumSuccessKey},
			TaskStatsNumFailedKey:       bson.M{"$sum": "$" + DBTaskStatsNumFailedKey},
			TaskStatsNumTimeoutKey:      bson.M{"$sum": "$" + DBTaskStatsNumTimeoutKey},
			TaskStatsNumTestFailedKey:   bson.M{"$sum": "$" + DBTaskStatsNumTestFailedKey},
			TaskStatsNumSystemFailedKey: bson.M{"$sum": "$" + DBTaskStatsNumSystemFailedKey},
			TaskStatsNumSetupFailedKey:  bson.M{"$sum": "$" + DBTaskStatsNumSetupFailedKey},
			"total_duration_success":    bson.M{"$sum": bson.M{"$multiply": Array{"$" + DBTaskStatsNumSuccessKey, "$" + DBTaskStatsAvgDurationSuccessKey}}},
		}}
}

// buildTaskStatsQuerySortStage creates an aggregation sort stage to query task statistics.
func (filter StatsFilter) BuildTaskStatsQuerySortStage() bson.M {
	return bson.M{"$sort": bson.D{
		{Key: TaskStatsDateKey, Value: SortDateOrder(filter.Sort)},
		{Key: TaskStatsBuildVariantKey, Value: 1},
		{Key: TaskStatsTaskNameKey, Value: 1},
		{Key: TaskStatsDistroKey, Value: 1},
	}}
}

// buildTaskStatsQueryProjectStage creates an aggregation project stage to query task statistics.
func (filter StatsFilter) BuildTaskStatsQueryProjectStage() bson.M {
	return bson.M{"$project": bson.M{
		TaskStatsTaskNameKey:        "$" + DBTaskStatsIDTaskNameKeyFull,
		TaskStatsBuildVariantKey:    "$" + DBTaskStatsIDBuildVariantKeyFull,
		TaskStatsDistroKey:          "$" + DBTaskStatsIDDistroKeyFull,
		TaskStatsDateKey:            "$" + DBTaskStatsIDDateKeyFull,
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
		filter.buildMatchStageForTask(),
		buildAddFieldsDateStage("date", DBTaskStatsIDDateKeyFull, filter.AfterDate, filter.BeforeDate, filter.GroupNumDays),
		filter.BuildTaskStatsQueryGroupStage(),
		filter.BuildTaskStatsQueryProjectStage(),
		filter.BuildTaskStatsQuerySortStage(),
		{"$limit": filter.Limit},
	}
}

// BuildMatchStageForTask builds the match stage of the task query pipeline based on the filter options.
func (filter StatsFilter) buildMatchStageForTask() bson.M {
	match := bson.M{
		DBTaskStatsIDDateKeyFull: bson.M{
			"$gte": filter.AfterDate,
			"$lt":  filter.BeforeDate,
		},
		DBTaskStatsIDProjectKeyFull:   filter.Project,
		DBTaskStatsIDRequesterKeyFull: bson.M{"$in": filter.Requesters},
	}

	if len(filter.Tasks) > 0 {
		match[DBTaskStatsIDTaskNameKeyFull] = BuildMatchArrayExpression(filter.Tasks)
	}
	if len(filter.BuildVariants) > 0 {
		match[DBTaskStatsIDBuildVariantKeyFull] = BuildMatchArrayExpression(filter.BuildVariants)
	}
	if len(filter.Distros) > 0 {
		match[DBTaskStatsIDDistroKeyFull] = BuildMatchArrayExpression(filter.Distros)
	}

	if filter.StartAt != nil {
		match["$or"] = filter.buildTaskPaginationOrBranches()
	}

	return bson.M{"$match": match}
}

// BuildTaskPaginationOrBranches builds an expression for the conditions imposed by the filter StartAt field.
func (filter StatsFilter) buildTaskPaginationOrBranches() []bson.M {
	var dateDescending = filter.Sort == SortLatestFirst
	var nextDate any

	if filter.GroupNumDays > 1 {
		nextDate = filter.getNextDate()
	}

	var fields []PaginationField

	switch filter.GroupBy {
	case GroupByTask:
		fields = []PaginationField{
			{Field: DBTaskStatsIDDateKeyFull, Descending: dateDescending, Strict: true, Value: filter.StartAt.Date, NextValue: nextDate},
			{Field: DBTaskStatsIDTaskNameKeyFull, Strict: false, Value: filter.StartAt.Task},
		}
	case GroupByVariant:
		fields = []PaginationField{
			{Field: DBTaskStatsIDDateKeyFull, Descending: dateDescending, Strict: true, Value: filter.StartAt.Date, NextValue: nextDate},
			{Field: DBTaskStatsIDBuildVariantKeyFull, Strict: true, Value: filter.StartAt.BuildVariant},
			{Field: DBTaskStatsIDTaskNameKeyFull, Strict: false, Value: filter.StartAt.Task},
		}
	case GroupByDistro:
		fields = []PaginationField{
			{Field: DBTaskStatsIDDateKeyFull, Descending: dateDescending, Strict: true, Value: filter.StartAt.Date, NextValue: nextDate},
			{Field: DBTaskStatsIDBuildVariantKeyFull, Strict: true, Value: filter.StartAt.BuildVariant},
			{Field: DBTaskStatsIDTaskNameKeyFull, Strict: true, Value: filter.StartAt.Task},
			{Field: DBTaskStatsIDDistroKeyFull, Strict: false, Value: filter.StartAt.Distro},
		}
	}

	return BuildPaginationOrBranches(fields)
}

// BuildPaginationOrBranches builds and returns the $or branches of the pagination constraints.
// fields is an array of field names, they must be in the same order as the sort order.
// operators is a list of MongoDB comparison operators ("$gte", "$gt", "$lte", "$lt") for the fields.
// values is a list of values for the fields.
// func (filter StatsFilter) buildPaginationOrBranches(fields []PaginationField) []bson.M {
func BuildPaginationOrBranches(fields []PaginationField) []bson.M {
	baseConstraints := bson.M{}
	branches := []bson.M{}

	for _, field := range fields {
		branch := bson.M{}
		for k, v := range baseConstraints {
			branch[k] = v
		}
		branch[field.Field] = field.GetNextExpression()
		branches = append(branches, branch)
		baseConstraints[field.Field] = field.GetEqExpression()
	}
	return branches
}

// dateBoundaries returns the date boundaries when splitting the period between 'start' and 'end' in groups of 'numDays' days.
// The boundaries are the start dates of the periods of 'numDays' (or less for the last period), starting with 'start'.
func dateBoundaries(start time.Time, end time.Time, numDays int) []time.Time {
	if numDays <= 0 {
		numDays = 1
	}

	start = utility.GetUTCDay(start)
	end = utility.GetUTCDay(end)
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

// PaginationField represents a statistics document field that is used to determine where to resume during pagination.
type PaginationField struct {
	Field      string
	Descending bool
	Strict     bool
	Value      any
	NextValue  any
}

// GetEqExpression returns an expression that can be used to match the documents which have the same field value or
// are in the same range as this PaginationField.
func (pf PaginationField) GetEqExpression() any {
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

// GetNextExpression returns an expression that can be used to match the documents which have a field value
// greater or smaller than the this PaginationField.
func (pf PaginationField) GetNextExpression() bson.M {
	var operator string
	var value any
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
// into the target collection, using the given hint.
func aggregateIntoCollection(ctx context.Context, collection string, pipeline []bson.M, outputCollection string) error {
	env := evergreen.GetEnvironment()
	opts := options.Aggregate().SetAllowDiskUse(true)
	cursor, err := env.DB().Collection(collection, options.Collection().SetReadPreference(readpref.SecondaryPreferred())).Aggregate(ctx, pipeline, opts)
	if err != nil {
		return errors.Wrap(err, "running aggregation")
	}
	defer func() {
		grip.Error(cursor.Close(ctx))
	}()

	buf := make([]mongo.WriteModel, 0, bulkSize)
	for cursor.Next(ctx) {
		var doc *birch.Document
		doc, err = birch.DC.ReaderErr(birch.Reader(cursor.Current))
		if err != nil {
			return errors.WithStack(err)
		}
		buf = append(buf, mongo.NewReplaceOneModel().
			SetUpsert(true).
			SetFilter(birch.DC.Elements(birch.EC.SubDocument("_id", doc.Lookup("_id").MutableDocument()))).
			SetReplacement(doc.Copy()))

		if len(buf) >= bulkSize {
			if err = doBulkWrite(ctx, env, outputCollection, buf); err != nil {
				return errors.Wrapf(err, "bulk writing to collection '%s'", outputCollection)
			}
			buf = make([]mongo.WriteModel, 0, bulkSize)

			time.Sleep(time.Duration(rand.Int63n(int64(time.Second))))

		}
	}

	if err = cursor.Err(); err != nil {
		return errors.Wrap(err, "running aggregation")
	}

	if err = doBulkWrite(ctx, env, outputCollection, buf); err != nil {
		return errors.Wrapf(err, "bulk writing to collection '%s'", outputCollection)
	}

	return nil
}

func doBulkWrite(ctx context.Context, env evergreen.Environment, outputCollection string, buf []mongo.WriteModel) error {
	if len(buf) == 0 {
		return nil
	}

	res, err := env.DB().Collection(outputCollection).BulkWrite(ctx, buf)
	if err != nil {
		return errors.Wrapf(err, "bulk writing operation to collection '%s'", outputCollection)
	}

	totalModified := res.UpsertedCount + res.ModifiedCount + res.InsertedCount

	if totalModified != int64(len(buf)) {
		return errors.Errorf("materializing view: %d of %d", totalModified, len(buf))
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

func GetDailyTaskDoc(ctx context.Context, id DBTaskStatsID) (*DBTaskStats, error) {
	doc := DBTaskStats{}
	q := db.Query(bson.M{DBTaskStatsIDKey: id})
	err := db.FindOneQ(ctx, DailyTaskStatsCollection, q, &doc)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return &doc, err
}
