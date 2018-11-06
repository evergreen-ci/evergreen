package testutil

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

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
