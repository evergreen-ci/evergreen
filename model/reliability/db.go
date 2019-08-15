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
// See stats.db.go for details on the structure of the backing daily_task_stats collection.

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/evergreen/util"
	"go.mongodb.org/mongo-driver/bson"
)

// BuildMatchStageForTask builds the match stage of the task query pipeline based on the filter options.
func (filter TaskReliabilityFilter) BuildMatchStageForTask(boundaries []time.Time) bson.M {
	start := boundaries[0]
	end := boundaries[len(boundaries)-1]

	match := bson.M{
		stats.DbTaskStatsIdDateKeyFull: bson.M{
			"$lt":  start,
			"$gte": end,
		},
		stats.DbTestStatsIdProjectKeyFull:   filter.Project,
		stats.DbTestStatsIdRequesterKeyFull: bson.M{"$in": filter.Requesters},
	}
	if len(filter.Tasks) > 0 {
		match[stats.DbTaskStatsIdTaskNameKeyFull] = stats.BuildMatchArrayExpression(filter.Tasks)
	}
	if len(filter.BuildVariants) > 0 {
		match[stats.DbTaskStatsIdBuildVariantKeyFull] = stats.BuildMatchArrayExpression(filter.BuildVariants)
	}
	if len(filter.Distros) > 0 {
		match[stats.DbTaskStatsIdDistroKeyFull] = stats.BuildMatchArrayExpression(filter.Distros)
	}

	if filter.StartAt != nil {
		match["$or"] = filter.BuildTaskPaginationOrBranches()
	}

	return bson.M{"$match": match}
}

// dateBoundaries returns the date boundaries when splitting the period between 'start' and 'end' in groups of 'numDays' days.
// The boundaries are the start dates of the periods of 'numDays' (or less for the last period), starting with 'start'.
func dateBoundaries(start time.Time, end time.Time, numDays int) []time.Time {
	if numDays <= 0 {
		numDays = 1
	}

	start = util.GetUTCDay(start)
	end = util.GetUTCDay(end)

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

// buildDateStageGroupID builds the date of the grouped
// period the stats document belongs in.
func buildDateStageGroupID(fieldName string, inputDateFieldName string, start time.Time, end time.Time, numDays int) interface{} {
	inputDateFieldRef := "$" + inputDateFieldName
	if numDays <= 1 {
		return inputDateFieldRef
	}
	boundaries := dateBoundaries(start, end, numDays)
	branches := make([]bson.M, 0, len(boundaries)-1)

	for i := 0; i < len(boundaries)-1; i++ {
		branches = append(branches, bson.M{
			"case": bson.M{"$and": stats.Array{
				bson.M{"$lt": stats.Array{inputDateFieldRef, boundaries[i]}},
				bson.M{"$gte": stats.Array{inputDateFieldRef, boundaries[i+1]}},
			}},
			"then": boundaries[i+1],
		})
	}
	return bson.M{"$switch": bson.M{"branches": branches}}
}

// buildGroupID builds the _id field for the $group stage corresponding to the GroupBy value.
func (filter TaskReliabilityFilter) buildGroupID() bson.M {
	id := bson.M{stats.TestStatsDateKey: buildDateStageGroupID("date", stats.DbTaskStatsIdDateKeyFull, filter.AfterDate, filter.BeforeDate, filter.GroupNumDays)}
	switch filter.GroupBy {
	case stats.GroupByDistro:
		id[stats.TestStatsDistroKey] = "$" + stats.DbTestStatsIdDistroKeyFull
		fallthrough
	case stats.GroupByVariant:
		id[stats.TestStatsBuildVariantKey] = "$" + stats.DbTestStatsIdBuildVariantKeyFull
		fallthrough
	case stats.GroupByTask:
		id[stats.TestStatsTaskNameKey] = "$" + stats.DbTestStatsIdTaskNameKeyFull
		fallthrough
	case stats.GroupByTest:
		id[stats.TestStatsTestFileKey] = "$" + stats.DbTestStatsIdTestFileKeyFull
	}
	return id
}

// BuildTaskStatsQueryGroupStage creates an aggregation pipeline to query task statistics.
func (filter TaskReliabilityFilter) BuildTaskStatsQueryGroupStage() bson.M {
	return bson.M{
		"$group": bson.M{
			"_id":                             filter.buildGroupID(),
			stats.TaskStatsNumSuccessKey:      bson.M{"$sum": "$" + stats.DbTaskStatsNumSuccessKey},
			stats.TaskStatsNumFailedKey:       bson.M{"$sum": "$" + stats.DbTaskStatsNumFailedKey},
			stats.TaskStatsNumTimeoutKey:      bson.M{"$sum": "$" + stats.DbTaskStatsNumTimeoutKey},
			stats.TaskStatsNumTestFailedKey:   bson.M{"$sum": "$" + stats.DbTaskStatsNumTestFailedKey},
			stats.TaskStatsNumSystemFailedKey: bson.M{"$sum": "$" + stats.DbTaskStatsNumSystemFailedKey},
			stats.TaskStatsNumSetupFailedKey:  bson.M{"$sum": "$" + stats.DbTaskStatsNumSetupFailedKey},
			"total_duration_success":          bson.M{"$sum": bson.M{"$multiply": stats.Array{"$" + stats.DbTaskStatsNumSuccessKey, "$" + stats.DbTaskStatsAvgDurationSuccessKey}}},
		}}
}

// TaskReliabilityQueryPipeline creates an aggregation pipeline to query task statistics for reliability.
func (filter TaskReliabilityFilter) TaskReliabilityQueryPipeline() []bson.M {
	boundaries := dateBoundaries(filter.AfterDate, filter.BeforeDate, filter.GroupNumDays)

	return []bson.M{
		filter.BuildMatchStageForTask(boundaries),
		filter.BuildTaskStatsQueryGroupStage(),
		filter.BuildTaskStatsQueryProjectStage(),
		filter.BuildTaskStatsQuerySortStage(),
		{"$limit": filter.Limit},
	}
}
