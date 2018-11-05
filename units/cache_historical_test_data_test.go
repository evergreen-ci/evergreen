package units

import (
	"context"
	"fmt"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/stats"
	modelUtil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"testing"
	"time"
)

type cacheHistoryTestDataSuite struct {
	suite.Suite
	projectId string
	requester string
	hours []time.Time
	days []time.Time
	taskLists [][]string
	statsList []stats.StatsToUpdate
}

func TestCacheHistoricalTestDataJob(t *testing.T) {
	suite.Run(t, new(cacheHistoryTestDataSuite))
}

func (s *cacheHistoryTestDataSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *cacheHistoryTestDataSuite) SetupTest() {
	s.projectId = "projectId-0"
	s.requester = "requester0"

	now := time.Now()
	s.hours = []time.Time{now, now.Add(time.Hour * -2), now.Add(time.Hour * -4)}
	s.days = []time.Time{now, now.Add(time.Hour * -48)}

	s.taskLists = append(s.taskLists, []string{"task0", "task1", "task2"})
	s.taskLists = append(s.taskLists, []string{"task3", "task4", "task5"})
	s.taskLists = append(s.taskLists, []string{"task6", "task7"})

	s.statsList = []stats.StatsToUpdate{
		{
			ProjectId: s.projectId,
			Requester: s.requester,
			Hour:      s.hours[0],
			Day:       s.days[0],
			Tasks:     s.taskLists[0],
		},
		{
			ProjectId: s.projectId,
			Requester: s.requester,
			Hour:      s.hours[1],
			Day:       s.days[1],
			Tasks:     s.taskLists[1],
		},
		{
			ProjectId: s.projectId,
			Requester: s.requester,
			Hour:      s.hours[2],
			Day:       s.days[0],
			Tasks:     s.taskLists[2],
		},
	}
}

func (s *cacheHistoryTestDataSuite) TestFindSyncDateWithAWeek() {
	prevSyncDate := time.Now().Add(time.Hour * 24 * -3)
	syncDate := findTargetTimeForSync(prevSyncDate)
	s.WithinDuration(time.Now(), syncDate, time.Minute)
}

func (s *cacheHistoryTestDataSuite) TestFindSyncDateOutsideOfAWeek() {
	prevSyncDate := time.Now().Add(time.Hour * 24 * -10)
	expectedSyncDate := prevSyncDate.Add(maxSyncDuration)
	syncDate := findTargetTimeForSync(prevSyncDate)
	s.WithinDuration(expectedSyncDate, syncDate, time.Minute)
}

func (s *cacheHistoryTestDataSuite) TestUpdateHourlyAndDailyStats() {
	now := time.Now()

	callCountFn0 := 0

	mockFn0 := func (p string, r string, h time.Time, ts []string, jobTime time.Time) error {
		s.Equal(s.projectId, p)
		s.Equal(s.requester, r)
		s.Contains(s.hours, h)
		s.Equal(now, jobTime)

		callCountFn0++

		return nil
	}

	callCountFn1 := 0

	mockFn1 := func (p string, r string, d time.Time, ts []string, jobTime time.Time) error {
		s.Equal(s.projectId, p)
		s.Equal(s.requester, r)
		s.Contains(s.days, d)
		s.Equal(now, jobTime)

		callCountFn1++

		return nil
	}

	mockGenFns := generateFunctions{
		HourlyFns: map[string]generateStatsFn{
			"type1": mockFn0,
		},
		DailyFns: map[string]generateStatsFn{
			"type1": mockFn1,
			"type2": mockFn1,
		},
	}

	err := updateHourlyAndDailyStats(s.projectId, s.statsList, now, mockGenFns)

	s.Nil(err)

	s.Equal(len(mockGenFns.HourlyFns) * 3, callCountFn0)
	s.Equal(len(mockGenFns.DailyFns) * 2, callCountFn1)
}

func (s *cacheHistoryTestDataSuite) TestUpdateHourlyAndDailyStatsWithAnHourlyError() {
	now := time.Now()
	errorMessage := "error message"

	mockFn0 := func (p string, r string, h time.Time, ts []string, jobTime time.Time) error {
		return fmt.Errorf(errorMessage)
	}

	mockFn1 := func (p string, r string, d time.Time, ts []string, jobTime time.Time) error {
		return nil
	}

	mockGenFns := generateFunctions{
		HourlyFns: map[string]generateStatsFn{
			"type1": mockFn0,
		},
		DailyFns: map[string]generateStatsFn{
			"type1": mockFn1,
			"type2": mockFn1,
		},
	}

	err := updateHourlyAndDailyStats(s.projectId, s.statsList, now, mockGenFns)

	s.EqualError(err, errorMessage)
}

func (s *cacheHistoryTestDataSuite) TestUpdateHourlyAndDailyStatsWithADailyError() {
	now := time.Now()
	errorMessage := "error message"

	mockFn0 := func (p string, r string, h time.Time, ts []string, jobTime time.Time) error {
		return nil
	}

	mockFn1 := func (p string, r string, d time.Time, ts []string, jobTime time.Time) error {
		return fmt.Errorf(errorMessage)
	}

	mockGenFns := generateFunctions{
		HourlyFns: map[string]generateStatsFn{
			"type1": mockFn0,
		},
		DailyFns: map[string]generateStatsFn{
			"type1": mockFn1,
			"type2": mockFn1,
		},
	}

	err := updateHourlyAndDailyStats(s.projectId, s.statsList, now, mockGenFns)

	s.EqualError(err, errorMessage)
}

func (s *cacheHistoryTestDataSuite) TestIteratorOverHourlyStats() {
	callCount := 0

	mockHourlyGenerateFn := func(projId string, req string, h time.Time, tasks []string,
		jobTime time.Time) error {
			callCount++
			s.Equal(s.projectId, projId)
			s.Equal(s.requester, req)
			s.Contains(s.hours, h)

			return nil
	}

	iteratorOverHourlyStats(s.statsList, time.Now(), mockHourlyGenerateFn, "hourly")

	s.Equal(len(s.statsList),  callCount)
}

func (s *cacheHistoryTestDataSuite) TestIteratorOverDailyStats() {
	statsRollup := dailyStatsRollup{
		s.days[0]: {
			s.requester: s.taskLists[0],
		},
		s.days[1]: {
			s.requester: s.taskLists[1],
		},
	}

	callCount := 0

	mockDailyGenerateFn := func(projId string, req string, d time.Time, tasks []string,
		jobTime time.Time) error {
		callCount++
		s.Equal(s.projectId, projId)
		s.Equal(s.requester, req)

		return nil
	}

	iteratorOverDailyStats(s.projectId, statsRollup, time.Now(), mockDailyGenerateFn, "daily")

	s.Equal(len(statsRollup),  callCount)
}

func (s *cacheHistoryTestDataSuite) TestDailyStatsRollupShouldGroupTasksByDay() {
	dailyStatsRollup := buildDailyStatsRollup(s.statsList)

	// There should be two groups, one for each day.
	s.Equal(2, len(dailyStatsRollup))
	s.Contains(dailyStatsRollup, s.days[0])
	s.Contains(dailyStatsRollup, s.days[1])

	// Make sure tasks ended up in the proper groups.
	for _, t := range append(s.taskLists[0], s.taskLists[2]...) {
		s.Contains(dailyStatsRollup[s.days[0]][s.requester], t)
		s.NotContains(dailyStatsRollup[s.days[1]][s.requester], t)
	}
	for _, t := range s.taskLists[1] {
		s.Contains(dailyStatsRollup[s.days[1]][s.requester], t)
		s.NotContains(dailyStatsRollup[s.days[0]][s.requester], t)
	}
	// Make sure there aren't any duplicate tasks.
	s.Equal(len(s.taskLists[0]) + len(s.taskLists[2]), len(dailyStatsRollup[s.days[0]][s.requester]))
	s.Equal(len(s.taskLists[1]), len(dailyStatsRollup[s.days[1]][s.requester]))
}

func (s *cacheHistoryTestDataSuite) TestDailyStatsRollupShouldGroupTasksByRequester() {
	requester0 := "requester0"
	requester1 := "requester1"

	day := time.Now()
	otherDay := day.Add(time.Hour * -48)
	hour := day

	tasks0 := []string{"task0", "task1", "task2"}
	tasks1 := []string{"task3", "task4", "task5"}
	tasks2 := []string{"task6", "task7", "task8"}

	statsList := []stats.StatsToUpdate{
		{
			ProjectId: "projectId-0",
			Requester: requester0,
			Hour:      hour,
			Day:       day,
			Tasks:     tasks0,
		},
		{
			ProjectId: "projectId-0",
			Requester: requester0,
			Hour:      hour.Add(time.Hour * -2),
			Day:       otherDay,
			Tasks:     tasks1,
		},
		{
			ProjectId: "projectId-0",
			Requester: requester1,
			Hour:      hour.Add(time.Hour * -4),
			Day:       day,
			Tasks:     tasks2,
		},
	}

	dailyStatsRollup := buildDailyStatsRollup(statsList)

	// There should be two groups, one for each day.
	s.Equal(2, len(dailyStatsRollup))
	s.Contains(dailyStatsRollup, day)
	s.Contains(dailyStatsRollup, otherDay)

	// The 'day' group should have two group, one for each requester.
	s.Equal(2, len(dailyStatsRollup[day]))

	// Make sure tasks ended up in the proper groups.
	for _, t := range tasks0 {
		s.Contains(dailyStatsRollup[day][requester0], t)
		s.NotContains(dailyStatsRollup[day][requester1], t)
		s.NotContains(dailyStatsRollup[otherDay][requester0], t)
	}
	for _, t := range tasks1 {
		s.Contains(dailyStatsRollup[otherDay][requester0], t)
		s.NotContains(dailyStatsRollup[day][requester0], t)
		s.NotContains(dailyStatsRollup[day][requester1], t)
	}
	for _, t := range tasks2 {
		s.Contains(dailyStatsRollup[day][requester1], t)
		s.NotContains(dailyStatsRollup[day][requester0], t)
		s.NotContains(dailyStatsRollup[otherDay][requester0], t)
	}
	// Make sure there aren't any duplicate tasks.
	s.Equal(len(tasks0), len(dailyStatsRollup[day][requester0]))
	s.Equal(len(tasks1), len(dailyStatsRollup[otherDay][requester0]))
	s.Equal(len(tasks2), len(dailyStatsRollup[day][requester1]))
}

func (s *cacheHistoryTestDataSuite) TestFilterIgnoredTasksFiltersTasks() {
	legitTasks := []string{
		"task0",
		"task1",
	}
	moreLegitTasks := []string{
		"task2",
	}
	fuzzerTasks := []string{
		"jstestfuzz-test-0",
		"agg-fuzzer-test-0",
		"agg-fuzzer-test-1",
	}
	concurrencyTasks := []string{
		"concurrency_simultaneous-0",
	}
	taskList := []string{}
	taskList = append(taskList, legitTasks...)
	taskList = append(taskList, fuzzerTasks...)
	taskList = append(taskList, moreLegitTasks...)
	taskList = append(taskList, concurrencyTasks...)

	filteredList := filterIgnoredTasks(taskList)

	for _, t := range append(legitTasks, moreLegitTasks...) {
		s.Contains(filteredList, t)
	}

	for _, t := range append(fuzzerTasks, concurrencyTasks...) {
		s.NotContains(filteredList, t)
	}
}

func (s *cacheHistoryTestDataSuite) TestCacheHistoricalTestDataJob() {
	clearStatsData(s.Suite)

	baseTime := time.Now().Add(-4 * 7 * 24 * time.Hour + 2 * time.Hour)

	s.createTestData(baseTime)

	job := NewCacheHistoricalTestDataJob(s.projectId, "1")
	job.Run(context.Background())
	s.NoError(job.Error())

	doc := modelUtil.GetDailyTestDoc(s.Suite, s.projectId, s.requester, "test1.js", "taskName", "", "", baseTime.Truncate(time.Hour * 24))
	s.NotNil(doc)
	s.Equal(2, doc.NumFail)
	s.Equal(0, doc.NumPass)

	doc = modelUtil.GetDailyTestDoc(s.Suite, s.projectId, s.requester, "test2.js", "taskName", "", "", baseTime.Truncate(time.Hour * 24))
	s.NotNil(doc)
	s.Equal(1, doc.NumFail)
	s.Equal(1, doc.NumPass)

	doc = modelUtil.GetDailyTestDoc(s.Suite, s.projectId, s.requester, "test3.js", "taskName", "", "", baseTime.Truncate(time.Hour * 24))
	s.NotNil(doc)
	s.Equal(0, doc.NumFail)
	s.Equal(2, doc.NumPass)
}

func (s *cacheHistoryTestDataSuite)createTestData(baseTime time.Time) {
	t0 := baseTime
	t1 := baseTime.Add(time.Minute * 30)

	taskName := "taskName"

	task0 := modelUtil.InsertFinishedTask(s.Suite, s.projectId, s.requester, taskName, t0, t0.Add(30 * time.Minute), 1)
	modelUtil.InsertTestResult(s.Suite, task0.Id, 1, "test1.js", "fail", 60)
	modelUtil.InsertTestResult(s.Suite, task0.Id, 1, "test2.js", "pass", 120)
	modelUtil.InsertTestResult(s.Suite, task0.Id, 1, "test3.js", "pass", 10)

	task1 := modelUtil.InsertFinishedTask(s.Suite, s.projectId, s.requester, taskName, t1, t1.Add(30 * time.Minute), 2)
	modelUtil.InsertTestResult(s.Suite, task1.Id, 2, "test1.js", "fail", 60)
	modelUtil.InsertTestResult(s.Suite, task1.Id, 2, "test2.js", "fail", 120)
	modelUtil.InsertTestResult(s.Suite, task1.Id, 2, "test3.js", "pass", 10)
}

func clearStatsData(s suite.Suite) {
	collectionsToClear := [...]string{
		"hourly_test_stats",
		"daily_test_stats",
		"daily_task_stats",
		"tasks",
		"old_tasks",
		"test_results",
		"daily_stats_status",
	}

	for _, coll := range collectionsToClear {
		modelUtil.ClearCollection(s, coll)
	}
}
