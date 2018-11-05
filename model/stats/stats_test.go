package stats

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	modelUtil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

var baseDay = time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC)
var baseHour = baseDay.Add(time.Hour * 16)
var baseTime = baseHour.Add(time.Minute * 45)
var jobTime = time.Date(1998, 7, 12, 20, 45, 0, 0, time.UTC)
var commit1 = baseTime
var commit2 = baseTime.Add(26 * time.Hour)
var finish1 = baseTime.Add(5 * 24 * time.Hour)
var finish2 = baseTime.Add(7 * 24 * time.Hour)

type statsSuite struct {
	suite.Suite
}

func TestStatsSuite(t *testing.T) {
	suite.Run(t, new(statsSuite))
}

func (s *statsSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *statsSuite) SetupTest() {
	modelUtil.ClearCollection(s.Suite, hourlyTestStatsCollection)
	modelUtil.ClearCollection(s.Suite, dailyTestStatsCollection)
	modelUtil.ClearCollection(s.Suite, dailyStatsStatusCollection)
	modelUtil.ClearCollection(s.Suite, dailyTaskStatsCollection)
	modelUtil.ClearCollection(s.Suite, task.Collection)
	modelUtil.ClearCollection(s.Suite, task.OldCollection)
	modelUtil.ClearCollection(s.Suite, testresult.Collection)
}

func (s *statsSuite) TestStatsStatus() {
	require := s.Require()

	// Check that we get a default status when there is no doc in the database.
	status, err := GetStatsStatus("p1")
	require.NoError(err)
	require.NotNil(status)
	// The default value is rounded off to the day so use a delta of over one day to cover all cases.
	oneDayOneMinute := 24*time.Hour + time.Minute
	expected := time.Now().Add(-defaultBackFillPeriod)
	require.WithinDuration(expected, status.LastJobRun, oneDayOneMinute)
	require.WithinDuration(expected, status.ProcessedTasksUntil, oneDayOneMinute)

	// Check that we can update the status and read the new values.
	err = UpdateStatsStatus("p1", baseHour, baseDay)
	require.NoError(err)

	status, err = GetStatsStatus("p1")
	require.NoError(err)
	require.NotNil(status)
	require.Equal(baseHour.UTC(), status.LastJobRun.UTC())
	require.Equal(baseDay.UTC(), status.ProcessedTasksUntil.UTC())
}

func (s *statsSuite) TestGenerateHourlyTestStats() {
	require := s.Require()

	// Insert task docs.
	s.initTasks()

	// Generate hourly stats for project p1 and an unknown task.
	err := GenerateHourlyTestStats("p1", "r1", baseHour, []string{"unknown_task"}, jobTime)
	require.NoError(err)
	require.Equal(0, s.countHourlyTestDocs())

	// Generate hourly stats for project p1.
	err = GenerateHourlyTestStats("p1", "r1", baseHour, []string{"task1"}, jobTime)
	require.NoError(err)
	require.Equal(5, s.countHourlyTestDocs())

	doc := modelUtil.GetHourlyTestDoc(s.Suite, "p1", "r1", "test1.js", "task1", "v1", "d1", baseHour)
	require.NotNil(doc)
	require.Equal("p1", doc.Id.Project)
	require.Equal("r1", doc.Id.Requester)
	require.Equal("test1.js", doc.Id.TestFile)
	require.Equal("task1", doc.Id.TaskName)
	require.Equal("v1", doc.Id.Variant)
	require.Equal("d1", doc.Id.Distro)
	require.Equal(baseHour.UTC(), doc.Id.Date.UTC())
	require.Equal(0, doc.NumPass)
	require.Equal(2, doc.NumFail)
	require.Equal(float32(0), doc.AvgDurationPass)
	require.WithinDuration(jobTime, doc.LastUpdate, 0)

	doc = modelUtil.GetHourlyTestDoc(s.Suite, "p1", "r1", "test2.js", "task1", "v1", "d1", baseHour)
	require.NotNil(doc)
	require.Equal(1, doc.NumPass)
	require.Equal(1, doc.NumFail)
	require.Equal(float32(120), doc.AvgDurationPass)

	doc = modelUtil.GetHourlyTestDoc(s.Suite, "p1", "r1", "test3.js", "task1", "v1", "d1", baseHour)
	require.NotNil(doc)
	require.Equal(2, doc.NumPass)
	require.Equal(0, doc.NumFail)
	require.Equal(float32(12.5), doc.AvgDurationPass)

	// Generate hourly stats for project p2
	// Testing old tasks.
	err = GenerateHourlyTestStats("p2", "r1", baseHour, []string{"task1"}, jobTime)
	require.NoError(err)
	require.Equal(8, s.countHourlyTestDocs()) // 3 more tests combination were added to the collection

	doc = modelUtil.GetHourlyTestDoc(s.Suite, "p2", "r1", "test1.js", "task1", "v1", "d1", baseHour)
	require.NotNil(doc)
	require.Equal(0, doc.NumPass)
	require.Equal(3, doc.NumFail)
	require.Equal(float32(0), doc.AvgDurationPass)
	require.WithinDuration(jobTime, doc.LastUpdate, 0)

	doc = modelUtil.GetHourlyTestDoc(s.Suite, "p2", "r1", "test2.js", "task1", "v1", "d1", baseHour)
	require.Equal(1, doc.NumPass)
	require.Equal(2, doc.NumFail)
	require.Equal(float32(120), doc.AvgDurationPass)
	require.WithinDuration(jobTime, doc.LastUpdate, 0)

	// Generate hourly stats for project p3.
	// Testing display task / execution task.
	err = GenerateHourlyTestStats("p3", "r1", baseHour, []string{"task_exec_1"}, jobTime)
	require.NoError(err)
	require.Equal(10, s.countHourlyTestDocs()) // 2 more tests combination were added to the collection

	doc = modelUtil.GetHourlyTestDoc(s.Suite, "p3", "r1", "test1.js", "task_exec_1", "v1", "d1", baseHour)
	require.Nil(doc)
	doc = modelUtil.GetHourlyTestDoc(s.Suite, "p3", "r1", "test1.js", "task_display_1", "v1", "d1", baseHour)
	require.NotNil(doc)
	require.Equal(0, doc.NumPass)
	require.Equal(1, doc.NumFail)
	require.Equal(float32(0), doc.AvgDurationPass)

	doc = modelUtil.GetHourlyTestDoc(s.Suite, "p3", "r1", "test2.js", "task_display_1", "v1", "d1", baseHour)
	require.NotNil(doc)
	require.Equal(1, doc.NumPass)
	require.Equal(0, doc.NumFail)
	require.Equal(float32(120), doc.AvgDurationPass)
}

func (s *statsSuite) TestGenerateDailyTestStatsFromHourly() {
	require := s.Require()

	// Insert hourly test stats docs.
	s.initHourly()
	// Generate daily test stats for unknown task.
	err := GenerateDailyTestStatsFromHourly("p1", "r1", baseDay, []string{"unknown_task"}, jobTime)
	require.NoError(err)
	require.Equal(0, s.countDailyTestDocs())

	// Generate daily test stats for exiting task
	err = GenerateDailyTestStatsFromHourly("p1", "r1", baseDay, []string{"task1"}, jobTime)
	require.NoError(err)
	require.Equal(1, s.countDailyTestDocs())

	doc := modelUtil.GetDailyTestDoc(s.Suite, "p1", "r1", "test1.js", "task1", "v1", "d1", baseDay)
	require.NotNil(doc)
	require.Equal("p1", doc.Id.Project)
	require.Equal("r1", doc.Id.Requester)
	require.Equal("test1.js", doc.Id.TestFile)
	require.Equal("task1", doc.Id.TaskName)
	require.Equal("v1", doc.Id.Variant)
	require.Equal("d1", doc.Id.Distro)
	require.Equal(baseDay.UTC(), doc.Id.Date.UTC())
	require.Equal(30, doc.NumPass)
	require.Equal(5, doc.NumFail)
	require.Equal(float32(4), doc.AvgDurationPass)
	require.WithinDuration(jobTime, doc.LastUpdate, 0)
}

func (s *statsSuite) TestGenerateDailyTaskStats() {
	require := s.Require()

	// Insert task docs.
	s.initTasks()

	// Generate task stats for project p1 and an unknown task.
	err := GenerateDailyTaskStats("p1", "r1", baseHour, []string{"unknown_task"}, jobTime)
	require.NoError(err)
	require.Equal(0, s.countDailyTaskDocs())

	// Generate task stats for project p1.
	err = GenerateDailyTaskStats("p1", "r1", baseHour, []string{"task1", "task2"}, jobTime)
	require.NoError(err)
	require.Equal(3, s.countDailyTaskDocs())
	doc := modelUtil.GetDailyTaskDoc(s.Suite, "p1", "r1", "task1", "v1", "d1", baseDay)
	require.NotNil(doc)
	doc = modelUtil.GetDailyTaskDoc(s.Suite, "p1", "r1", "task1", "v2", "d1", baseDay)
	require.NotNil(doc)
	doc = modelUtil.GetDailyTaskDoc(s.Suite, "p1", "r1", "task2", "v1", "d1", baseDay)
	require.NotNil(doc)

	// Generate task stats for project p4 to check status aggregation
	err = GenerateDailyTaskStats("p4", "r1", baseHour, []string{"task1"}, jobTime)
	require.NoError(err)
	require.Equal(4, s.countDailyTaskDocs()) // 1 more task combination was added to the collection
	doc = modelUtil.GetDailyTaskDoc(s.Suite, "p4", "r1", "task1", "v1", "d1", baseDay)
	require.NotNil(doc)
	require.Equal(2, doc.NumSuccess)
	require.Equal(8, doc.NumFailed)
	require.Equal(1, doc.NumTestFailed)
	require.Equal(2, doc.NumSystemFailed)
	require.Equal(3, doc.NumSetupFailed)
	require.Equal(2, doc.NumTimeout)
	require.Equal(float32(150), doc.AvgDurationSuccess)
	require.WithinDuration(jobTime, doc.LastUpdate, 0)

	// Generate task for project p2 to check we get data for old tasks
	err = GenerateDailyTaskStats("p2", "r1", baseHour, []string{"task1"}, jobTime)
	require.NoError(err)
	require.Equal(5, s.countDailyTaskDocs()) // 1 more task combination was added to the collection
	doc = modelUtil.GetDailyTaskDoc(s.Suite, "p2", "r1", "task1", "v1", "d1", baseDay)
	require.NotNil(doc)
	require.Equal(1, doc.NumSuccess) // 1 old task
	require.Equal(3, doc.NumFailed)  // 2 tasks + 1 old tasks
	require.Equal(3, doc.NumTestFailed)
	require.Equal(0, doc.NumSystemFailed)
	require.Equal(0, doc.NumSetupFailed)
	require.Equal(0, doc.NumTimeout)
	require.Equal(float32(100), doc.AvgDurationSuccess)
	require.WithinDuration(jobTime, doc.LastUpdate, 0)
}

func (s *statsSuite) TestFindStatsToUpdate() {
	require := s.Require()

	// Insert task docs.
	s.initTasksToUpdate()

	// Find stats for p5 for a period with no finished tasks
	start := baseHour
	end := baseHour.Add(time.Hour)
	statsList, err := FindStatsToUpdate("p5", start, end)
	require.NoError(err)
	require.Len(statsList, 0)

	// Find stats for p5 for a period around finish1
	start = finish1.Add(-1 * time.Hour)
	end = finish1.Add(time.Hour)
	statsList, err = FindStatsToUpdate("p5", start, end)
	require.NoError(err)
	require.Len(statsList, 2)
	// The results are sorted so we know the order
	require.Equal("p5", statsList[0].ProjectId)
	require.Equal("r1", statsList[0].Requester)
	require.WithinDuration(util.GetUTCHour(commit1), statsList[0].Hour, 0)
	require.WithinDuration(util.GetUTCDay(commit1), statsList[0].Day, 0)
	require.Equal([]string{"task1"}, statsList[0].Tasks)

	// Find stats for p5 for a period around finish1
	start = finish1.Add(-1 * time.Hour)
	end = finish1.Add(time.Hour)
	statsList, err = FindStatsToUpdate("p5", start, end)
	require.NoError(err)
	require.Len(statsList, 2)
	// The results are sorted so we know the order
	require.Equal("p5", statsList[0].ProjectId)
	require.Equal("r1", statsList[0].Requester)
	require.WithinDuration(util.GetUTCHour(commit1), statsList[0].Hour, 0)
	require.WithinDuration(util.GetUTCDay(commit1), statsList[0].Day, 0)
	require.Equal([]string{"task1"}, statsList[0].Tasks)
	require.Equal("p5", statsList[1].ProjectId)
	require.Equal("r2", statsList[1].Requester)
	require.WithinDuration(util.GetUTCHour(commit2), statsList[1].Hour, 0)
	require.WithinDuration(util.GetUTCDay(commit2), statsList[1].Day, 0)
	require.Len(statsList[1].Tasks, 3)
	require.Contains(statsList[1].Tasks, "task2")
	require.Contains(statsList[1].Tasks, "task2bis")
	require.Contains(statsList[1].Tasks, "task2old")
}

func (s *statsSuite) TestStatsToUpdate() {
	require := s.Require()

	stats1 := StatsToUpdate{"p1", "r1", baseHour, baseDay, []string{"task1", "task2"}}
	stats1bis := StatsToUpdate{"p1", "r1", baseHour, baseDay, []string{"task1", "task"}}
	stats1later := StatsToUpdate{"p1", "r1", baseHour.Add(time.Hour), baseDay, []string{"task1", "task2"}}
	stats1r2 := StatsToUpdate{"p1", "r2", baseHour, baseDay, []string{"task1", "task2"}}
	stats2 := StatsToUpdate{"p2", "r1", baseHour, baseDay, []string{"task1", "task2"}}

	// canMerge
	require.True(stats1.canMerge(&stats1))
	require.True(stats1.canMerge(&stats1bis))
	require.False(stats1.canMerge(&stats1later))
	require.False(stats1.canMerge(&stats1r2))
	// comparison
	require.True(stats1.lt(&stats2))
	require.True(stats1.lt(&stats1later))
	require.True(stats1.lt(&stats1r2))
	require.False(stats1.lt(&stats1))
	require.False(stats1.lt(&stats1bis))
	// merge
	merged := stats1.merge(&stats1bis)
	require.Equal(merged.ProjectId, stats1.ProjectId)
	require.Equal(merged.Requester, stats1.Requester)
	require.Equal(merged.Hour, stats1.Hour)
	require.Equal(merged.Day, stats1.Day)
	for _, t := range stats1.Tasks {
		require.Contains(merged.Tasks, t)
	}
	for _, t := range stats1bis.Tasks {
		require.Contains(merged.Tasks, t)
	}
}

/////////////////////////////////////////
// Methods to initialize database data //
/////////////////////////////////////////

func (s *statsSuite) initHourly() {
	hour1 := baseHour
	hour2 := baseHour.Add(time.Hour)
	hour3 := baseHour.Add(24 * time.Hour)
	s.insertHourlyTestStats("p1", "r1", "test1.js", "task1", "v1", "d1", hour1, 10, 5, 2)
	s.insertHourlyTestStats("p1", "r1", "test1.js", "task1", "v1", "d1", hour2, 20, 0, 5)
	s.insertHourlyTestStats("p1", "r1", "test1.js", "task1", "v1", "d1", hour3, 20, 0, 5)
}

func (s *statsSuite) insertHourlyTestStats(project string, requester string, testFile string, taskName string, variant string, distro string, date time.Time, numPass int, numFail int, avgDuration float32) {

	err := db.Insert(hourlyTestStatsCollection, bson.M{
		"_id":               createTestStatsId(project, requester, testFile, taskName, variant, distro, date),
		"num_pass":          numPass,
		"num_fail":          numFail,
		"avg_duration_pass": avgDuration,
	})
	s.Require().NoError(err)
}

func (s *statsSuite) initTasks() {
	t0 := baseTime
	t0plus10m := baseTime.Add(10 * time.Minute)
	t0plus1h := baseTime.Add(time.Hour)
	t0min10m := baseTime.Add(-10 * time.Minute)
	t0min1h := baseTime.Add(-1 * time.Hour)

	// Task
	modelUtil.InsertTask(s.Suite, "p1", "r1", "task_id_1", 0, "task1", "v1", "d1", t0, modelUtil.TestFailed)
	modelUtil.InsertTestResult(s.Suite, "task_id_1", 0, "test1.js", "fail", 60)
	modelUtil.InsertTestResult(s.Suite, "task_id_1", 0, "test2.js", "fail", 120)
	modelUtil.InsertTestResult(s.Suite, "task_id_1", 0, "test3.js", "pass", 10)
	// Task on variant v2
	modelUtil.InsertTask(s.Suite, "p1", "r1", "task_id_2", 0, "task1", "v2", "d1", t0, modelUtil.TestFailed)
	modelUtil.InsertTestResult(s.Suite, "task_id_2", 0, "test1.js", "fail", 60)
	modelUtil.InsertTestResult(s.Suite, "task_id_2", 0, "test2.js", "fail", 120)
	// Task with different task name
	modelUtil.InsertTask(s.Suite, "p1", "r1", "task_id_3", 0, "task2", "v1", "d1", t0, modelUtil.TestFailed)
	modelUtil.InsertTestResult(s.Suite, "task_id_3", 0, "test1.js", "fail", 60)
	modelUtil.InsertTestResult(s.Suite, "task_id_3", 0, "test2.js", "fail", 120)
	// Task 10 minutes later
	modelUtil.InsertTask(s.Suite, "p1", "r1", "task_id_4", 0, "task1", "v1", "d1", t0plus10m, modelUtil.TestFailed)
	modelUtil.InsertTestResult(s.Suite, "task_id_4", 0, "test1.js", "fail", 60)
	modelUtil.InsertTestResult(s.Suite, "task_id_4", 0, "test2.js", "pass", 120)
	modelUtil.InsertTestResult(s.Suite, "task_id_4", 0, "test3.js", "pass", 15)
	// Task 1 hour later
	modelUtil.InsertTask(s.Suite, "p1", "r1", "task_id_5", 0, "task1", "v1", "d1", t0plus1h, modelUtil.TestFailed)
	modelUtil.InsertTestResult(s.Suite, "task_id_5", 0, "test1.js", "fail", 60)
	modelUtil.InsertTestResult(s.Suite, "task_id_5", 0, "test2.js", "fail", 120)
	// Task different requester
	modelUtil.InsertTask(s.Suite, "p1", "r2", "task_id_6", 0, "task1", "v1", "d1", t0, modelUtil.TestFailed)
	modelUtil.InsertTestResult(s.Suite, "task_id_6", 0, "test1.js", "fail", 60)
	modelUtil.InsertTestResult(s.Suite, "task_id_6", 0, "test2.js", "fail", 120)
	// Task different project
	modelUtil.InsertTask(s.Suite, "p2", "r1", "task_id_7", 0, "task1", "v1", "d1", t0, modelUtil.TestFailed)
	modelUtil.InsertTestResult(s.Suite, "task_id_7", 0, "test1.js", "fail", 60)
	modelUtil.InsertTestResult(s.Suite, "task_id_7", 0, "test2.js", "fail", 120)
	// Task with old executions.
	modelUtil.InsertTask(s.Suite, "p2", "r1", "task_id_8", 2, "task1", "v1", "d1", t0plus10m, modelUtil.TestFailed)
	modelUtil.InsertTestResult(s.Suite, "task_id_8", 2, "test1.js", "fail", 60)
	modelUtil.InsertTestResult(s.Suite, "task_id_8", 2, "test2.js", "fail", 120)
	modelUtil.InsertOldTask(s.Suite, "p2", "r1", "task_id_8", 0, "task1", "v1", "d1", t0min10m, modelUtil.TestFailed)
	modelUtil.InsertTestResult(s.Suite, "task_id_8", 0, "test1.js", "fail", 60)
	modelUtil.InsertTestResult(s.Suite, "task_id_8", 0, "test2.js", "pass", 120)
	modelUtil.InsertTestResult(s.Suite, "task_id_8", 0, "testOld.js", "fail", 120)
	modelUtil.InsertOldTask(s.Suite, "p2", "r1", "task_id_8", 1, "task1", "v1", "d1", t0min1h, modelUtil.Success100)
	modelUtil.InsertTestResult(s.Suite, "task_id_8", 1, "test1.js", "pass", 60)
	modelUtil.InsertTestResult(s.Suite, "task_id_8", 1, "test2.js", "pass", 120)
	// Execution task
	modelUtil.InsertTask(s.Suite, "p3", "r1", "task_id_9", 0, "task_exec_1", "v1", "d1", t0, modelUtil.TestFailed)
	modelUtil.InsertTestResult(s.Suite, "task_id_9", 0, "test1.js", "fail", 60)
	modelUtil.InsertTestResult(s.Suite, "task_id_9", 0, "test2.js", "pass", 120)
	// Display task
	modelUtil.InsertDisplayTask(s.Suite, "p3", "r1", "task_id_10", 0, "task_display_1", "v1", "d1", t0, modelUtil.TestFailed, []string{"task_id_9"})
	// Project p4 used to test various task statuses
	modelUtil.InsertTask(s.Suite, "p4", "r1", "task_id_11", 0, "task1", "v1", "d1", t0, modelUtil.Success100)
	modelUtil.InsertTask(s.Suite, "p4", "r1", "task_id_12", 0, "task1", "v1", "d1", t0, modelUtil.Success200)
	modelUtil.InsertTask(s.Suite, "p4", "r1", "task_id_13", 0, "task1", "v1", "d1", t0, modelUtil.TestFailed)
	modelUtil.InsertTask(s.Suite, "p4", "r1", "task_id_14", 0, "task1", "v1", "d1", t0, modelUtil.SystemFailed)
	modelUtil.InsertTask(s.Suite, "p4", "r1", "task_id_15", 0, "task1", "v1", "d1", t0, modelUtil.SystemFailed)
	modelUtil.InsertTask(s.Suite, "p4", "r1", "task_id_16", 0, "task1", "v1", "d1", t0, modelUtil.SetupFailed)
	modelUtil.InsertTask(s.Suite, "p4", "r1", "task_id_17", 0, "task1", "v1", "d1", t0, modelUtil.SetupFailed)
	modelUtil.InsertTask(s.Suite, "p4", "r1", "task_id_18", 0, "task1", "v1", "d1", t0, modelUtil.SetupFailed)
	modelUtil.InsertTask(s.Suite, "p4", "r1", "task_id_19", 0, "task1", "v1", "d1", t0, modelUtil.Timeout)
	modelUtil.InsertTask(s.Suite, "p4", "r1", "task_id_20", 0, "task1", "v1", "d1", t0, modelUtil.Timeout)
}

func (s *statsSuite) initTasksToUpdate() {
	modelUtil.InsertFinishedTask(s.Suite, "p5", "r1", "task1", commit1, finish1, 0)
	modelUtil.InsertFinishedTask(s.Suite, "p5", "r2", "task2", commit2, finish1, 0)
	modelUtil.InsertFinishedTask(s.Suite, "p5", "r2", "task2bis", commit2, finish1, 0)
	modelUtil.InsertFinishedOldTask(s.Suite, "p5", "r2", "task2old", commit2, finish1)
	modelUtil.InsertFinishedTask(s.Suite, "p5", "r1", "task3", commit1, finish2, 0)
	modelUtil.InsertFinishedTask(s.Suite, "p5", "r1", "task4", commit2, finish2, 0)
}

func createTestStatsId(project string, requester string, testFile string, taskName string, variant string, distro string, date time.Time) bson.D {
	return bson.D{
		{Name: "test_file", Value: testFile},
		{Name: "task_name", Value: taskName},
		{Name: "variant", Value: variant},
		{Name: "distro", Value: distro},
		{Name: "project", Value: project},
		{Name: "requester", Value: requester},
		{Name: "date", Value: date},
	}
}

/////////////////////////////////////
// Methods to access database data //
/////////////////////////////////////

func (s *statsSuite) countDocs(collection string) int {
	count, err := db.Count(collection, bson.M{})
	s.Require().NoError(err)
	return count
}

func (s *statsSuite) countDailyTestDocs() int {
	return s.countDocs(dailyTestStatsCollection)
}

func (s *statsSuite) countHourlyTestDocs() int {
	return s.countDocs(hourlyTestStatsCollection)
}

func (s *statsSuite) countDailyTaskDocs() int {
	return s.countDocs(dailyTaskStatsCollection)
}
