package stats

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	adb "github.com/mongodb/anser/db"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

var baseTime = time.Date(2018, 7, 15, 16, 45, 0, 0, time.UTC)
var baseHour = time.Date(2018, 7, 15, 16, 0, 0, 0, time.UTC)
var baseDay = time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC)
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

func (s *statsSuite) SetupTest() {
	collectionsToClear := []string{
		HourlyTestStatsCollection,
		DailyTestStatsCollection,
		DailyStatsStatusCollection,
		DailyTaskStatsCollection,
		task.Collection,
		testresult.Collection,
	}

	for _, coll := range collectionsToClear {
		s.Nil(db.Clear(coll))
	}
}

func (s *statsSuite) TestStatsStatus() {

	// Check that we get a default status when there is no doc in the database.
	status, err := GetStatsStatus("p1")
	s.NoError(err)
	s.NotNil(status)
	// The default value is rounded off to the day so use a delta of over one day to cover all cases.
	oneDayOneMinute := 24*time.Hour + time.Minute
	expected := time.Now().Add(-defaultBackFillPeriod)
	s.WithinDuration(expected, status.LastJobRun, oneDayOneMinute)
	s.WithinDuration(expected, status.ProcessedTasksUntil, oneDayOneMinute)

	// Check that we can update the status and read the new values.
	err = UpdateStatsStatus("p1", baseHour, baseDay, time.Hour)
	s.NoError(err)

	status, err = GetStatsStatus("p1")
	s.NoError(err)
	s.NotNil(status)
	s.Equal(baseHour.UTC(), status.LastJobRun.UTC())
	s.Equal(baseDay.UTC(), status.ProcessedTasksUntil.UTC())
}

func (s *statsSuite) TestGenerateHourlyTestStats() {
	rand.Seed(314159265)

	// Insert task docs.
	s.initTasks()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Generate hourly stats for project p1 and an unknown task.
	err := GenerateHourlyTestStats(ctx, GenerateOptions{
		ProjectID: "p1",
		Requester: "r1",
		Window:    baseHour,
		Tasks:     []string{"unknown_task"},
		Runtime:   jobTime})
	s.NoError(err)
	s.Equal(0, s.countHourlyTestDocs())

	// Generate hourly stats for project p1.
	err = GenerateHourlyTestStats(ctx, GenerateOptions{
		ProjectID: "p1",
		Requester: "r1",
		Window:    baseHour,
		Tasks:     []string{"task1"},
		Runtime:   jobTime})
	s.NoError(err)
	s.Equal(5, s.countHourlyTestDocs())

	testStatsID := DbTestStatsId{
		Project:      "p1",
		Requester:    "r1",
		TestFile:     "test1.js",
		TaskName:     "task1",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseHour,
	}
	doc, err := GetHourlyTestDoc(testStatsID)

	s.NoError(err)
	s.NotNil(doc)
	s.Equal("p1", doc.Id.Project)
	s.Equal("r1", doc.Id.Requester)
	s.Equal("test1.js", doc.Id.TestFile)
	s.Equal("task1", doc.Id.TaskName)
	s.Equal("v1", doc.Id.BuildVariant)
	s.Equal("d1", doc.Id.Distro)
	s.Equal(baseHour.UTC(), doc.Id.Date.UTC())
	s.Equal(0, doc.NumPass)
	s.Equal(2, doc.NumFail)
	s.Equal(float64(0), doc.AvgDurationPass)
	s.WithinDuration(jobTime, doc.LastUpdate, 0)

	var lastTestResult *testresult.TestResult
	lastTestResult, err = s.getLastTestResult(testStatsID)
	s.NoError(err)
	s.Equal(lastTestResult.ID, doc.LastID)

	testStatsID = DbTestStatsId{
		Project:      "p1",
		Requester:    "r1",
		TestFile:     "test2.js",
		TaskName:     "task1",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseHour,
	}
	doc, err = GetHourlyTestDoc(testStatsID)
	s.NoError(err)
	s.NotNil(doc)
	s.Equal(1, doc.NumPass)
	s.Equal(1, doc.NumFail)
	s.Equal(float64(120), doc.AvgDurationPass)

	lastTestResult, err = s.getLastTestResult(testStatsID)
	s.NoError(err)
	s.Equal(lastTestResult.ID, doc.LastID)

	testStatsID = DbTestStatsId{
		Project:      "p1",
		Requester:    "r1",
		TestFile:     "test3.js",
		TaskName:     "task1",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseHour,
	}
	doc, err = GetHourlyTestDoc(testStatsID)
	s.NoError(err)
	s.NotNil(doc)
	s.Equal(2, doc.NumPass)
	s.Equal(0, doc.NumFail)
	s.Equal(float64(12.5), doc.AvgDurationPass)

	lastTestResult, err = s.getLastTestResult(testStatsID)
	s.NoError(err)
	s.Equal(lastTestResult.ID, doc.LastID)

	// Generate hourly stats for project p3.
	// Testing display task / execution task.
	err = GenerateHourlyTestStats(ctx, GenerateOptions{
		ProjectID: "p3",
		Requester: "r1",
		Window:    baseHour,
		Tasks:     []string{"task_exec_1"},
		Runtime:   jobTime,
	})
	s.NoError(err)
	s.Equal(7, s.countHourlyTestDocs()) // 2 more tests combination were added to the collection

	testStatsID = DbTestStatsId{
		Project:      "p3",
		Requester:    "r1",
		TestFile:     "test1.js",
		TaskName:     "task_exec_1",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseHour,
	}
	doc, err = GetHourlyTestDoc(testStatsID)
	s.NoError(err)
	s.Nil(doc)

	testStatsID = DbTestStatsId{
		Project:      "p3",
		Requester:    "r1",
		TestFile:     "test1.js",
		TaskName:     "task_display_1",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseHour,
	}
	doc, err = GetHourlyTestDoc(testStatsID)
	s.NoError(err)
	s.NotNil(doc)
	s.Equal(0, doc.NumPass)
	s.Equal(1, doc.NumFail)
	s.Equal(float64(0), doc.AvgDurationPass)

	lastTestResult, err = s.getLastTestResult(testStatsID)
	s.NoError(err)
	s.Equal(lastTestResult.ID, doc.LastID)

	testStatsID = DbTestStatsId{
		Project:      "p3",
		Requester:    "r1",
		TestFile:     "test2.js",
		TaskName:     "task_display_1",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseHour,
	}
	doc, err = GetHourlyTestDoc(testStatsID)
	s.NoError(err)
	s.NotNil(doc)
	s.Equal(1, doc.NumPass)
	s.Equal(0, doc.NumFail)
	s.Equal(float64(120), doc.AvgDurationPass)

	lastTestResult, err = s.getLastTestResult(testStatsID)
	s.NoError(err)
	s.Equal(lastTestResult.ID, doc.LastID)

	// Generate hourly stats for project p5.
	// Testing tests with status 'skip'.
	err = GenerateHourlyTestStats(ctx, GenerateOptions{
		ProjectID: "p5",
		Requester: "r1",
		Window:    baseHour,
		Tasks:     []string{"task1", "task2"},
		Runtime:   jobTime})
	s.NoError(err)
	s.Equal(9, s.countHourlyTestDocs()) // 2 more tests combination were added to the collection.

	// test1.js passed once and was skipped once.
	testStatsID = DbTestStatsId{
		Project:      "p5",
		Requester:    "r1",
		TestFile:     "test1.js",
		TaskName:     "task1",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseHour,
	}
	doc, err = GetHourlyTestDoc(testStatsID)
	s.NoError(err)
	s.NotNil(doc)
	s.Equal(1, doc.NumPass)
	s.Equal(0, doc.NumFail)
	s.Equal(float64(60), doc.AvgDurationPass)

	lastTestResult, err = s.getLastTestResult(testStatsID)
	s.NoError(err)
	s.Equal(lastTestResult.ID, doc.LastID)

	// test2.js failed once and was skipped once.
	testStatsID = DbTestStatsId{
		Project:      "p5",
		Requester:    "r1",
		TestFile:     "test2.js",
		TaskName:     "task1",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseHour,
	}
	doc, err = GetHourlyTestDoc(testStatsID)
	s.NoError(err)
	s.NotNil(doc)
	s.Equal(0, doc.NumPass)
	s.Equal(1, doc.NumFail)
	s.Equal(float64(0), doc.AvgDurationPass)

	lastTestResult, err = s.getLastTestResult(testStatsID)
	s.NoError(err)
	s.Equal(lastTestResult.ID, doc.LastID)
}

func (s *statsSuite) TestGenerateDailyTestStatsFromHourly() {
	rand.Seed(314159265)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Insert hourly test stats docs.
	s.initHourly()
	// Generate daily test stats for unknown task.
	err := GenerateDailyTestStatsFromHourly(ctx, GenerateOptions{
		ProjectID: "p1",
		Requester: "r1",
		Window:    baseDay,
		Tasks:     []string{"unknown_task"},
		Runtime:   jobTime})
	s.NoError(err)
	s.Equal(0, s.countDailyTestDocs())

	// Generate daily test stats for exiting task
	err = GenerateDailyTestStatsFromHourly(ctx, GenerateOptions{ProjectID: "p1", Requester: "r1", Window: baseDay, Tasks: []string{"task1"}, Runtime: jobTime})
	s.NoError(err)
	s.Equal(1, s.countDailyTestDocs())

	testStatsID := DbTestStatsId{
		Project:      "p1",
		Requester:    "r1",
		TestFile:     "test1.js",
		TaskName:     "task1",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseDay,
	}
	doc, err := GetDailyTestDoc(testStatsID)
	s.Nil(err)
	s.NotNil(doc)
	s.Equal("p1", doc.Id.Project)
	s.Equal("r1", doc.Id.Requester)
	s.Equal("test1.js", doc.Id.TestFile)
	s.Equal("task1", doc.Id.TaskName)
	s.Equal("v1", doc.Id.BuildVariant)
	s.Equal("d1", doc.Id.Distro)
	s.Equal(baseDay.UTC(), doc.Id.Date.UTC())
	s.Equal(30, doc.NumPass)
	s.Equal(5, doc.NumFail)
	s.Equal(float64(4), doc.AvgDurationPass)
	s.WithinDuration(jobTime, doc.LastUpdate, 0)

	var lastTestResult *dbTestStats
	lastTestResult, err = s.getLastHourlyTestStat(testStatsID)
	s.NoError(err)
	s.Equal(lastTestResult.LastID, doc.LastID)
}

func (s *statsSuite) TestGenerateDailyTaskStats() {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Insert task docs.
	s.initTasks()

	// Generate task stats for project p1 and an unknown task.
	err := GenerateDailyTaskStats(ctx, GenerateOptions{
		ProjectID: "p1",
		Requester: "r1",
		Window:    baseHour,
		Tasks:     []string{"unknown_task"},
		Runtime:   jobTime})
	s.NoError(err)
	s.Equal(0, s.countDailyTaskDocs())

	// Generate task stats for project p1.
	err = GenerateDailyTaskStats(ctx, GenerateOptions{
		ProjectID: "p1",
		Requester: "r1",
		Window:    baseHour,
		Tasks:     []string{"task1", "task2"},
		Runtime:   jobTime})
	s.NoError(err)
	s.Equal(3, s.countDailyTaskDocs())
	doc, err := GetDailyTaskDoc(DbTaskStatsId{
		Project:      "p1",
		Requester:    "r1",
		TaskName:     "task1",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseDay,
	})
	s.NoError(err)
	s.NotNil(doc)
	doc, err = GetDailyTaskDoc(DbTaskStatsId{
		Project:      "p1",
		Requester:    "r1",
		TaskName:     "task1",
		BuildVariant: "v2",
		Distro:       "d1",
		Date:         baseDay,
	})
	s.NoError(err)
	s.NotNil(doc)
	doc, err = GetDailyTaskDoc(DbTaskStatsId{
		Project:      "p1",
		Requester:    "r1",
		TaskName:     "task2",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseDay,
	})
	s.NoError(err)
	s.NotNil(doc)

	// Generate task stats for project p4 to check status aggregation
	err = GenerateDailyTaskStats(ctx, GenerateOptions{ProjectID: "p4", Requester: "r1", Window: baseHour, Tasks: []string{"task1"}, Runtime: jobTime})
	s.NoError(err)
	s.Equal(4, s.countDailyTaskDocs()) // 1 more task combination was added to the collection
	doc, err = GetDailyTaskDoc(DbTaskStatsId{
		Project:      "p4",
		Requester:    "r1",
		TaskName:     "task1",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseDay,
	})
	s.NoError(err)
	s.NotNil(doc)
	s.Equal(2, doc.NumSuccess)
	s.Equal(8, doc.NumFailed)
	s.Equal(1, doc.NumTestFailed)
	s.Equal(2, doc.NumSystemFailed)
	s.Equal(3, doc.NumSetupFailed)
	s.Equal(2, doc.NumTimeout)
	s.Equal(float64(150), doc.AvgDurationSuccess)
	s.WithinDuration(jobTime, doc.LastUpdate, 0)

	// Generate task for project p2
	err = GenerateDailyTaskStats(ctx, GenerateOptions{ProjectID: "p2", Requester: "r1", Window: baseHour, Tasks: []string{"task1"}, Runtime: jobTime})
	s.NoError(err)
	s.Equal(5, s.countDailyTaskDocs()) // 1 more task combination was added to the collection
	doc, err = GetDailyTaskDoc(DbTaskStatsId{
		Project:      "p2",
		Requester:    "r1",
		TaskName:     "task1",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseDay,
	})
	s.NoError(err)
	s.NotNil(doc)
	s.Equal(0, doc.NumSuccess)
	s.Equal(2, doc.NumFailed)
	s.Equal(2, doc.NumTestFailed)
	s.Equal(0, doc.NumSystemFailed)
	s.Equal(0, doc.NumSetupFailed)
	s.Equal(0, doc.NumTimeout)
	s.WithinDuration(jobTime, doc.LastUpdate, 0)
}

func (s *statsSuite) TestFindStatsToUpdate() {

	// Insert task docs.
	s.initTasksToUpdate()

	// Find stats for p5 for a period with no finished tasks
	start := baseHour
	end := baseHour.Add(time.Hour)
	statsList, err := FindStatsToUpdate(FindStatsOptions{ProjectID: "p5", Requesters: nil, Start: start, End: end})
	s.NoError(err)
	s.Len(statsList, 0)

	// Find stats for p5 for a period around finish1
	start = finish1.Add(-1 * time.Hour)
	end = finish1.Add(time.Hour)
	statsList, err = FindStatsToUpdate(FindStatsOptions{ProjectID: "p5", Requesters: nil, Start: start, End: end})
	s.NoError(err)
	s.Len(statsList, 2)

	// Find stats for p5 for a period around finished1, filtering
	// by requester
	statsList, err = FindStatsToUpdate(FindStatsOptions{ProjectID: "p5", Requesters: []string{"r2"}, Start: start, End: end})
	s.NoError(err)
	s.Len(statsList, 1)
	statsList, err = FindStatsToUpdate(FindStatsOptions{ProjectID: "p5", Requesters: []string{"r1", "r2"}, Start: start, End: end})
	s.NoError(err)
	s.Len(statsList, 2)

	// The results are sorted so we know the order
	s.Equal("p5", statsList[0].ProjectId)
	s.Equal("r1", statsList[0].Requester)
	s.WithinDuration(utility.GetUTCHour(commit1), statsList[0].Hour, 0)
	s.WithinDuration(utility.GetUTCDay(commit1), statsList[0].Day, 0)
	s.Equal([]string{"task1"}, statsList[0].Tasks)

	// Find stats for p5 for a period around finish1
	start = finish1.Add(-1 * time.Hour)
	end = finish1.Add(time.Hour)
	statsList, err = FindStatsToUpdate(FindStatsOptions{ProjectID: "p5", Requesters: nil, Start: start, End: end})
	s.NoError(err)
	s.Len(statsList, 2)
	// The results are sorted so we know the order
	s.Equal("p5", statsList[0].ProjectId)
	s.Equal("r1", statsList[0].Requester)
	s.WithinDuration(utility.GetUTCHour(commit1), statsList[0].Hour, 0)
	s.WithinDuration(utility.GetUTCDay(commit1), statsList[0].Day, 0)
	s.Equal([]string{"task1"}, statsList[0].Tasks)
	s.Equal("p5", statsList[1].ProjectId)
	s.Equal("r2", statsList[1].Requester)
	s.WithinDuration(utility.GetUTCHour(commit2), statsList[1].Hour, 0)
	s.WithinDuration(utility.GetUTCDay(commit2), statsList[1].Day, 0)
	s.Len(statsList[1].Tasks, 2)
	s.Contains(statsList[1].Tasks, "task2")
	s.Contains(statsList[1].Tasks, "task2bis")
}

func (s *statsSuite) TestStatsToUpdate() {

	stats1 := StatsToUpdate{"p1", "r1", baseHour, baseDay, []string{"task1", "task2"}}
	stats1bis := StatsToUpdate{"p1", "r1", baseHour, baseDay, []string{"task1", "task"}}
	stats1later := StatsToUpdate{"p1", "r1", baseHour.Add(time.Hour), baseDay, []string{"task1", "task2"}}
	stats1r2 := StatsToUpdate{"p1", "r2", baseHour, baseDay, []string{"task1", "task2"}}
	stats2 := StatsToUpdate{"p2", "r1", baseHour, baseDay, []string{"task1", "task2"}}

	// canMerge
	s.True(stats1.canMerge(&stats1))
	s.True(stats1.canMerge(&stats1bis))
	s.False(stats1.canMerge(&stats1later))
	s.False(stats1.canMerge(&stats1r2))
	// comparison
	s.True(stats1.lt(&stats2))
	s.True(stats1.lt(&stats1later))
	s.True(stats1.lt(&stats1r2))
	s.False(stats1.lt(&stats1))
	s.False(stats1.lt(&stats1bis))
	// merge
	merged := stats1.merge(&stats1bis)
	s.Equal(merged.ProjectId, stats1.ProjectId)
	s.Equal(merged.Requester, stats1.Requester)
	s.Equal(merged.Hour, stats1.Hour)
	s.Equal(merged.Day, stats1.Day)
	for _, t := range stats1.Tasks {
		s.Contains(merged.Tasks, t)
	}
	for _, t := range stats1bis.Tasks {
		s.Contains(merged.Tasks, t)
	}
}

/////////////////////////////////////////
// Methods to initialize database data //
/////////////////////////////////////////

func (s *statsSuite) initHourly() {
	hour1 := baseHour
	hour2 := baseHour.Add(time.Hour)
	hour3 := baseHour.Add(24 * time.Hour)
	s.insertHourlyTestStats("p1", "r1", "test1.js", "task1", "v1", "d1", hour1, 10, 5, 2, mgobson.NewObjectIdWithTime(hour1.Add(-time.Hour)))
	s.insertHourlyTestStats("p1", "r1", "test1.js", "task1", "v1", "d1", hour2, 20, 0, 5, mgobson.NewObjectIdWithTime(hour2.Add(-time.Hour)))
	s.insertHourlyTestStats("p1", "r1", "test1.js", "task1", "v1", "d1", hour3, 20, 0, 5, mgobson.NewObjectIdWithTime(hour3.Add(-time.Hour)))
}

func (s *statsSuite) insertHourlyTestStats(project string, requester string, testFile string, taskName string, variant string, distro string, date time.Time, numPass int, numFail int, avgDuration float64, lastID mgobson.ObjectId) {

	err := db.Insert(HourlyTestStatsCollection, bson.M{
		"_id": DbTestStatsId{
			Project:      project,
			Requester:    requester,
			TestFile:     testFile,
			TaskName:     taskName,
			BuildVariant: variant,
			Distro:       distro,
			Date:         date,
		},
		"num_pass":          numPass,
		"num_fail":          numFail,
		"avg_duration_pass": avgDuration,
		"last_id":           lastID,
	})
	s.Require().NoError(err)
}

type taskStatus struct {
	Status         string
	DetailsType    string
	DetailsTimeout bool
	TimeTaken      time.Duration
}

func (s *statsSuite) initTasks() {
	t0 := baseTime
	t0plus10m := baseTime.Add(10 * time.Minute)
	t0plus1h := baseTime.Add(time.Hour)
	success100 := taskStatus{"success", "test", false, 100 * 1000 * 1000 * 1000}
	success200 := taskStatus{"success", "test", false, 200 * 1000 * 1000 * 1000}
	testFailed := taskStatus{"failed", "test", false, 20}
	timeout := taskStatus{"failed", "test", true, 100}
	setupFailed := taskStatus{"failed", "setup", false, 10}
	systemFailed := taskStatus{"failed", "system", false, 10}

	// Task
	task := s.insertTask("p1", "r1", "task_id_1", 0, "task1", "v1", "d1", t0, testFailed)
	s.insertTestResult("task_id_1", 0, "test1.js", evergreen.TestFailedStatus, 60, &task, nil)
	s.insertTestResult("task_id_1", 0, "test2.js", evergreen.TestFailedStatus, 120, &task, nil)
	s.insertTestResult("task_id_1", 0, "test3.js", evergreen.TestSucceededStatus, 10, &task, nil)
	// Task on variant v2
	task = s.insertTask("p1", "r1", "task_id_2", 0, "task1", "v2", "d1", t0, testFailed)
	s.insertTestResult("task_id_2", 0, "test1.js", evergreen.TestFailedStatus, 60, &task, nil)
	s.insertTestResult("task_id_2", 0, "test2.js", evergreen.TestFailedStatus, 120, &task, nil)
	// Task with different task name
	task = s.insertTask("p1", "r1", "task_id_3", 0, "task2", "v1", "d1", t0, testFailed)
	s.insertTestResult("task_id_3", 0, "test1.js", evergreen.TestFailedStatus, 60, &task, nil)
	s.insertTestResult("task_id_3", 0, "test2.js", evergreen.TestFailedStatus, 120, &task, nil)
	// Task 10 minutes later
	task = s.insertTask("p1", "r1", "task_id_4", 0, "task1", "v1", "d1", t0plus10m, testFailed)
	s.insertTestResult("task_id_4", 0, "test1.js", evergreen.TestFailedStatus, 60, &task, nil)
	s.insertTestResult("task_id_4", 0, "test2.js", evergreen.TestSucceededStatus, 120, &task, nil)
	s.insertTestResult("task_id_4", 0, "test3.js", evergreen.TestSucceededStatus, 15, &task, nil)
	// Task 1 hour later
	task = s.insertTask("p1", "r1", "task_id_5", 0, "task1", "v1", "d1", t0plus1h, testFailed)
	s.insertTestResult("task_id_5", 0, "test1.js", evergreen.TestFailedStatus, 60, &task, nil)
	s.insertTestResult("task_id_5", 0, "test2.js", evergreen.TestFailedStatus, 120, &task, nil)
	// Task different requester
	task = s.insertTask("p1", "r2", "task_id_6", 0, "task1", "v1", "d1", t0, testFailed)
	s.insertTestResult("task_id_6", 0, "test1.js", evergreen.TestFailedStatus, 60, &task, nil)
	s.insertTestResult("task_id_6", 0, "test2.js", evergreen.TestFailedStatus, 120, &task, nil)
	// Task different project
	task = s.insertTask("p2", "r1", "task_id_7", 0, "task1", "v1", "d1", t0, testFailed)
	s.insertTestResult("task_id_7", 0, "test1.js", evergreen.TestFailedStatus, 60, &task, nil)
	s.insertTestResult("task_id_7", 0, "test2.js", evergreen.TestFailedStatus, 120, &task, nil)
	// Task with old executions.
	task = s.insertTask("p2", "r1", "task_id_8", 2, "task1", "v1", "d1", t0plus10m, testFailed)
	s.insertTestResult("task_id_8", 2, "test1.js", evergreen.TestFailedStatus, 60, &task, nil)
	s.insertTestResult("task_id_8", 2, "test2.js", evergreen.TestFailedStatus, 120, &task, nil)
	s.insertTestResult("task_id_8", 0, "test1.js", evergreen.TestFailedStatus, 60, &task, nil)
	s.insertTestResult("task_id_8", 0, "test2.js", evergreen.TestSucceededStatus, 120, &task, nil)
	s.insertTestResult("task_id_8", 0, "testOld.js", evergreen.TestFailedStatus, 120, &task, nil)
	s.insertTestResult("task_id_8", 1, "test1.js", evergreen.TestSucceededStatus, 60, &task, nil)
	s.insertTestResult("task_id_8", 1, "test2.js", evergreen.TestSucceededStatus, 120, &task, nil)
	// Execution task
	task = s.insertTask("p3", "r1", "task_id_9", 0, "task_exec_1", "v1", "d1", t0, testFailed)
	// Display task
	displayTask := s.insertDisplayTask("p3", "r1", "task_id_10", 0, "task_display_1", "v1", "d1", t0, testFailed, []string{"task_id_9"})
	s.insertTestResult("task_id_9", 0, "test1.js", evergreen.TestFailedStatus, 60, &task, &displayTask)
	s.insertTestResult("task_id_9", 0, "test2.js", evergreen.TestSucceededStatus, 120, &task, &displayTask)
	// Project p4 used to test various task statuses
	s.insertTask("p4", "r1", "task_id_11", 0, "task1", "v1", "d1", t0, success100)
	s.insertTask("p4", "r1", "task_id_12", 0, "task1", "v1", "d1", t0, success200)
	s.insertTask("p4", "r1", "task_id_13", 0, "task1", "v1", "d1", t0, testFailed)
	s.insertTask("p4", "r1", "task_id_14", 0, "task1", "v1", "d1", t0, systemFailed)
	s.insertTask("p4", "r1", "task_id_15", 0, "task1", "v1", "d1", t0, systemFailed)
	s.insertTask("p4", "r1", "task_id_16", 0, "task1", "v1", "d1", t0, setupFailed)
	s.insertTask("p4", "r1", "task_id_17", 0, "task1", "v1", "d1", t0, setupFailed)
	s.insertTask("p4", "r1", "task_id_18", 0, "task1", "v1", "d1", t0, setupFailed)
	s.insertTask("p4", "r1", "task_id_19", 0, "task1", "v1", "d1", t0, timeout)
	s.insertTask("p4", "r1", "task_id_20", 0, "task1", "v1", "d1", t0, timeout)
	// Project p5 used to test handling of skipped tests.
	task = s.insertTask("p5", "r1", "task_id_5_1", 0, "task1", "v1", "d1", t0, success100)
	s.insertTestResult("task_id_5_1", 0, "test1.js", evergreen.TestSkippedStatus, 60, &task, nil)
	s.insertTestResult("task_id_5_1", 0, "test2.js", evergreen.TestSkippedStatus, 60, &task, nil)
	task = s.insertTask("p5", "r1", "task_id_5_2", 0, "task1", "v1", "d1", t0, testFailed)
	s.insertTestResult("task_id_5_2", 0, "test1.js", evergreen.TestSucceededStatus, 60, &task, nil)
	s.insertTestResult("task_id_5_2", 0, "test2.js", evergreen.TestFailedStatus, 60, &task, nil)
}

func (s *statsSuite) initTasksToUpdate() {
	s.insertFinishedTask("p5", "r1", "task1", commit1, finish1)
	s.insertFinishedTask("p5", "r2", "task2", commit2, finish1)
	s.insertFinishedTask("p5", "r2", "task2bis", commit2, finish1)
	s.insertFinishedTask("p5", "r1", "task3", commit1, finish2)
	s.insertFinishedTask("p5", "r1", "task4", commit2, finish2)
}

func (s *statsSuite) insertTask(project string, requester string, taskId string, execution int, taskName string, variant string, distro string, createTime time.Time, status taskStatus) task.Task {
	details := apimodels.TaskEndDetail{
		Status:   status.Status,
		Type:     status.DetailsType,
		TimedOut: status.DetailsTimeout,
	}
	newTask := task.Task{
		Id:           taskId,
		Execution:    execution,
		Project:      project,
		DisplayName:  taskName,
		Requester:    requester,
		BuildVariant: variant,
		DistroId:     distro,
		CreateTime:   createTime,
		Status:       status.Status,
		Details:      details,
		TimeTaken:    status.TimeTaken,
	}
	err := newTask.Insert()
	s.Require().NoError(err)
	return newTask
}

func (s *statsSuite) insertDisplayTask(project string, requester string, taskId string, execution int, taskName string, variant string, distro string, createTime time.Time, status taskStatus, executionTasks []string) task.Task {
	details := apimodels.TaskEndDetail{
		Status:   status.Status,
		Type:     status.DetailsType,
		TimedOut: status.DetailsTimeout,
	}
	newTask := task.Task{
		Id:             taskId,
		Execution:      execution,
		Project:        project,
		DisplayName:    taskName,
		Requester:      requester,
		BuildVariant:   variant,
		DistroId:       distro,
		CreateTime:     createTime,
		Status:         status.Status,
		Details:        details,
		TimeTaken:      status.TimeTaken,
		ExecutionTasks: executionTasks}
	err := newTask.Insert()
	s.Require().NoError(err)
	return newTask
}

func (s *statsSuite) insertTestResult(taskId string, execution int, testFile string, status string, durationSeconds int, theExecutionTask *task.Task, theDisplayTask *task.Task) {
	startTime := time.Now()
	endTime := startTime.Add(time.Duration(durationSeconds) * time.Second)
	newTestResult := testresult.TestResult{
		TaskID:    taskId,
		Execution: execution,
		TestFile:  testFile,
		Status:    status,
		StartTime: float64(startTime.Unix()),
		EndTime:   float64(endTime.Unix()),
	}
	if theExecutionTask != nil {
		newTestResult.Project = theExecutionTask.Project
		newTestResult.BuildVariant = theExecutionTask.BuildVariant
		newTestResult.DistroId = theExecutionTask.DistroId
		newTestResult.Requester = theExecutionTask.Requester
		newTestResult.DisplayName = theExecutionTask.DisplayName
		newTestResult.TaskCreateTime = theExecutionTask.CreateTime
	}
	if theDisplayTask != nil {
		newTestResult.ExecutionDisplayName = theDisplayTask.DisplayName
	}
	err := newTestResult.Insert()
	s.Require().NoError(err)
}

func (s *statsSuite) insertFinishedTask(project string, requester string, taskName string, createTime time.Time, finishTime time.Time) {
	newTask := task.Task{
		Id:          mgobson.NewObjectId().Hex(),
		DisplayName: taskName,
		Project:     project,
		Requester:   requester,
		CreateTime:  createTime,
		FinishTime:  finishTime,
	}
	err := newTask.Insert()
	s.Require().NoError(err)
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
	return s.countDocs(DailyTestStatsCollection)
}

func (s *statsSuite) countHourlyTestDocs() int {
	return s.countDocs(HourlyTestStatsCollection)
}

func (s *statsSuite) countDailyTaskDocs() int {
	return s.countDocs(DailyTaskStatsCollection)
}

func (s *statsSuite) getLastTestResult(testStatsID DbTestStatsId) (*testresult.TestResult, error) {
	var lastTestResult *testresult.TestResult
	start := utility.GetUTCHour(testStatsID.Date)
	end := start.Add(time.Hour)

	qry := bson.M{
		testresult.ProjectKey:   testStatsID.Project,
		testresult.RequesterKey: testStatsID.Requester,
		testresult.TestFileKey:  testStatsID.TestFile,
		"$or": []bson.M{
			{testresult.DisplayNameKey: testStatsID.TaskName},
			{testresult.ExecutionDisplayNameKey: testStatsID.TaskName},
		},
		testresult.DistroIdKey:     testStatsID.Distro,
		testresult.BuildVariantKey: testStatsID.BuildVariant,
		testresult.TaskCreateTimeKey: bson.M{
			"$gte": start,
			"$lt":  end,
		},
	}
	q := db.Query(qry).Sort([]string{"-_id"}).Limit(1)
	testResults, err := testresult.Find(q)
	if err != nil || len(testResults) == 0 {
		lastTestResult = nil
	} else {
		lastTestResult = &testResults[0]
	}
	return lastTestResult, err
}

func (s *statsSuite) getLastHourlyTestStat(testStatsID DbTestStatsId) (*dbTestStats, error) {
	testResults := &dbTestStats{}

	start := utility.GetUTCDay(testStatsID.Date)
	end := start.Add(24 * time.Hour)

	q := db.Query(bson.M{
		"_id." + dbTestStatsIdProjectKey:      testStatsID.Project,
		"_id." + dbTestStatsIdRequesterKey:    testStatsID.Requester,
		"_id." + dbTestStatsIdTestFileKey:     testStatsID.TestFile,
		"_id." + DbTestStatsIdTaskNameKey:     testStatsID.TaskName,
		"_id." + DbTestStatsIdDistroKey:       testStatsID.Distro,
		"_id." + DbTestStatsIdBuildVariantKey: testStatsID.BuildVariant,
		"_id." + dbTestStatsIdDateKey: bson.M{
			"$gte": start,
			"$lt":  end,
		},
	}).Sort([]string{"-last_id"})
	err := db.FindOneQ(HourlyTestStatsCollection, q, testResults)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return testResults, err
}
