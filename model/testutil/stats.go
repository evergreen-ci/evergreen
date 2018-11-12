package testutil

import (
	"time"

	"gopkg.in/mgo.v2"

	"github.com/evergreen-ci/evergreen/db"
	"gopkg.in/mgo.v2/bson"
)

func GetDailyTestDoc(id DbTestStatsId) (*dbTestStats, error) {
	doc := dbTestStats{}
	err := db.FindOne("daily_test_stats", bson.M{"_id": id}, db.NoProjection, db.NoSort, &doc)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return &doc, err
}

func GetHourlyTestDoc(id DbTestStatsId) (*dbTestStats, error) {
	doc := dbTestStats{}
	err := db.FindOne("hourly_test_stats", bson.M{"_id": id}, db.NoProjection, db.NoSort, &doc)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return &doc, err
}

func GetDailyTaskDoc(id DbTaskStatsId) (*dbTaskStats, error) {
	doc := dbTaskStats{}
	err := db.FindOne("daily_task_stats", bson.M{"_id": id}, db.NoProjection, db.NoSort, &doc)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return &doc, err
}

////////////////////////////////////////
// Structs to represent database data //
////////////////////////////////////////

type DbTestStatsId struct {
	TestFile  string    `bson:"test_file"`
	TaskName  string    `bson:"task_name"`
	Variant   string    `bson:"variant"`
	Distro    string    `bson:"distro"`
	Project   string    `bson:"project"`
	Requester string    `bson:"requester"`
	Date      time.Time `bson:"date"`
}

type dbTestStats struct {
	Id              DbTestStatsId `bson:"_id"`
	NumPass         int           `bson:"num_pass"`
	NumFail         int           `bson:"num_fail"`
	AvgDurationPass float64       `bson:"avg_duration_pass"`
	LastUpdate      time.Time     `bson:"last_update"`
}

type DbTaskStatsId struct {
	TaskName  string    `bson:"task_name"`
	Variant   string    `bson:"variant"`
	Distro    string    `bson:"distro"`
	Project   string    `bson:"project"`
	Requester string    `bson:"requester"`
	Date      time.Time `bson:"date"`
}

type dbTaskStats struct {
	Id                 DbTaskStatsId `bson:"_id"`
	NumSuccess         int           `bson:"num_success"`
	NumFailed          int           `bson:"num_failed"`
	NumTimeout         int           `bson:"num_timeout"`
	NumTestFailed      int           `bson:"num_test_failed"`
	NumSystemFailed    int           `bson:"num_system_failed"`
	NumSetupFailed     int           `bson:"num_setup_failed"`
	AvgDurationSuccess float64       `bson:"avg_duration_success"`
	LastUpdate         time.Time     `bson:"last_update"`
}
