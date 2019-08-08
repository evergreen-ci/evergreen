package reliability

// This file provides database layer logic for task reliability statistics.
// See stats.db.go for more details.

import (
	"github.com/evergreen-ci/evergreen/model/stats"
	"go.mongodb.org/mongo-driver/bson"
)

// TaskReliabilityQueryPipeline creates an aggregation pipeline to query task statistics for reliability.
func (filter TaskReliabilityFilter) TaskReliabilityQueryPipeline() []bson.M {
	statsFilter := stats.StatsFilter{
		Project:       filter.Project,
		Requesters:    filter.Requesters,
		AfterDate:     filter.AfterDate,
		BeforeDate:    filter.BeforeDate,
		Tasks:         filter.Tasks,
		BuildVariants: filter.BuildVariants,
		Distros:       filter.Distros,
		GroupNumDays:  filter.GroupNumDays,
		GroupBy:       filter.GroupBy,
		StartAt:       filter.StartAt,
		Limit:         filter.Limit,
		Sort:          filter.Sort,
	}
	return statsFilter.TaskStatsQueryPipeline()
}
