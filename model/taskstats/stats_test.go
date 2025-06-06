package taskstats

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

var baseTime = time.Date(2018, 7, 15, 16, 45, 0, 0, time.UTC)
var baseHour = time.Date(2018, 7, 15, 16, 0, 0, 0, time.UTC)
var baseDay = time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC)
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
		DailyStatsStatusCollection,
		DailyTaskStatsCollection,
		task.Collection,
	}

	for _, coll := range collectionsToClear {
		s.NoError(db.Clear(coll))
	}
}

func (s *statsSuite) TestStatsStatus() {
	// Check that we get a default status when there is no doc in the database.
	status, err := GetStatsStatus(s.T().Context(), "p1")
	s.NoError(err)
	s.NotNil(status)
	// The default value is rounded off to the day so use a delta of over one day to cover all cases.
	oneDayOneMinute := 24*time.Hour + time.Minute
	expected := time.Now().Add(-defaultBackFillPeriod)
	s.WithinDuration(expected, status.LastJobRun, oneDayOneMinute)
	s.WithinDuration(expected, status.ProcessedTasksUntil, oneDayOneMinute)

	// Check that we can update the status and read the new values.
	err = UpdateStatsStatus(s.T().Context(), "p1", baseHour, baseDay, time.Hour)
	s.NoError(err)

	status, err = GetStatsStatus(s.T().Context(), "p1")
	s.NoError(err)
	s.NotNil(status)
	s.Equal(baseHour.UTC(), status.LastJobRun.UTC())
	s.Equal(baseDay.UTC(), status.ProcessedTasksUntil.UTC())
}

func (s *statsSuite) TestGenerateStats() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Insert task docs.
	s.initTasks()

	// Generate task stats for project p1 and an unknown task.
	s.Require().NoError(GenerateStats(ctx, GenerateStatsOptions{
		ProjectID: "p1",
		Requester: "r1",
		Date:      baseHour,
		Tasks:     []string{"unknown_task"},
	}))
	s.Equal(0, s.countDailyTaskDocs(s.T().Context()))

	// Generate task stats for project p1.
	s.Require().NoError(GenerateStats(ctx, GenerateStatsOptions{
		ProjectID: "p1",
		Requester: "r1",
		Date:      baseHour,
		Tasks:     []string{"task1", "task2"},
	}))
	s.Equal(3, s.countDailyTaskDocs(s.T().Context()))
	doc, err := GetDailyTaskDoc(s.T().Context(), DBTaskStatsID{
		Project:      "p1",
		Requester:    "r1",
		TaskName:     "task1",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseDay,
	})
	s.Require().NoError(err)
	s.NotNil(doc)
	doc, err = GetDailyTaskDoc(s.T().Context(), DBTaskStatsID{
		Project:      "p1",
		Requester:    "r1",
		TaskName:     "task1",
		BuildVariant: "v2",
		Distro:       "d1",
		Date:         baseDay,
	})
	s.Require().NoError(err)
	s.NotNil(doc)
	doc, err = GetDailyTaskDoc(s.T().Context(), DBTaskStatsID{
		Project:      "p1",
		Requester:    "r1",
		TaskName:     "task2",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseDay,
	})
	s.Require().NoError(err)
	s.NotNil(doc)

	// Generate task stats for project p4 to check status aggregation.
	s.Require().NoError(GenerateStats(ctx, GenerateStatsOptions{ProjectID: "p4", Requester: "r1", Date: baseHour, Tasks: []string{"task1"}}))
	s.Equal(4, s.countDailyTaskDocs(s.T().Context())) // 1 more task combination was added to the collection.
	doc, err = GetDailyTaskDoc(s.T().Context(), DBTaskStatsID{
		Project:      "p4",
		Requester:    "r1",
		TaskName:     "task1",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseDay,
	})
	s.Require().NoError(err)
	s.Require().NotNil(doc)
	s.Equal(2, doc.NumSuccess)
	s.Equal(8, doc.NumFailed)
	s.Equal(1, doc.NumTestFailed)
	s.Equal(2, doc.NumSystemFailed)
	s.Equal(3, doc.NumSetupFailed)
	s.Equal(2, doc.NumTimeout)
	//nolint:testifylint // We expect it to be exactly 150.0.
	s.Equal(float64(150), doc.AvgDurationSuccess)
	s.WithinDuration(time.Now(), doc.LastUpdate, time.Minute)

	// Generate task for project p2
	s.Require().NoError(GenerateStats(ctx, GenerateStatsOptions{ProjectID: "p2", Requester: "r1", Date: baseHour, Tasks: []string{"task1"}}))
	s.Equal(5, s.countDailyTaskDocs(s.T().Context())) // 1 more task combination was added to the collection.
	doc, err = GetDailyTaskDoc(s.T().Context(), DBTaskStatsID{
		Project:      "p2",
		Requester:    "r1",
		TaskName:     "task1",
		BuildVariant: "v1",
		Distro:       "d1",
		Date:         baseDay,
	})
	s.Require().NoError(err)
	s.Require().NotNil(doc)
	s.Equal(0, doc.NumSuccess)
	s.Equal(2, doc.NumFailed)
	s.Equal(2, doc.NumTestFailed)
	s.Equal(0, doc.NumSystemFailed)
	s.Equal(0, doc.NumSetupFailed)
	s.Equal(0, doc.NumTimeout)
	s.WithinDuration(time.Now(), doc.LastUpdate, time.Minute)
}

func TestGetUpdateWindow(t *testing.T) {
	startTime := time.Now()

	t.Run("Within12HourWindow", func(t *testing.T) {
		processedTasksUntil := startTime.Add(-10 * time.Hour)
		status := StatsStatus{ProcessedTasksUntil: processedTasksUntil}
		start, end := status.GetUpdateWindow()
		assert.WithinDuration(t, processedTasksUntil, start, time.Minute)
		assert.WithinDuration(t, time.Now(), end, time.Minute)
	})
	t.Run("24HourWindowShouldBeCapped", func(t *testing.T) {
		processedTasksUntil := startTime.Add(-24 * time.Hour)
		status := StatsStatus{ProcessedTasksUntil: processedTasksUntil}
		start, end := status.GetUpdateWindow()
		assert.WithinDuration(t, processedTasksUntil, start, time.Minute)
		assert.WithinDuration(t, processedTasksUntil.Add(12*time.Hour), end, time.Minute)
	})
	t.Run("2DayWindowShouldBeCapped", func(t *testing.T) {
		processedTasksUntil := startTime.Add(-2 * 24 * time.Hour)
		status := StatsStatus{ProcessedTasksUntil: processedTasksUntil}
		start, end := status.GetUpdateWindow()
		assert.WithinDuration(t, processedTasksUntil, start, time.Minute)
		assert.WithinDuration(t, processedTasksUntil.Add(12*time.Hour), end, time.Minute)
	})
}

func (s *statsSuite) TestFindStatsToUpdate() {
	// Insert task docs.
	s.initTasksToUpdate()

	// Find stats for p5 for a period with no finished tasks.
	start := baseHour
	end := baseHour.Add(time.Hour)
	statsList, err := FindStatsToUpdate(s.T().Context(), FindStatsToUpdateOptions{ProjectID: "p5", Requesters: nil, Start: start, End: end})
	s.Require().NoError(err)
	s.Empty(statsList)

	// Find stats for p5 for a period around finish1.
	start = finish1.Add(-1 * time.Hour)
	end = finish1.Add(time.Hour)
	statsList, err = FindStatsToUpdate(s.T().Context(), FindStatsToUpdateOptions{ProjectID: "p5", Requesters: nil, Start: start, End: end})
	s.Require().NoError(err)
	s.Len(statsList, 2)

	// Find stats for p5 for a period around finished1, filtering by
	// requester.
	statsList, err = FindStatsToUpdate(s.T().Context(), FindStatsToUpdateOptions{ProjectID: "p5", Requesters: []string{"r2"}, Start: start, End: end})
	s.Require().NoError(err)
	s.Len(statsList, 1)
	statsList, err = FindStatsToUpdate(s.T().Context(), FindStatsToUpdateOptions{ProjectID: "p5", Requesters: []string{"r1", "r2"}, Start: start, End: end})
	s.Require().NoError(err)
	s.Require().Len(statsList, 2)

	// The results are not sorted so we can't make any assumptions about the order.
	for _, stats := range statsList {
		if stats.Requester == "r1" {
			s.Equal(utility.GetUTCDay(commit1), stats.Day)
			s.Equal([]string{"task1"}, stats.Tasks)
		} else if stats.Requester == "r2" {
			s.Equal(utility.GetUTCDay(commit2), stats.Day)
			s.Require().Len(stats.Tasks, 2)
			s.Contains(stats.Tasks, "task2")
			s.Contains(stats.Tasks, "task2bis")
		} else {
			s.Fail("Unexpected requester")
		}
	}

	// Find stats for p5 for a period around finish1
	start = finish1.Add(-1 * time.Hour)
	end = finish1.Add(time.Hour)
	statsList, err = FindStatsToUpdate(s.T().Context(), FindStatsToUpdateOptions{ProjectID: "p5", Requesters: nil, Start: start, End: end})
	s.Require().NoError(err)
	s.Require().Len(statsList, 2)

	// The results are not sorted so we can't make any assumptions about the order.
	for _, stats := range statsList {
		if stats.Requester == "r1" {
			s.Equal(utility.GetUTCDay(commit1), stats.Day)
			s.Equal([]string{"task1"}, stats.Tasks)
		} else if stats.Requester == "r2" {
			s.Equal(utility.GetUTCDay(commit2), stats.Day)
			s.Require().Len(stats.Tasks, 2)
			s.Contains(stats.Tasks, "task2")
			s.Contains(stats.Tasks, "task2bis")
		} else {
			s.Fail("Unexpected requester")
		}
	}
}

/////////////////////////////////////////
// Methods to initialize database data //
/////////////////////////////////////////

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
	s.insertTask("p1", "r1", "task_id_1", 0, "task1", "v1", "d1", t0, testFailed)
	// Task on variant v2
	s.insertTask("p1", "r1", "task_id_2", 0, "task1", "v2", "d1", t0, testFailed)
	// Task with different task name
	s.insertTask("p1", "r1", "task_id_3", 0, "task2", "v1", "d1", t0, testFailed)
	// Task 10 minutes later
	s.insertTask("p1", "r1", "task_id_4", 0, "task1", "v1", "d1", t0plus10m, testFailed)
	// Task 1 hour later
	s.insertTask("p1", "r1", "task_id_5", 0, "task1", "v1", "d1", t0plus1h, testFailed)
	// Task different requester
	s.insertTask("p1", "r2", "task_id_6", 0, "task1", "v1", "d1", t0, testFailed)
	// Task different project
	s.insertTask("p2", "r1", "task_id_7", 0, "task1", "v1", "d1", t0, testFailed)
	// Task with old executions.
	s.insertTask("p2", "r1", "task_id_8", 2, "task1", "v1", "d1", t0plus10m, testFailed)
	// Execution task
	s.insertTask("p3", "r1", "task_id_9", 0, "task_exec_1", "v1", "d1", t0, testFailed)
	// Display task
	s.insertDisplayTask("p3", "r1", "task_id_10", 0, "task_display_1", "v1", "d1", t0, testFailed, []string{"task_id_9"})
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
	s.insertTask("p5", "r1", "task_id_5_1", 0, "task1", "v1", "d1", t0, success100)
	s.insertTask("p5", "r1", "task_id_5_2", 0, "task1", "v1", "d1", t0, testFailed)
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
	err := newTask.Insert(s.T().Context())
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
	err := newTask.Insert(s.T().Context())
	s.Require().NoError(err)
	return newTask
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
	err := newTask.Insert(s.T().Context())
	s.Require().NoError(err)
}

/////////////////////////////////////
// Methods to access database data //
/////////////////////////////////////

func (s *statsSuite) countDocs(ctx context.Context, collection string) int {
	count, err := db.Count(ctx, collection, bson.M{})
	s.Require().NoError(err)
	return count
}

func (s *statsSuite) countDailyTaskDocs(ctx context.Context) int {
	return s.countDocs(ctx, DailyTaskStatsCollection)
}
