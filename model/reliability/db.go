package reliability

// This file provides database layer logic for task reliability statistics.
// The Task Reliability aggregation is slightly different to the task_stats implementation.
// The differences being:
//   1. The date grouping starts from the before_date and walks backwards so that the date ranges
//      contain the same number of days. There is no 'remainder' which contains a different number of days.
//      For example, if before date is 2019-08-09, group_num_days is 10 and limit is 10, then
//      the results will contain dates from:
//         2019-08-09 - 2019-07-31
//         2019-07-31 - 2019-07-21
//         2019-07-21 - 2019-07-11
//         2019-07-11 - 2019-07-01
//         2019-07-01 - 2019-06-21
//         2019-06-21 - 2019-06-11
//      This approach ensures that all the score can be fairly compared as they are for the same number of days.
//   2. The date field is generated in the $group stage. It doesn't require an $addField pipeline stage.
//
// See taskstats.db.go for details on the structure of the backing daily_task_stats collection.

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/taskstats"
	"github.com/evergreen-ci/utility"
	"go.mongodb.org/mongo-driver/bson"
)

// DateBoundaries returns the date boundaries when splitting the period between 'start' and 'end' in groups of 'numDays' days.
// The boundaries are the start dates of the periods of 'numDays' (or less for the last period), starting with 'start'.
func (filter TaskReliabilityFilter) dateBoundaries() []time.Time {
	start := filter.AfterDate
	end := filter.BeforeDate
	numDays := filter.GroupNumDays

	if numDays <= 0 {
		numDays = 1
	}

	start = utility.GetUTCDay(start)
	end = utility.GetUTCDay(end)

	boundaries := []time.Time{}
	duration := 24 * time.Hour * time.Duration(numDays)
	duration = -1 * duration
	boundary := end.Add(24 * time.Hour)

	for boundary.After(start) {
		boundaries = append(boundaries, boundary)
		boundary = boundary.Add(duration)
	}
	boundaries = append(boundaries, boundary)
	return boundaries
}

// BuildTaskPaginationOrBranches builds an expression for the conditions imposed by the filter StartAt field.
func (filter TaskReliabilityFilter) buildTaskPaginationOrBranches() []bson.M {
	var dateDescending = filter.Sort == taskstats.SortLatestFirst
	var nextDate interface{}

	if filter.GroupNumDays > 1 {
		nextDate = filter.StartAt.Date
	}

	var fields []taskstats.PaginationField

	switch filter.GroupBy {
	case taskstats.GroupByTask:
		fields = []taskstats.PaginationField{
			{Field: taskstats.DbTaskStatsIdDateKeyFull, Descending: dateDescending, Strict: true, Value: filter.StartAt.Date, NextValue: nextDate},
			{Field: taskstats.DbTaskStatsIdTaskNameKeyFull, Strict: true, Value: filter.StartAt.Task},
		}
	case taskstats.GroupByVariant:
		fields = []taskstats.PaginationField{
			{Field: taskstats.DbTaskStatsIdDateKeyFull, Descending: dateDescending, Strict: true, Value: filter.StartAt.Date, NextValue: nextDate},
			{Field: taskstats.DbTaskStatsIdBuildVariantKeyFull, Strict: true, Value: filter.StartAt.BuildVariant},
			{Field: taskstats.DbTaskStatsIdTaskNameKeyFull, Strict: true, Value: filter.StartAt.Task},
		}
	case taskstats.GroupByDistro:
		fields = []taskstats.PaginationField{
			{Field: taskstats.DbTaskStatsIdDateKeyFull, Descending: dateDescending, Strict: true, Value: filter.StartAt.Date, NextValue: nextDate},
			{Field: taskstats.DbTaskStatsIdBuildVariantKeyFull, Strict: true, Value: filter.StartAt.BuildVariant},
			{Field: taskstats.DbTaskStatsIdTaskNameKeyFull, Strict: true, Value: filter.StartAt.Task},
			{Field: taskstats.DbTaskStatsIdDistroKeyFull, Strict: true, Value: filter.StartAt.Distro},
		}
	}

	return taskstats.BuildPaginationOrBranches(fields)
}

// BuildMatchStageForTask builds the match stage of the task query pipeline based on the filter options.
func (filter TaskReliabilityFilter) buildMatchStageForTask() bson.M {
	boundaries := filter.dateBoundaries()

	start := boundaries[0]
	end := boundaries[len(boundaries)-1]

	match := bson.M{
		taskstats.DbTaskStatsIdDateKeyFull: bson.M{
			"$lt":  start,
			"$gte": end,
		},
		taskstats.DbTaskStatsIdProjectKeyFull:   filter.Project,
		taskstats.DbTaskStatsIdRequesterKeyFull: bson.M{"$in": filter.Requesters},
	}
	if len(filter.Tasks) > 0 {
		match[taskstats.DbTaskStatsIdTaskNameKeyFull] = taskstats.BuildMatchArrayExpression(filter.Tasks)
	}
	if len(filter.BuildVariants) > 0 {
		match[taskstats.DbTaskStatsIdBuildVariantKeyFull] = taskstats.BuildMatchArrayExpression(filter.BuildVariants)
	}
	if len(filter.Distros) > 0 {
		match[taskstats.DbTaskStatsIdDistroKeyFull] = taskstats.BuildMatchArrayExpression(filter.Distros)
	}

	if filter.StartAt != nil {
		match["$or"] = filter.buildTaskPaginationOrBranches()
	}
	return bson.M{"$match": match}
}

// buildDateStageGroupID builds the date of the grouped
// period the stats document belongs in.
func (filter TaskReliabilityFilter) buildDateStageGroupID(fieldName string, inputDateFieldName string) interface{} {
	numDays := filter.GroupNumDays
	inputDateFieldRef := "$" + inputDateFieldName
	if numDays <= 1 {
		return inputDateFieldRef
	}
	boundaries := filter.dateBoundaries()
	branches := make([]bson.M, 0, len(boundaries)-1)

	for i := 0; i < len(boundaries)-1; i++ {
		branches = append(branches, bson.M{
			"case": bson.M{"$and": taskstats.Array{
				bson.M{"$lt": taskstats.Array{inputDateFieldRef, boundaries[i]}},
				bson.M{"$gte": taskstats.Array{inputDateFieldRef, boundaries[i+1]}},
			}},
			"then": boundaries[i+1],
		})
	}
	return bson.M{"$switch": bson.M{"branches": branches}}
}

// buildGroupID builds the _id field for the $group stage corresponding to the GroupBy value.
func (filter TaskReliabilityFilter) buildGroupID() bson.M {
	id := bson.M{taskstats.TaskStatsDateKey: filter.buildDateStageGroupID("date", taskstats.DbTaskStatsIdDateKeyFull)}
	switch filter.GroupBy {
	case taskstats.GroupByDistro:
		id[taskstats.TaskStatsDistroKey] = "$" + taskstats.DbTaskStatsIdDistroKeyFull
		fallthrough
	case taskstats.GroupByVariant:
		id[taskstats.TaskStatsBuildVariantKey] = "$" + taskstats.DbTaskStatsIdBuildVariantKeyFull
		fallthrough
	case taskstats.GroupByTask:
		id[taskstats.TaskStatsTaskNameKey] = "$" + taskstats.DbTaskStatsIdTaskNameKeyFull
	}
	return id
}

// BuildTaskStatsQueryGroupStage creates an aggregation pipeline to query task statistics.
func (filter TaskReliabilityFilter) BuildTaskStatsQueryGroupStage() bson.M {
	return bson.M{
		"$group": bson.M{
			"_id":                                 filter.buildGroupID(),
			taskstats.TaskStatsNumSuccessKey:      bson.M{"$sum": "$" + taskstats.DbTaskStatsNumSuccessKey},
			taskstats.TaskStatsNumFailedKey:       bson.M{"$sum": "$" + taskstats.DbTaskStatsNumFailedKey},
			taskstats.TaskStatsNumTimeoutKey:      bson.M{"$sum": "$" + taskstats.DbTaskStatsNumTimeoutKey},
			taskstats.TaskStatsNumTestFailedKey:   bson.M{"$sum": "$" + taskstats.DbTaskStatsNumTestFailedKey},
			taskstats.TaskStatsNumSystemFailedKey: bson.M{"$sum": "$" + taskstats.DbTaskStatsNumSystemFailedKey},
			taskstats.TaskStatsNumSetupFailedKey:  bson.M{"$sum": "$" + taskstats.DbTaskStatsNumSetupFailedKey},
			"total_duration_success":              bson.M{"$sum": bson.M{"$multiply": taskstats.Array{"$" + taskstats.DbTaskStatsNumSuccessKey, "$" + taskstats.DbTaskStatsAvgDurationSuccessKey}}},
		}}
}

// TaskReliabilityQueryPipeline creates an aggregation pipeline to query task statistics for reliability.
func (filter TaskReliabilityFilter) taskReliabilityQueryPipeline() []bson.M {
	return []bson.M{
		filter.buildMatchStageForTask(),
		filter.BuildTaskStatsQueryGroupStage(),
		filter.BuildTaskStatsQueryProjectStage(),
		filter.BuildTaskStatsQuerySortStage(),
		{"$limit": filter.Limit},
	}
}

// GetTaskStats create an aggregation to find task stats matching the filter state.
func (filter TaskReliabilityFilter) GetTaskStats() (taskStats []taskstats.TaskStats, err error) {
	pipeline := filter.taskReliabilityQueryPipeline()
	err = db.Aggregate(taskstats.DailyTaskStatsCollection, pipeline, &taskStats)
	return
}
