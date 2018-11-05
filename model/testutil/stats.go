package testutil

import (
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type TaskStatus struct {
	Status         string
	DetailsType    string
	DetailsTimeout bool
	TimeTaken      time.Duration
}

var Success100 = TaskStatus{"success", "test", false, 100 * 1000 * 1000 * 1000}
var Success200 = TaskStatus{"success", "test", false, 200 * 1000 * 1000 * 1000}
var TestFailed = TaskStatus{"failed", "test", false, 20}
var Timeout = TaskStatus{"failed", "test", true, 100}
var SetupFailed = TaskStatus{"failed", "setup", false, 10}
var SystemFailed = TaskStatus{"failed", "system", false, 10}

func ClearCollection(s suite.Suite, name string) {
	err := db.Clear(name)
	s.Require().NoError(err)
}

func InsertTask(s suite.Suite, project string, requester string, taskId string, execution int,
	taskName string, variant string, distro string, createTime time.Time, status TaskStatus) {
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
}

func InsertFinishedTask(s suite.Suite, project string, requester string, taskName string,
	createTime time.Time, finishTime time.Time, execution int) task.Task {
	newTask := task.Task{
		Id:          bson.NewObjectId().Hex(),
		DisplayName: taskName,
		Project:     project,
		Requester:   requester,
		CreateTime:  createTime,
		FinishTime:  finishTime,
		Execution:   execution,
	}
	err := newTask.Insert()
	s.Require().NoError(err)

	return newTask
}

func InsertOldTask(s suite.Suite, project string, requester string, taskId string, execution int,
	taskName string, variant string, distro string, createTime time.Time, status TaskStatus) {
	details := apimodels.TaskEndDetail{
		Status:   status.Status,
		Type:     status.DetailsType,
		TimedOut: status.DetailsTimeout,
	}
	oldTaskId := taskId
	taskId = taskId + "_" + strconv.Itoa(execution)
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
		OldTaskId:    oldTaskId}
	err := db.Insert(task.OldCollection, &newTask)
	s.Require().NoError(err)
}

func InsertFinishedOldTask(s suite.Suite, project string, requester string, taskName string, createTime time.Time, finishTime time.Time) {
	newTask := task.Task{
		Id:          bson.NewObjectId().String(),
		DisplayName: taskName,
		Project:     project,
		Requester:   requester,
		CreateTime:  createTime,
		FinishTime:  finishTime,
	}
	err := db.Insert(task.OldCollection, &newTask)
	s.Require().NoError(err)
}

func InsertDisplayTask(s suite.Suite, project string, requester string, taskId string,
	execution int, taskName string, variant string, distro string, createTime time.Time,
	status TaskStatus, executionTasks []string) {
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
}

func InsertTestResult(s suite.Suite, taskId string, execution int, testFile string, status string,
	durationSeconds int) {
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
	err := newTestResult.Insert()
	s.Require().NoError(err)
}

func createTestStatsId(project string, requester string, testFile string, taskName string,
	variant string, distro string, date time.Time) bson.D {
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

func createTaskStatsId(project string, requester string, taskName string, variant string, distro string, date time.Time) bson.D {
	return bson.D{
		{Name: "task_name", Value: taskName},
		{Name: "variant", Value: variant},
		{Name: "distro", Value: distro},
		{Name: "project", Value: project},
		{Name: "requester", Value: requester},
		{Name: "date", Value: date},
	}
}

func getTestStatsDoc(s suite.Suite, collection string, project string, requester string,
	testFile string, taskName string, variant string, distro string, date time.Time) *dbTestStats {
	doc := dbTestStats{}
	docId := createTestStatsId(project, requester, testFile, taskName, variant, distro, date)
	err := db.FindOne(collection, bson.M{"_id": docId}, db.NoProjection, db.NoSort, &doc)
	if err == mgo.ErrNotFound {
		return nil
	}
	s.Require().NoError(err)
	return &doc
}

func GetDailyTestDoc(s suite.Suite, project string, requester string, testFile string,
	taskName string, variant string, distro string, date time.Time) *dbTestStats {
	return getTestStatsDoc(s, "daily_test_stats", project, requester, testFile, taskName,
		variant, distro, date)
}

func GetHourlyTestDoc(s suite.Suite, project string, requester string, testFile string,
	taskName string, variant string, distro string, date time.Time) *dbTestStats {
	return getTestStatsDoc(s, "hourly_test_stats", project, requester, testFile, taskName,
		variant, distro, date)
}

func GetDailyTaskDoc(s suite.Suite, project string, requester string, taskName string, variant string, distro string, date time.Time) *dbTaskStats {
	doc := dbTaskStats{}
	docId := createTaskStatsId(project, requester, taskName, variant, distro, date)
	err := db.FindOne("daily_task_stats", bson.M{"_id": docId}, db.NoProjection, db.NoSort, &doc)
	if err == mgo.ErrNotFound {
		return nil
	}
	s.Require().NoError(err)
	return &doc
}

////////////////////////////////////////
// Structs to represent database data //
////////////////////////////////////////

type dbTestStatsId struct {
	TestFile  string    `bson:"test_file"`
	TaskName  string    `bson:"task_name"`
	Variant   string    `bson:"variant"`
	Distro    string    `bson:"distro"`
	Project   string    `bson:"project"`
	Requester string    `bson:"requester"`
	Date      time.Time `bson:"date"`
}

type dbTestStats struct {
	Id              dbTestStatsId `bson:"_id"`
	NumPass         int           `bson:"num_pass"`
	NumFail         int           `bson:"num_fail"`
	AvgDurationPass float32       `bson:"avg_duration_pass"`
	LastUpdate      time.Time     `bson:"last_update"`
}

type dbTaskStatsId struct {
	TaskName  string    `bson:"task_name"`
	Variant   string    `bson:"variant"`
	Distro    string    `bson:"distro"`
	Project   string    `bson:"project"`
	Requester string    `bson:"requester"`
	Date      time.Time `bson:"date"`
}

type dbTaskStats struct {
	Id                 dbTaskStatsId `bson:"_id"`
	NumSuccess         int           `bson:"num_success"`
	NumFailed          int           `bson:"num_failed"`
	NumTimeout         int           `bson:"num_timeout"`
	NumTestFailed      int           `bson:"num_test_failed"`
	NumSystemFailed    int           `bson:"num_system_failed"`
	NumSetupFailed     int           `bson:"num_setup_failed"`
	AvgDurationSuccess float32       `bson:"avg_duration_success"`
	LastUpdate         time.Time     `bson:"last_update"`
}
