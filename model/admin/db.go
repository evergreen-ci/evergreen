package admin

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	Collection       = "admin"
	systemSettingsID = "0"
)

var (
	IdKey     = bsonutil.MustHaveTag(AdminSettings{}, "Id")
	BannerKey = bsonutil.MustHaveTag(AdminSettings{}, "Banner")

	ServiceFlagsKey  = bsonutil.MustHaveTag(AdminSettings{}, "ServiceFlags")
	TaskDispatchKey  = bsonutil.MustHaveTag(ServiceFlags{}, "TaskDispatchDisabled")
	HostinitKey      = bsonutil.MustHaveTag(ServiceFlags{}, "HostinitDisabled")
	MonitorKey       = bsonutil.MustHaveTag(ServiceFlags{}, "MonitorDisabled")
	NotificationsKey = bsonutil.MustHaveTag(ServiceFlags{}, "NotificationsDisabled")
	AlertsKey        = bsonutil.MustHaveTag(ServiceFlags{}, "AlertsDisabled")
	TaskrunnerKey    = bsonutil.MustHaveTag(ServiceFlags{}, "TaskrunnerDisabled")
	RepotrackerKey   = bsonutil.MustHaveTag(ServiceFlags{}, "RepotrackerDisabled")
	SchedulerKey     = bsonutil.MustHaveTag(ServiceFlags{}, "SchedulerDisabled")
)

var settingsQuery = db.Query(bson.M{IdKey: systemSettingsID})

func GetSettingsFromDB() (*AdminSettings, error) {
	settings := &AdminSettings{}
	query := db.Q{}
	query = query.Filter(settingsQuery)
	err := db.FindOneQ(Collection, query, settings)
	if err != nil {
		return nil, errors.Wrap(err, "Error retrieving admin settings from DB")
	}
	if settings == nil {
		return nil, errors.New("Settings are nil")
	}
	return settings, nil
}

func SetBanner(bannerText string) error {
	_, err := db.Upsert(
		Collection,
		settingsQuery,
		bson.M{
			"$set": bson.M{IdKey: systemSettingsID, BannerKey: bannerText},
		},
	)

	return err
}

func SetTaskDispatchDisabled(disableTasks bool) error {
	_, err := db.Upsert(
		Collection,
		settingsQuery,
		bson.M{
			"$set": bson.M{IdKey: systemSettingsID, TaskDispatchKey: disableTasks},
		},
	)

	return err
}

func SetRunnerFlags(flags ServiceFlags) error {
	_, err := db.Upsert(
		Collection,
		settingsQuery,
		bson.M{
			"$set": bson.M{IdKey: systemSettingsID, ServiceFlagsKey: flags},
		},
	)

	return err
}

func Upsert(settings *AdminSettings) error {
	update := bson.M{
		"$set": bson.M{
			IdKey:           systemSettingsID,
			BannerKey:       settings.Banner,
			ServiceFlagsKey: settings.ServiceFlags,
		},
	}
	_, err := db.Upsert(Collection, settingsQuery, update)

	return err
}
