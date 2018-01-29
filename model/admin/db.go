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
	idKey = bsonutil.MustHaveTag(AdminSettings{}, "Id")

	bannerKey       = bsonutil.MustHaveTag(AdminSettings{}, "Banner")
	bannerThemeKey  = bsonutil.MustHaveTag(AdminSettings{}, "BannerTheme")
	serviceFlagsKey = bsonutil.MustHaveTag(AdminSettings{}, "ServiceFlags")

	configDirKey          = bsonutil.MustHaveTag(AdminSettings{}, "ConfigDir")
	apiUrlKey             = bsonutil.MustHaveTag(AdminSettings{}, "ApiUrl")
	clientBinariesDirKey  = bsonutil.MustHaveTag(AdminSettings{}, "ClientBinariesDir")
	superUsersKey         = bsonutil.MustHaveTag(AdminSettings{}, "SuperUsers")
	jiraKey               = bsonutil.MustHaveTag(AdminSettings{}, "Jira")
	splunkKey             = bsonutil.MustHaveTag(AdminSettings{}, "Splunk")
	slackKey              = bsonutil.MustHaveTag(AdminSettings{}, "Slack")
	providersKey          = bsonutil.MustHaveTag(AdminSettings{}, "Providers")
	keysKey               = bsonutil.MustHaveTag(AdminSettings{}, "Keys")
	credentialsKey        = bsonutil.MustHaveTag(AdminSettings{}, "Credentials")
	authConfigKey         = bsonutil.MustHaveTag(AdminSettings{}, "AuthConfig")
	repoTrackerConfigKey  = bsonutil.MustHaveTag(AdminSettings{}, "RepoTracker")
	apiKey                = bsonutil.MustHaveTag(AdminSettings{}, "Api")
	alertsConfigKey       = bsonutil.MustHaveTag(AdminSettings{}, "Alerts")
	uiKey                 = bsonutil.MustHaveTag(AdminSettings{}, "Ui")
	hostInitConfigKey     = bsonutil.MustHaveTag(AdminSettings{}, "HostInit")
	notifyKey             = bsonutil.MustHaveTag(AdminSettings{}, "Notify")
	schedulerConfigKey    = bsonutil.MustHaveTag(AdminSettings{}, "Scheduler")
	amboyKey              = bsonutil.MustHaveTag(AdminSettings{}, "Amboy")
	expansionsKey         = bsonutil.MustHaveTag(AdminSettings{}, "Expansions")
	pluginsKey            = bsonutil.MustHaveTag(AdminSettings{}, "Plugins")
	isNonProdKey          = bsonutil.MustHaveTag(AdminSettings{}, "IsNonProd")
	loggerConfigKey       = bsonutil.MustHaveTag(AdminSettings{}, "LoggerConfig")
	logPathKey            = bsonutil.MustHaveTag(AdminSettings{}, "LogPath")
	pprofPortKey          = bsonutil.MustHaveTag(AdminSettings{}, "PprofPort")
	githubPRCreatorOrgKey = bsonutil.MustHaveTag(AdminSettings{}, "GithubPRCreatorOrg")
	newRelicKey           = bsonutil.MustHaveTag(AdminSettings{}, "NewRelic")

	// degraded mode flags
	taskDispatchKey                 = bsonutil.MustHaveTag(ServiceFlags{}, "TaskDispatchDisabled")
	hostinitKey                     = bsonutil.MustHaveTag(ServiceFlags{}, "HostinitDisabled")
	monitorKey                      = bsonutil.MustHaveTag(ServiceFlags{}, "MonitorDisabled")
	notificationsKey                = bsonutil.MustHaveTag(ServiceFlags{}, "NotificationsDisabled")
	alertsKey                       = bsonutil.MustHaveTag(ServiceFlags{}, "AlertsDisabled")
	taskrunnerKey                   = bsonutil.MustHaveTag(ServiceFlags{}, "TaskrunnerDisabled")
	repotrackerKey                  = bsonutil.MustHaveTag(ServiceFlags{}, "RepotrackerDisabled")
	schedulerKey                    = bsonutil.MustHaveTag(ServiceFlags{}, "SchedulerDisabled")
	githubPRTestingDisabledKey      = bsonutil.MustHaveTag(ServiceFlags{}, "GithubPRTestingDisabled")
	repotrackerPushEventDisabledKey = bsonutil.MustHaveTag(ServiceFlags{}, "RepotrackerPushEventDisabled")
	cliUpdatesDisabledKey           = bsonutil.MustHaveTag(ServiceFlags{}, "CLIUpdatesDisabled")
	githubStatusAPIDisabled         = bsonutil.MustHaveTag(ServiceFlags{}, "GithubStatusAPIDisabled")
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
			idKey:                 systemSettingsDocID,
			bannerKey:             settings.Banner,
			serviceFlagsKey:       settings.ServiceFlags,
			configDirKey:          settings.ConfigDir,
			apiUrlKey:             settings.ApiUrl,
			clientBinariesDirKey:  settings.ClientBinariesDir,
			superUsersKey:         settings.SuperUsers,
			jiraKey:               settings.Jira,
			splunkKey:             settings.Splunk,
			slackKey:              settings.Slack,
			providersKey:          settings.Providers,
			keysKey:               settings.Keys,
			credentialsKey:        settings.Credentials,
			authConfigKey:         settings.AuthConfig,
			repoTrackerConfigKey:  settings.RepoTracker,
			apiKey:                settings.Api,
			alertsConfigKey:       settings.Alerts,
			uiKey:                 settings.Ui,
			hostInitConfigKey:     settings.HostInit,
			notifyKey:             settings.Notify,
			schedulerConfigKey:    settings.Scheduler,
			amboyKey:              settings.Amboy,
			expansionsKey:         settings.Expansions,
			pluginsKey:            settings.Plugins,
			isNonProdKey:          settings.IsNonProd,
			loggerConfigKey:       settings.LoggerConfig,
			logPathKey:            settings.LogPath,
			pprofPortKey:          settings.PprofPort,
			githubPRCreatorOrgKey: settings.GithubPRCreatorOrg,
			newRelicKey:           settings.NewRelic,
		},
	}
	_, err := db.Upsert(Collection, settingsQuery, update)

	return err
}
