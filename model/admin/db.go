package admin

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	Collection          = "admin"
	systemSettingsDocID = "global"
)

var (
	idKey                      = bsonutil.MustHaveTag(AdminSettings{}, "Id")
	bannerKey                  = bsonutil.MustHaveTag(AdminSettings{}, "Banner")
	bannerThemeKey             = bsonutil.MustHaveTag(AdminSettings{}, "BannerTheme")
	serviceFlagsKey            = bsonutil.MustHaveTag(AdminSettings{}, "ServiceFlags")
	taskDispatchKey            = bsonutil.MustHaveTag(ServiceFlags{}, "TaskDispatchDisabled")
	hostinitKey                = bsonutil.MustHaveTag(ServiceFlags{}, "HostinitDisabled")
	monitorKey                 = bsonutil.MustHaveTag(ServiceFlags{}, "MonitorDisabled")
	notificationsKey           = bsonutil.MustHaveTag(ServiceFlags{}, "NotificationsDisabled")
	alertsKey                  = bsonutil.MustHaveTag(ServiceFlags{}, "AlertsDisabled")
	taskrunnerKey              = bsonutil.MustHaveTag(ServiceFlags{}, "TaskrunnerDisabled")
	repotrackerKey             = bsonutil.MustHaveTag(ServiceFlags{}, "RepotrackerDisabled")
	schedulerKey               = bsonutil.MustHaveTag(ServiceFlags{}, "SchedulerDisabled")
	githubPRTestingDisabledKey = bsonutil.MustHaveTag(ServiceFlags{}, "GithubPRTestingDisabled")
	githubPushEventDisabledKey = bsonutil.MustHaveTag(ServiceFlags{}, "GithubPushEventDisabled")
)

var settingsQuery = db.Query(bson.M{idKey: systemSettingsDocID})

// GetSettings retrieves the admin settings document. If no document is
// present in the DB, it will return the defaults
func GetSettings() (*AdminSettings, error) {
	settings := &AdminSettings{}
	query := db.Q{}
	query = query.Filter(settingsQuery)
	err := db.FindOneQ(Collection, query, settings)
	if err != nil {
		// if the settings document doesn't exist, return the defaults
		if err.Error() == "not found" {
			return settings, nil
		} else {
			return nil, errors.Wrap(err, "Error retrieving admin settings from DB")
		}
	}
	return settings, nil
}

// SetBanner sets the text of the Evergreen site-wide banner. Setting a blank
// string here means that there is no banner
func SetBanner(bannerText string) error {
	_, err := db.Upsert(
		Collection,
		settingsQuery,
		bson.M{
			"$set": bson.M{idKey: systemSettingsDocID, bannerKey: bannerText},
		},
	)

	return err
}

// SetBanner sets the text of the Evergreen site-wide banner. Setting a blank
// string here means that there is no banner
func SetBannerTheme(theme BannerTheme) error {
	_, err := db.Upsert(
		Collection,
		settingsQuery,
		bson.M{
			"$set": bson.M{idKey: systemSettingsDocID, bannerThemeKey: theme},
		},
	)

	return err
}

// SetServiceFlags sets whether each of the runner/API server processes is enabled
func SetServiceFlags(flags ServiceFlags) error {
	_, err := db.Upsert(
		Collection,
		settingsQuery,
		bson.M{
			"$set": bson.M{idKey: systemSettingsDocID, serviceFlagsKey: flags},
		},
	)

	return err
}

// Upsert will update/insert the admin settings document
func Upsert(settings *AdminSettings) error {
	update := bson.M{
		"$set": bson.M{
			idKey:           systemSettingsDocID,
			bannerKey:       settings.Banner,
			serviceFlagsKey: settings.ServiceFlags,
		},
	}
	_, err := db.Upsert(Collection, settingsQuery, update)

	return err
}
