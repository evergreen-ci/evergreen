package admin

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	Collection       = "admin"
	SystemSettingsID = "0"
)

var (
	IdKey            = bsonutil.MustHaveTag(AdminSettings{}, "Id")
	BannerKey        = bsonutil.MustHaveTag(AdminSettings{}, "Banner")
	TaskDispatchKey  = bsonutil.MustHaveTag(AdminSettings{}, "TaskDispatchDisabled")
	RunnerFlagsKey   = bsonutil.MustHaveTag(AdminSettings{}, "RunnerFlags")
	HostinitKey      = bsonutil.MustHaveTag(RunnerFlags{}, "HostinitDisabled")
	MonitorKey       = bsonutil.MustHaveTag(RunnerFlags{}, "MonitorDisabled")
	NotificationsKey = bsonutil.MustHaveTag(RunnerFlags{}, "NotificationsDisabled")
	AlertsKey        = bsonutil.MustHaveTag(RunnerFlags{}, "AlertsDisabled")
	TaskrunnerKey    = bsonutil.MustHaveTag(RunnerFlags{}, "TaskrunnerDisabled")
	RepotrackerKey   = bsonutil.MustHaveTag(RunnerFlags{}, "RepotrackerDisabled")
	SchedulerKey     = bsonutil.MustHaveTag(RunnerFlags{}, "SchedulerDisabled")
)

var SettingsQuery = db.Query(bson.M{IdKey: SystemSettingsID})

func GetSettingsFromDB() (*AdminSettings, error) {
	settings := &AdminSettings{}
	query := db.Q{}
	query = query.Filter(SettingsQuery)
	err := db.FindOneQ(Collection, query, settings)
	if err != nil {
		return nil, errors.Wrap(err, "Error retrieving admin settings from DB")
	}
	if settings == nil {
		return nil, errors.New("Settings are nil")
	}
	return settings, nil
}

func SetBanner(bannerText string) (*mgo.ChangeInfo, error) {
	return db.Upsert(
		Collection,
		SettingsQuery,
		bson.M{
			"$set": bson.M{IdKey: SystemSettingsID, BannerKey: bannerText},
		},
	)
}

func SetTaskDispatchDisabled(disableTasks bool) (*mgo.ChangeInfo, error) {
	return db.Upsert(
		Collection,
		SettingsQuery,
		bson.M{
			"$set": bson.M{IdKey: SystemSettingsID, TaskDispatchKey: disableTasks},
		},
	)
}

func SetRunnerFlags(flags *RunnerFlags) (*mgo.ChangeInfo, error) {
	if flags == nil {
		return nil, errors.New("Runner flags are nil")
	}
	return db.Upsert(
		Collection,
		SettingsQuery,
		bson.M{
			"$set": bson.M{IdKey: SystemSettingsID, RunnerFlagsKey: flags},
		},
	)
}

func Upsert(settings *AdminSettings) (*mgo.ChangeInfo, error) {
	update := bson.M{
		"$set": bson.M{
			IdKey:           SystemSettingsID,
			BannerKey:       settings.GetBanner(),
			TaskDispatchKey: settings.GetTaskDispatchDisabled(),
			RunnerFlagsKey:  settings.GetRunnerFlags(),
		},
	}
	return db.Upsert(Collection, SettingsQuery, update)
}
