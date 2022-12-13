package stats

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	day1 = baseDay
	day2 = baseDay.Add(24 * time.Hour)
	day8 = baseDay.Add(7 * 24 * time.Hour)
)

type statsQuerySuite struct {
	baseTaskFilter StatsFilter

	suite.Suite
}

func TestStatsQuerySuite(t *testing.T) {
	suite.Run(t, new(statsQuerySuite))
}

func (s *statsQuerySuite) SetupTest() {
	s.clearCollection(DailyTaskStatsCollection)
	s.baseTaskFilter = StatsFilter{
		AfterDate:     day1,
		BeforeDate:    day8,
		GroupNumDays:  1,
		Project:       "p1",
		Requesters:    []string{"r1", "r2"},
		Tasks:         []string{"task1", "task2"},
		BuildVariants: []string{"v1", "v2"},
		Distros:       []string{"d1", "d2"},
		GroupBy:       GroupByDistro,
		Sort:          SortEarliestFirst,
		Limit:         MaxQueryLimit,
	}
}

func (s *statsQuerySuite) TestValidFilter() {
	require := s.Require()

	project := "p1"
	requesters := []string{"r1", "r2"}
	tasks := []string{"task1", "task2"}

	filter := StatsFilter{
		Project:      project,
		Requesters:   requesters,
		AfterDate:    day1,
		BeforeDate:   day2,
		Tasks:        tasks,
		GroupBy:      GroupByDistro,
		GroupNumDays: 1,
		Sort:         SortEarliestFirst,
		Limit:        MaxQueryLimit,
	}

	err := filter.ValidateForTasks()
	require.NoError(err)
}

func (s *statsQuerySuite) TestFilterInvalidDate() {
	require := s.Require()

	var filter StatsFilter
	var err error

	project := "p1"
	requesters := []string{"r1", "r2"}
	tasks := []string{"task1", "task2"}

	filter = StatsFilter{
		Project:      project,
		Requesters:   requesters,
		Tasks:        tasks,
		GroupBy:      GroupByDistro,
		GroupNumDays: 1,
		Sort:         SortEarliestFirst,
		Limit:        MaxQueryLimit,
	}

	// With AfterDate after BeforeDate.
	filter.AfterDate = day2
	filter.BeforeDate = day1
	err = filter.ValidateForTasks()
	require.Error(err)

	// With AfterDate equal to BeforeDate.
	filter.AfterDate = day1
	filter.BeforeDate = day1
	err = filter.ValidateForTasks()
	require.Error(err)

	// With AfterDate not a UTC day.
	filter.AfterDate = day1
	filter.BeforeDate = day1.Add(time.Hour)
	err = filter.ValidateForTasks()
	require.Error(err)

	// With BeforeDate not a UTC day.
	filter.AfterDate = day1
	filter.BeforeDate = day1.Add(time.Hour)
	err = filter.ValidateForTasks()
	require.Error(err)
}

func (s *statsQuerySuite) TestFilterInvalidForTasks() {
	require := s.Require()

	var filter StatsFilter
	var err error

	project := "p1"
	requesters := []string{"r1", "r2"}

	filter = StatsFilter{
		Project:      project,
		Requesters:   requesters,
		AfterDate:    day1,
		BeforeDate:   day2,
		Tasks:        nil,
		GroupBy:      GroupByDistro,
		GroupNumDays: 1,
		Sort:         SortEarliestFirst,
		Limit:        MaxQueryLimit,
	}
	err = filter.ValidateForTasks()
	require.Error(err)
}

func (s *statsQuerySuite) TestFilterInvalidGroupNumDays() {
	require := s.Require()

	var filter StatsFilter
	var err error

	project := "p1"
	requesters := []string{"r1", "r2"}
	tasks := []string{"task1", "task2"}

	// Invalid GroupNumDays.
	filter = StatsFilter{
		Project:      project,
		Requesters:   requesters,
		AfterDate:    day1,
		BeforeDate:   day2,
		Tasks:        tasks,
		GroupBy:      GroupByDistro,
		GroupNumDays: -1,
		Sort:         SortEarliestFirst,
		Limit:        MaxQueryLimit,
	}
	err = filter.ValidateForTasks()
	require.Error(err)
}

func (s *statsQuerySuite) TestFilterMissingRequesters() {
	require := s.Require()

	var filter StatsFilter
	var err error

	project := "p1"
	tasks := []string{"task1", "task2"}

	// Missing requesters.
	filter = StatsFilter{
		Project:      project,
		AfterDate:    day1,
		BeforeDate:   day2,
		Tasks:        tasks,
		GroupBy:      GroupByDistro,
		GroupNumDays: 1,
		Sort:         SortEarliestFirst,
		Limit:        MaxQueryLimit,
	}
	err = filter.ValidateForTasks()
	require.Error(err)
}

func (s *statsQuerySuite) TestFilterInvalidLimit() {
	require := s.Require()

	var filter StatsFilter
	var err error

	project := "p1"
	requesters := []string{"r1", "r2"}
	tasks := []string{"task1", "task2"}

	// Invalid Limit
	filter = StatsFilter{
		Project:      project,
		Requesters:   requesters,
		AfterDate:    day1,
		BeforeDate:   day2,
		Tasks:        tasks,
		GroupBy:      GroupByDistro,
		GroupNumDays: 1,
		Sort:         SortEarliestFirst,
		Limit:        -3,
	}
	err = filter.ValidateForTasks()
	require.Error(err)

	filter.Limit = MaxQueryLimit + 1
	err = filter.ValidateForTasks()
	require.Error(err)
}

func (s *statsQuerySuite) TestFilterInvalidSort() {
	require := s.Require()

	var filter StatsFilter
	var err error

	project := "p1"
	requesters := []string{"r1", "r2"}
	tasks := []string{"task1", "task2"}

	// Invalid Sort
	filter = StatsFilter{
		Project:      project,
		Requesters:   requesters,
		AfterDate:    day1,
		BeforeDate:   day2,
		Tasks:        tasks,
		GroupBy:      GroupByDistro,
		GroupNumDays: 1,
		Sort:         Sort("invalid"),
		Limit:        MaxQueryLimit,
	}
	err = filter.ValidateForTasks()
	require.Error(err)
}

func (s *statsQuerySuite) TestFilterInvalidGroupBy() {
	require := s.Require()

	var filter StatsFilter
	var err error

	project := "p1"
	requesters := []string{"r1", "r2"}
	tasks := []string{"task1", "task2"}

	// Invalid GroupBy
	filter = StatsFilter{
		Project:      project,
		Requesters:   requesters,
		AfterDate:    day1,
		BeforeDate:   day2,
		Tasks:        tasks,
		GroupBy:      GroupBy("invalid"),
		GroupNumDays: 1,
		Sort:         SortEarliestFirst,
		Limit:        MaxQueryLimit,
	}
	err = filter.ValidateForTasks()
	require.Error(err)
}

func (s *statsQuerySuite) TestGetTaskStatsEmptyCollection() {
	require := s.Require()

	docs, err := GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Empty(docs)
}

func (s *statsQuerySuite) TestGetTaskStatsOneDocument() {
	require := s.Require()

	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, 10.5)

	docs, err := GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 1)
	s.checkTaskStats(docs[0], "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, float64(10.5))
}

func (s *statsQuerySuite) TestGetTaskStatsTwoDocuments() {
	require := s.Require()

	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, 10.5)
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day2, 20, 7, 7, 0, 0, 0, 20.0)

	docs, err := GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, float64(10.5))
	s.checkTaskStats(docs[1], "task1", "v1", "d1", day2, 20, 7, 7, 0, 0, 0, float64(20))
}

func (s *statsQuerySuite) TestGetTaskStatsFilterScope() {
	require := s.Require()

	// Adding documents in filter scope
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, 10.5)
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day2, 20, 7, 7, 0, 0, 0, 20.0)
	// Adding documents out of filter scope, the returned stats should be the same
	s.insertDailyTaskStats("p3", "r1", "task1", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, 1)
	s.insertDailyTaskStats("p1", "r3", "task1", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, 1)
	s.insertDailyTaskStats("p1", "r1", "task3", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, 1)
	s.insertDailyTaskStats("p1", "r1", "task1", "v3", "d1", day1, 1, 1, 1, 0, 0, 0, 1)
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day8, 1, 1, 1, 0, 0, 0, 1)

	docs, err := GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, float64(10.5))
	s.checkTaskStats(docs[1], "task1", "v1", "d1", day2, 20, 7, 7, 0, 0, 0, float64(20))
}

func (s *statsQuerySuite) TestGetTaskStatsSortOrder() {
	require := s.Require()

	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, 10.5)
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day2, 20, 7, 7, 0, 0, 0, 20.0)

	s.baseTaskFilter.Sort = SortEarliestFirst

	docs, err := GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, float64(10.5))
	s.checkTaskStats(docs[1], "task1", "v1", "d1", day2, 20, 7, 7, 0, 0, 0, float64(20))

	s.baseTaskFilter.Sort = SortLatestFirst

	docs, err = GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task1", "v1", "d1", day2, 20, 7, 7, 0, 0, 0, float64(20))
	s.checkTaskStats(docs[1], "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, float64(10.5))
}

func (s *statsQuerySuite) TestGetTaskStatsGroupNumDays() {
	require := s.Require()

	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, 10.5)
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day2, 20, 7, 7, 0, 0, 0, 20.0)

	s.baseTaskFilter.GroupNumDays = 2
	docs, err := GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 1)
	s.checkTaskStats(docs[0], "task1", "v1", "d1", day1, 30, 12, 8, 1, 1, 2, float64(16.833333333333332))
}

func (s *statsQuerySuite) TestGetTaskStatsGroupByDistro() {
	require := s.Require()

	// Adding documents with same task+variant+distro, they should be grouped together in the results
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, 10.5)
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day2, 20, 7, 7, 0, 0, 0, 20.0)
	// Adding a document for task1+v1+d2 that should appear separately in the results
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d2", day1, 2, 3, 2, 1, 0, 0, 10)

	s.baseTaskFilter.GroupNumDays = 2
	s.baseTaskFilter.GroupBy = GroupByDistro

	docs, err := GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task1", "v1", "d1", day1, 30, 12, 8, 1, 1, 2, float64(16.833333333333332))
	s.checkTaskStats(docs[1], "task1", "v1", "d2", day1, 2, 3, 2, 1, 0, 0, float64(10))
}

func (s *statsQuerySuite) TestGetTaskStatsGroupByVariant() {
	require := s.Require()

	// Adding documents with same task+variant+distro, they should be grouped together in the results
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, 10.5)
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day2, 20, 7, 7, 0, 0, 0, 20.0)
	// Adding a document for task1+v1+d2 that should also be grouped with the previous stats in the results
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d2", day1, 2, 3, 2, 1, 0, 0, 10)
	// Adding a document for task1+v2+d2 that should appear separately in the results
	s.insertDailyTaskStats("p1", "r1", "task1", "v2", "d2", day1, 2, 3, 2, 1, 0, 0, 10)

	s.baseTaskFilter.GroupNumDays = 2
	s.baseTaskFilter.GroupBy = GroupByVariant

	docs, err := GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task1", "v1", "", day1, 32, 15, 10, 2, 1, 2, float64(16.40625))
	s.checkTaskStats(docs[1], "task1", "v2", "", day1, 2, 3, 2, 1, 0, 0, float64(10))
}

func (s *statsQuerySuite) TestGetTaskStatsGroupByTask() {
	require := s.Require()

	// Adding documents with same task+variant+distro, they should be grouped together in the results
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, 10.5)
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day2, 20, 7, 7, 0, 0, 0, 20.0)
	// Adding a document for task1+v1+d2 that should also be grouped with the previous stats in the results
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d2", day1, 2, 3, 2, 1, 0, 0, 10)
	// Adding a document for task1+v2+d2 that should also be grouped with the previous stats in the results
	s.insertDailyTaskStats("p1", "r1", "task1", "v2", "d2", day1, 2, 3, 2, 1, 0, 0, 10)
	// Adding a document for test1+task2+v2+d2 that should appear separately in the results
	s.insertDailyTaskStats("p1", "r1", "task2", "v2", "d2", day1, 2, 3, 2, 1, 0, 0, 10)

	s.baseTaskFilter.GroupNumDays = 2
	s.baseTaskFilter.GroupBy = GroupByTask

	docs, err := GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task1", "", "", day1, 34, 18, 12, 3, 1, 2, float64(16.029411764705884))
	s.checkTaskStats(docs[1], "task2", "", "", day1, 2, 3, 2, 1, 0, 0, float64(10))
}

func (s *statsQuerySuite) TestGetTaskStatsPagination() {
	require := s.Require()

	s.insertDailyTaskStats("p2", "r1", "task1", "v2", "d2", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task2", "v2", "d1", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task3", "v2", "d1", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task4", "v1", "d2", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task5", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task6", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task1", "v1", "d1", day2, 1, 1, 1, 0, 0, 0, 10)

	s.baseTaskFilter.Project = "p2"
	s.baseTaskFilter.Tasks = []string{"task1", "task2", "task3", "task4", "task5", "task6"}
	s.baseTaskFilter.Limit = 3

	docs, err := GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 3)
	// expecting the results ordered by date/variant/task/test/distro
	s.checkTaskStats(docs[0], "task4", "v1", "d2", day1, 1, 1, 1, 0, 0, 0, float64(10))
	s.checkTaskStats(docs[1], "task5", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, float64(10))
	s.checkTaskStats(docs[2], "task6", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, float64(10))

	s.baseTaskFilter.StartAt = &StartAt{Date: day1, Task: "task6", BuildVariant: "v1", Distro: "d1"}

	docs, err = GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 3)
	s.checkTaskStats(docs[0], "task6", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, float64(10))
	s.checkTaskStats(docs[1], "task1", "v2", "d2", day1, 1, 1, 1, 0, 0, 0, float64(10))
	s.checkTaskStats(docs[2], "task2", "v2", "d1", day1, 1, 1, 1, 0, 0, 0, float64(10))

	s.baseTaskFilter.StartAt = &StartAt{Date: day1, Task: "task2", BuildVariant: "v2", Distro: "d1"}

	docs, err = GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 3)
	s.checkTaskStats(docs[0], "task2", "v2", "d1", day1, 1, 1, 1, 0, 0, 0, float64(10))
	s.checkTaskStats(docs[1], "task3", "v2", "d1", day1, 1, 1, 1, 0, 0, 0, float64(10))
	s.checkTaskStats(docs[2], "task1", "v1", "d1", day2, 1, 1, 1, 0, 0, 0, float64(10))
}

func (s *statsQuerySuite) TestGetTaskStatsPaginationGroupByTask() {
	require := s.Require()

	s.insertDailyTaskStats("p2", "r1", "task1", "v2", "d2", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task2", "v2", "d1", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task3", "v2", "d1", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task4", "v1", "d2", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task5", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task6", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, 10)

	s.baseTaskFilter.Project = "p2"
	s.baseTaskFilter.Tasks = []string{"task1", "task2", "task3", "task4", "task5", "task6"}
	s.baseTaskFilter.Limit = 3
	s.baseTaskFilter.GroupBy = GroupByTask

	docs, err := GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 3)
	s.checkTaskStats(docs[0], "task1", "", "", day1, 1, 1, 1, 0, 0, 0, float64(10))
	s.checkTaskStats(docs[1], "task2", "", "", day1, 1, 1, 1, 0, 0, 0, float64(10))
	s.checkTaskStats(docs[2], "task3", "", "", day1, 1, 1, 1, 0, 0, 0, float64(10))

	s.baseTaskFilter.StartAt = &StartAt{Date: day1, Task: "task3"}

	docs, err = GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 3)
	s.checkTaskStats(docs[0], "task3", "", "", day1, 1, 1, 1, 0, 0, 0, float64(10))
	s.checkTaskStats(docs[1], "task4", "", "", day1, 1, 1, 1, 0, 0, 0, float64(10))
	s.checkTaskStats(docs[2], "task5", "", "", day1, 1, 1, 1, 0, 0, 0, float64(10))
}

func (s *statsQuerySuite) TestGetTaskStatsPaginationGroupNumDays() {
	require := s.Require()

	s.insertDailyTaskStats("p2", "r1", "task1", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, float64(10))
	s.insertDailyTaskStats("p2", "r1", "task2", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, float64(10))
	s.insertDailyTaskStats("p2", "r1", "task1", "v1", "d1", day2, 1, 1, 1, 0, 0, 0, float64(10))
	s.insertDailyTaskStats("p2", "r1", "task3", "v1", "d1", day2, 1, 1, 1, 0, 0, 0, float64(10))
	s.insertDailyTaskStats("p2", "r1", "task4", "v1", "d1", day2, 1, 1, 1, 0, 0, 0, float64(10))
	s.insertDailyTaskStats("p2", "r1", "task1", "v1", "d1", day8, 1, 1, 1, 0, 0, 0, float64(10))
	s.insertDailyTaskStats("p2", "r1", "task2", "v1", "d1", day8, 1, 1, 1, 0, 0, 0, float64(10))

	s.baseTaskFilter.Project = "p2"
	s.baseTaskFilter.Tasks = []string{"task1", "task2", "task3", "task4"}
	s.baseTaskFilter.Limit = 3
	s.baseTaskFilter.BeforeDate = day8.Add(7 * 24 * time.Hour)
	s.baseTaskFilter.GroupNumDays = 7

	docs, err := GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 3)
	s.checkTaskStats(docs[0], "task1", "v1", "d1", day1, 2, 2, 2, 0, 0, 0, float64(10))
	s.checkTaskStats(docs[1], "task2", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, float64(10))
	s.checkTaskStats(docs[2], "task3", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, float64(10))

	s.baseTaskFilter.StartAt = &StartAt{Date: day1, Task: "task3", BuildVariant: "v1", Distro: "d1"}
	docs, err = GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 3)
	s.checkTaskStats(docs[0], "task3", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, float64(10))
	s.checkTaskStats(docs[1], "task4", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, float64(10))
	s.checkTaskStats(docs[2], "task1", "v1", "d1", day8, 1, 1, 1, 0, 0, 0, float64(10))

	s.baseTaskFilter.StartAt = &StartAt{Date: day8, Task: "task1", BuildVariant: "v1", Distro: "d1"}
	docs, err = GetTaskStats(s.baseTaskFilter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task1", "v1", "d1", day8, 1, 1, 1, 0, 0, 0, float64(10))
	s.checkTaskStats(docs[1], "task2", "v1", "d1", day8, 1, 1, 1, 0, 0, 0, float64(10))
}

func (s *statsQuerySuite) checkTaskStats(stats TaskStats, task, variant, distro string, date time.Time, numSuccess, numFailed, numTimeout, numTestFailed, numSystemFailed, numSetupFailed int, avgDuration float64) {
	require := s.Require()
	require.Equal(task, stats.TaskName)
	require.Equal(variant, stats.BuildVariant)
	require.Equal(distro, stats.Distro)
	require.WithinDuration(date, stats.Date, 0)
	require.Equal(numSuccess, stats.NumSuccess)
	require.Equal(numFailed, stats.NumFailed)
	require.Equal(numSuccess+numFailed, stats.NumTotal)
	require.Equal(numTestFailed, stats.NumTestFailed)
	require.Equal(numSystemFailed, stats.NumSystemFailed)
	require.Equal(numSetupFailed, stats.NumSetupFailed)
	require.Equal(avgDuration, stats.AvgDurationSuccess)
}

/////////////////////////////////////////
// Methods to initialize database data //
/////////////////////////////////////////

func (s *statsQuerySuite) clearCollection(name string) {
	err := db.Clear(name)
	s.Require().NoError(err)
}

func (s *statsQuerySuite) insertDailyTaskStats(project string, requester string, taskName string, variant string, distro string, date time.Time, numSuccess, numFailed, numTimeout, numTestFailed, numSystemFailed, numSetupFailed int, avgDuration float64) {

	err := db.Insert(DailyTaskStatsCollection, bson.M{
		"_id": DbTaskStatsId{
			Project:      project,
			Requester:    requester,
			TaskName:     taskName,
			BuildVariant: variant,
			Distro:       distro,
			Date:         date,
		},
		"num_success":          numSuccess,
		"num_failed":           numFailed,
		"num_timeout":          numTimeout,
		"num_test_failed":      numTestFailed,
		"num_system_failed":    numSystemFailed,
		"num_setup_failed":     numSetupFailed,
		"avg_duration_success": avgDuration,
	})
	s.Require().NoError(err)
}
