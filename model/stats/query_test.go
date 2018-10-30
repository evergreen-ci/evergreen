package stats

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type statsQuerySuite struct {
	suite.Suite
}

func TestStatsQuerySuite(t *testing.T) {
	suite.Run(t, new(statsQuerySuite))
}

func (s *statsQuerySuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *statsQuerySuite) SetupTest() {
	s.clearCollection(dailyTestStatsCollection)
	s.clearCollection(dailyTaskStatsCollection)
}

func (s *statsQuerySuite) TestStatsFilterConstructor() {
	require := s.Require()

	project := "p1"
	requesters := []string{"r1", "r2"}
	day1 := baseDay
	day2 := baseDay.Add(24 * time.Hour)
	tests := []string{"test1", "test2"}
	tasks := []string{"task1", "task2"}

	filter := NewDefaultStatsFilter(project, requesters, day1, day2, tests, tasks)

	require.Equal(project, filter.Project)
	require.Equal(requesters, filter.Requesters)
	require.Equal(day1, filter.AfterDate)
	require.Equal(day2, filter.BeforeDate)
	require.Equal(tests, filter.Tests)
	require.Equal(tasks, filter.Tasks)

	// Default values
	require.Nil(filter.BuildVariants)
	require.Nil(filter.Distros)
	require.Equal(1, filter.GroupNumDays)
	require.Equal(GroupByDistro, filter.GroupBy)
	require.Nil(filter.StartAt)
	require.Equal(MaxQueryLimit, filter.Limit)
	require.Equal(SortLatestFirst, filter.Sort)

	// Check that validate does not find any errors
	// For tests
	err := filter.validate(true)
	require.NoError(err)
	// For tasks
	filter.Tests = nil
	err = filter.validate(false)
	require.NoError(err)
}

func (s *statsQuerySuite) TestInvalidFilters() {
	require := s.Require()

	project := "p1"
	requesters := []string{"r1", "r2"}
	day1 := baseDay
	day2 := baseDay.Add(24 * time.Hour)
	tests := []string{"test1", "test2"}
	tasks := []string{"task1", "task2"}

	var filter StatsFilter
	var err error

	// Invalid dates
	filter = NewDefaultStatsFilter(project, requesters, day2, day1, tests, tasks)
	err = filter.validate(true)
	require.Error(err)

	filter = NewDefaultStatsFilter(project, requesters, day1, day1, tests, tasks)
	err = filter.validate(true)
	require.Error(err)

	filter = NewDefaultStatsFilter(project, requesters, day1, day1.Add(time.Hour), tests, tasks)
	err = filter.validate(true)
	require.Error(err)

	// Filter for tasks should not have Tests set.
	filter = NewDefaultStatsFilter(project, requesters, day1, day2, tests, tasks)
	err = filter.validate(false)
	require.Error(err)

	// Filter for tasks should have Tasks set.
	filter = NewDefaultStatsFilter(project, requesters, day1, day2, nil, nil)
	err = filter.validate(false)
	require.Error(err)

	// Filter for tests should have at least one of Tests or Tasks set.
	filter = NewDefaultStatsFilter(project, requesters, day1, day2, nil, nil)
	err = filter.validate(true)
	require.Error(err)

	// Invalid GroupNumDays
	filter = NewDefaultStatsFilter(project, requesters, day1, day2, tests, tasks)
	filter.GroupNumDays = -1
	err = filter.validate(true)
	require.Error(err)

	// Invalid Requesters
	filter = NewDefaultStatsFilter(project, nil, day1, day2, tests, tasks)
	err = filter.validate(true)
	require.Error(err)

	// Invalid Limit
	filter = NewDefaultStatsFilter(project, requesters, day1, day2, tests, tasks)
	filter.Limit = -3
	err = filter.validate(true)
	require.Error(err)

	filter.Limit = MaxQueryLimit + 1
	err = filter.validate(true)
	require.Error(err)

	// Invalid Sort
	filter = NewDefaultStatsFilter(project, requesters, day1, day2, tests, tasks)
	filter.Sort = Sort("invalid")
	err = filter.validate(true)
	require.Error(err)

	// Invalid GroupBy
	filter = NewDefaultStatsFilter(project, requesters, day1, day2, tests, tasks)
	filter.GroupBy = GroupBy("invalid")
	err = filter.validate(true)
	require.Error(err)
}

func (s *statsQuerySuite) TestGetTestStats() {
	require := s.Require()

	day1 := baseDay
	day2 := baseDay.Add(24 * time.Hour)
	day8 := baseDay.Add(7 * 24 * time.Hour)

	filter := StatsFilter{
		AfterDate:     day1,
		BeforeDate:    day8,
		GroupNumDays:  1,
		Project:       "p1",
		Requesters:    []string{"r1", "r2"},
		Tests:         []string{"test1", "test2"},
		Tasks:         []string{"task1", "task2"},
		BuildVariants: []string{"v1", "v2"},
		Distros:       []string{"d1", "d2"},
		GroupBy:       GroupByDistro,
		Sort:          SortEarliestFirst,
		Limit:         MaxQueryLimit,
	}

	// Query on empty collection
	docs, err := GetTestStats(&filter)
	require.NoError(err)
	require.Empty(docs)

	// Query on collection with 1 document
	s.insertDailyTestStats("p1", "r1", "test1", "task1", "v1", "d1", day1, 10, 2, 12.22)

	docs, err = GetTestStats(&filter)
	require.NoError(err)
	require.Len(docs, 1)
	s.checkTestStats(docs[0], "test1", "task1", "v1", "d1", day1, 10, 2, float32(12.22))

	// Query on collection with 2 documents
	s.insertDailyTestStats("p1", "r1", "test1", "task1", "v1", "d1", day2, 20, 7, 45.45)

	docs, err = GetTestStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTestStats(docs[0], "test1", "task1", "v1", "d1", day1, 10, 2, float32(12.22))
	s.checkTestStats(docs[1], "test1", "task1", "v1", "d1", day2, 20, 7, float32(45.45))

	// Adding documents out of filter scope, the returned stats should be the same
	s.insertDailyTestStats("p3", "r1", "test1", "task1", "v1", "d1", day1, 1, 1, 1)
	s.insertDailyTestStats("p1", "r3", "test1", "task1", "v1", "d1", day1, 1, 1, 1)
	s.insertDailyTestStats("p1", "r1", "test3", "task1", "v1", "d1", day1, 1, 1, 1)
	s.insertDailyTestStats("p1", "r1", "test1", "task3", "v1", "d1", day1, 1, 1, 1)
	s.insertDailyTestStats("p1", "r1", "test1", "task1", "v3", "d1", day1, 1, 1, 1)
	s.insertDailyTestStats("p1", "r1", "test1", "task1", "v1", "d3", day1, 1, 1, 1)
	s.insertDailyTestStats("p1", "r1", "test1", "task1", "v1", "d1", filter.BeforeDate.Add(24*time.Hour), 1, 1, 1)

	docs, err = GetTestStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTestStats(docs[0], "test1", "task1", "v1", "d1", day1, 10, 2, float32(12.22))
	s.checkTestStats(docs[1], "test1", "task1", "v1", "d1", day2, 20, 7, float32(45.45))

	// Change the sort order
	filter.Sort = SortLatestFirst
	docs, err = GetTestStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTestStats(docs[0], "test1", "task1", "v1", "d1", day2, 20, 7, float32(45.45))
	s.checkTestStats(docs[1], "test1", "task1", "v1", "d1", day1, 10, 2, float32(12.22))
	filter.Sort = SortEarliestFirst

	// Query with grouping by number of days, there should only be one result with aggregated stats
	filter.GroupNumDays = 2
	docs, err = GetTestStats(&filter)
	require.NoError(err)
	require.Len(docs, 1)
	s.checkTestStats(docs[0], "test1", "task1", "v1", "d1", day1, 30, 9, float32(34.373333))

	// Query with grouping by test+task+variant+distro
	// Adding a document for test1+task1+v1+d2 that should appear separately in the results
	s.insertDailyTestStats("p1", "r1", "test1", "task1", "v1", "d2", day1, 2, 2, 2)
	docs, err = GetTestStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTestStats(docs[0], "test1", "task1", "v1", "d1", day1, 30, 9, float32(34.373333))
	s.checkTestStats(docs[1], "test1", "task1", "v1", "d2", day1, 2, 2, float32(2))

	// Query with grouping by test+task+variant
	// The previous stats should be grouped
	filter.GroupBy = GroupByVariant
	// Adding a document for test1+task1+v2+d2 that should appear separately in the results
	s.insertDailyTestStats("p1", "r1", "test1", "task1", "v2", "d2", day1, 3, 3, 3)
	docs, err = GetTestStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTestStats(docs[0], "test1", "task1", "v1", "", day1, 32, 11, float32(32.350002))
	s.checkTestStats(docs[1], "test1", "task1", "v2", "", day1, 3, 3, float32(3))

	// Query with grouping by test+task
	// The previous stats should be grouped
	filter.GroupBy = GroupByTask
	// Adding a document for test1+task2+v2+d2 that should appear separately in the results
	s.insertDailyTestStats("p1", "r1", "test1", "task2", "v2", "d2", day1, 4, 4, 4)
	docs, err = GetTestStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTestStats(docs[0], "test1", "task1", "", "", day1, 35, 14, float32(29.834286))
	s.checkTestStats(docs[1], "test1", "task2", "", "", day1, 4, 4, float32(4))

	// Query with grouping by test
	// The previous stats should be grouped
	filter.GroupBy = GroupByTest
	// Adding a document for test2+task2+v2+d2 that should appear separately in the results
	s.insertDailyTestStats("p1", "r1", "test2", "task2", "v2", "d2", day1, 5, 5, 5)
	docs, err = GetTestStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTestStats(docs[0], "test1", "", "", "", day1, 39, 18, float32(27.184616))
	s.checkTestStats(docs[1], "test2", "", "", "", day1, 5, 5, float32(5))

	// Test pagination
	s.insertDailyTestStats("p2", "r1", "test1", "task2", "v2", "d1", day1, 1, 1, 1)
	s.insertDailyTestStats("p2", "r1", "test2", "task2", "v1", "d1", day1, 1, 1, 1)
	s.insertDailyTestStats("p2", "r1", "test3", "task2", "v2", "d1", day1, 1, 1, 1)
	s.insertDailyTestStats("p2", "r1", "test4", "task1", "v1", "d2", day1, 1, 1, 1)
	s.insertDailyTestStats("p2", "r1", "test5", "task1", "v2", "d1", day1, 1, 1, 1)
	s.insertDailyTestStats("p2", "r1", "test6", "task1", "v1", "d1", day1, 1, 1, 1)

	filter.Project = "p2"
	filter.Tests = nil
	filter.Limit = 2
	docs, err = GetTestStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTestStats(docs[0], "test1", "", "", "", day1, 1, 1, float32(1))
	s.checkTestStats(docs[1], "test2", "", "", "", day1, 1, 1, float32(1))

	filter.StartAt = &StartAt{Date: day1, Test: "test2"}
	docs, err = GetTestStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTestStats(docs[0], "test3", "", "", "", day1, 1, 1, float32(1))
	s.checkTestStats(docs[1], "test4", "", "", "", day1, 1, 1, float32(1))

	// Test pagination with grouping by test+task+variant+distro
	filter.GroupBy = GroupByDistro
	filter.StartAt = nil
	filter.Limit = 2
	docs, err = GetTestStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	// expecting the results ordered by date/variant/task/test/distro
	s.checkTestStats(docs[0], "test4", "task1", "v1", "d2", day1, 1, 1, float32(1))
	s.checkTestStats(docs[1], "test6", "task1", "v1", "d1", day1, 1, 1, float32(1))

	filter.StartAt = &StartAt{Date: day1, Test: "test6", Task: "task1", BuildVariant: "v1", Distro: "d1"}
	docs, err = GetTestStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTestStats(docs[0], "test2", "task2", "v1", "d1", day1, 1, 1, float32(1))
	s.checkTestStats(docs[1], "test5", "task1", "v2", "d1", day1, 1, 1, float32(1))

	filter.StartAt = &StartAt{Date: day1, Test: "test5", Task: "task1", BuildVariant: "v2", Distro: "d1"}
	docs, err = GetTestStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTestStats(docs[0], "test1", "task2", "v2", "d1", day1, 1, 1, float32(1))
	s.checkTestStats(docs[1], "test3", "task2", "v2", "d1", day1, 1, 1, float32(1))
}

func (s *statsQuerySuite) TestGetTaskStats() {
	require := s.Require()

	day1 := baseDay
	day2 := baseDay.Add(24 * time.Hour)
	day8 := baseDay.Add(7 * 24 * time.Hour)

	filter := StatsFilter{
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

	// Query on empty collection
	docs, err := GetTaskStats(&filter)
	require.NoError(err)
	require.Empty(docs)

	// Query on collection with 1 document
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, 10.5)

	docs, err = GetTaskStats(&filter)
	require.NoError(err)
	require.Len(docs, 1)
	s.checkTaskStats(docs[0], "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, float32(10.5))

	// Query on collection with 2 documents
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", day2, 20, 7, 7, 0, 0, 0, 20.0)

	docs, err = GetTaskStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, float32(10.5))
	s.checkTaskStats(docs[1], "task1", "v1", "d1", day2, 20, 7, 7, 0, 0, 0, float32(20))

	// Adding documents out of filter scope, the returned stats should be the same
	s.insertDailyTaskStats("p3", "r1", "task1", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, 1)
	s.insertDailyTaskStats("p1", "r3", "task1", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, 1)
	s.insertDailyTaskStats("p1", "r1", "task3", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, 1)
	s.insertDailyTaskStats("p1", "r1", "task1", "v3", "d1", day1, 1, 1, 1, 0, 0, 0, 1)
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d1", filter.BeforeDate.Add(24*time.Hour), 1, 1, 1, 0, 0, 0, 1)

	docs, err = GetTaskStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, float32(10.5))
	s.checkTaskStats(docs[1], "task1", "v1", "d1", day2, 20, 7, 7, 0, 0, 0, float32(20))

	// Change the sort order
	filter.Sort = SortLatestFirst
	docs, err = GetTaskStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task1", "v1", "d1", day2, 20, 7, 7, 0, 0, 0, float32(20))
	s.checkTaskStats(docs[1], "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, float32(10.5))
	filter.Sort = SortEarliestFirst

	// Query with grouping by number of days, there should only be one result with aggregated stats
	filter.GroupNumDays = 2
	docs, err = GetTaskStats(&filter)
	require.NoError(err)
	require.Len(docs, 1)
	s.checkTaskStats(docs[0], "task1", "v1", "d1", day1, 30, 12, 8, 1, 1, 2, float32(16.833334))

	// Query with grouping by test+task+variant+distro
	// Adding a document for task1+v1+d2 that should appear separately in the results
	s.insertDailyTaskStats("p1", "r1", "task1", "v1", "d2", day1, 2, 3, 2, 1, 0, 0, 10)
	docs, err = GetTaskStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task1", "v1", "d1", day1, 30, 12, 8, 1, 1, 2, float32(16.833334))
	s.checkTaskStats(docs[1], "task1", "v1", "d2", day1, 2, 3, 2, 1, 0, 0, float32(10))

	// Query with grouping by test+task+variant
	// The previous stats should be grouped
	filter.GroupBy = GroupByVariant
	// Adding a document for test1+task1+v2+d2 that should appear separately in the results
	s.insertDailyTaskStats("p1", "r1", "task1", "v2", "d2", day1, 2, 3, 2, 1, 0, 0, 10)
	docs, err = GetTaskStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task1", "v1", "", day1, 32, 15, 10, 2, 1, 2, float32(16.40625))
	s.checkTaskStats(docs[1], "task1", "v2", "", day1, 2, 3, 2, 1, 0, 0, float32(10))

	// Query with grouping by test+task
	// The previous stats should be grouped
	filter.GroupBy = GroupByTask
	// Adding a document for test1+task2+v2+d2 that should appear separately in the results
	s.insertDailyTaskStats("p1", "r1", "task2", "v2", "d2", day1, 2, 3, 2, 1, 0, 0, 10)
	docs, err = GetTaskStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task1", "", "", day1, 34, 18, 12, 3, 1, 2, float32(16.029411))
	s.checkTaskStats(docs[1], "task2", "", "", day1, 2, 3, 2, 1, 0, 0, float32(10))

	// Test pagination
	s.insertDailyTaskStats("p2", "r1", "task1", "v2", "d2", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task2", "v2", "d1", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task3", "v2", "d1", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task4", "v1", "d2", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task5", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, 10)
	s.insertDailyTaskStats("p2", "r1", "task6", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, 10)

	filter.Project = "p2"
	filter.Tasks = []string{"task1", "task2", "task3", "task4", "task5", "task6"}
	filter.Limit = 2
	docs, err = GetTaskStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task1", "", "", day1, 1, 1, 1, 0, 0, 0, float32(10))
	s.checkTaskStats(docs[1], "task2", "", "", day1, 1, 1, 1, 0, 0, 0, float32(10))

	filter.StartAt = &StartAt{Date: day1, Task: "task2"}
	docs, err = GetTaskStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task3", "", "", day1, 1, 1, 1, 0, 0, 0, float32(10))
	s.checkTaskStats(docs[1], "task4", "", "", day1, 1, 1, 1, 0, 0, 0, float32(10))

	// Test pagination with grouping by task+variant+distro
	filter.GroupBy = GroupByDistro
	filter.StartAt = nil
	filter.Limit = 2
	docs, err = GetTaskStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	// expecting the results ordered by date/variant/task/test/distro
	s.checkTaskStats(docs[0], "task4", "v1", "d2", day1, 1, 1, 1, 0, 0, 0, float32(10))
	s.checkTaskStats(docs[1], "task5", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, float32(10))

	filter.StartAt = &StartAt{Date: day1, Task: "task5", BuildVariant: "v1", Distro: "d1"}
	docs, err = GetTaskStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task6", "v1", "d1", day1, 1, 1, 1, 0, 0, 0, float32(10))
	s.checkTaskStats(docs[1], "task1", "v2", "d2", day1, 1, 1, 1, 0, 0, 0, float32(10))

	filter.StartAt = &StartAt{Date: day1, Task: "task1", BuildVariant: "v2", Distro: "d2"}
	docs, err = GetTaskStats(&filter)
	require.NoError(err)
	require.Len(docs, 2)
	s.checkTaskStats(docs[0], "task2", "v2", "d1", day1, 1, 1, 1, 0, 0, 0, float32(10))
	s.checkTaskStats(docs[1], "task3", "v2", "d1", day1, 1, 1, 1, 0, 0, 0, float32(10))
}

func (s *statsQuerySuite) checkTestStats(stats TestStats, test, task, variant, distro string, date time.Time, numPass, numFail int, avgDuration float32) {
	require := s.Require()
	require.Equal(test, stats.TestFile)
	require.Equal(task, stats.TaskName)
	require.Equal(variant, stats.BuildVariant)
	require.Equal(distro, stats.Distro)
	require.WithinDuration(date, stats.Date, 0)
	require.Equal(numPass, stats.NumPass)
	require.Equal(numFail, stats.NumFail)
	require.Equal(avgDuration, stats.AvgDurationPass)
}

func (s *statsQuerySuite) checkTaskStats(stats TaskStats, task, variant, distro string, date time.Time, numSuccess, numFailed, numTimeout, numTestFailed, numSystemFailed, numSetupFailed int, avgDuration float32) {
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

func (s *statsQuerySuite) insertDailyTestStats(project string, requester string, testFile string, taskName string, variant string, distro string, date time.Time, numPass int, numFail int, avgDuration float32) {

	err := db.Insert(dailyTestStatsCollection, bson.M{
		"_id":               createTestStatsId(project, requester, testFile, taskName, variant, distro, date),
		"num_pass":          numPass,
		"num_fail":          numFail,
		"avg_duration_pass": avgDuration,
	})
	s.Require().NoError(err)
}

func (s *statsQuerySuite) insertDailyTaskStats(project string, requester string, taskName string, variant string, distro string, date time.Time, numSuccess, numFailed, numTimeout, numTestFailed, numSystemFailed, numSetupFailed int, avgDuration float32) {

	err := db.Insert(dailyTaskStatsCollection, bson.M{
		"_id":                  createTaskStatsId(project, requester, taskName, variant, distro, date),
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
