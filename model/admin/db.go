package admin

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"gopkg.in/mgo.v2/bson"
)

const (
	Collection  = "admin"
	configDocID = "global"
)

var (
	idKey = bsonutil.MustHaveTag(Config{}, "Id")

	bannerKey       = bsonutil.MustHaveTag(Config{}, "Banner")
	bannerThemeKey  = bsonutil.MustHaveTag(Config{}, "BannerTheme")
	serviceFlagsKey = bsonutil.MustHaveTag(Config{}, "ServiceFlags")

	configDirKey          = bsonutil.MustHaveTag(Config{}, "ConfigDir")
	apiUrlKey             = bsonutil.MustHaveTag(Config{}, "ApiUrl")
	clientBinariesDirKey  = bsonutil.MustHaveTag(Config{}, "ClientBinariesDir")
	superUsersKey         = bsonutil.MustHaveTag(Config{}, "SuperUsers")
	jiraKey               = bsonutil.MustHaveTag(Config{}, "Jira")
	splunkKey             = bsonutil.MustHaveTag(Config{}, "Splunk")
	slackKey              = bsonutil.MustHaveTag(Config{}, "Slack")
	providersKey          = bsonutil.MustHaveTag(Config{}, "Providers")
	keysKey               = bsonutil.MustHaveTag(Config{}, "Keys")
	credentialsKey        = bsonutil.MustHaveTag(Config{}, "Credentials")
	authConfigKey         = bsonutil.MustHaveTag(Config{}, "AuthConfig")
	repoTrackerConfigKey  = bsonutil.MustHaveTag(Config{}, "RepoTracker")
	apiKey                = bsonutil.MustHaveTag(Config{}, "Api")
	alertsConfigKey       = bsonutil.MustHaveTag(Config{}, "Alerts")
	uiKey                 = bsonutil.MustHaveTag(Config{}, "Ui")
	hostInitConfigKey     = bsonutil.MustHaveTag(Config{}, "HostInit")
	notifyKey             = bsonutil.MustHaveTag(Config{}, "Notify")
	schedulerConfigKey    = bsonutil.MustHaveTag(Config{}, "Scheduler")
	amboyKey              = bsonutil.MustHaveTag(Config{}, "Amboy")
	expansionsKey         = bsonutil.MustHaveTag(Config{}, "Expansions")
	pluginsKey            = bsonutil.MustHaveTag(Config{}, "Plugins")
	isNonProdKey          = bsonutil.MustHaveTag(Config{}, "IsNonProd")
	loggerConfigKey       = bsonutil.MustHaveTag(Config{}, "LoggerConfig")
	logPathKey            = bsonutil.MustHaveTag(Config{}, "LogPath")
	pprofPortKey          = bsonutil.MustHaveTag(Config{}, "PprofPort")
	githubPRCreatorOrgKey = bsonutil.MustHaveTag(Config{}, "GithubPRCreatorOrg")
	newRelicKey           = bsonutil.MustHaveTag(Config{}, "NewRelic")

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

var settingsQuery = db.Query(bson.M{idKey: configDocID})

func byId(id string) bson.M {
	return bson.M{idKey: id}
}

// SetBanner sets the text of the Evergreen site-wide banner. Setting a blank
// string here means that there is no banner
func SetBanner(bannerText string) error {
	_, err := db.Upsert(
		Collection,
		settingsQuery,
		bson.M{
			"$set": bson.M{idKey: configDocID, bannerKey: bannerText},
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
			"$set": bson.M{idKey: configDocID, bannerThemeKey: theme},
		},
	)

	return err
}

// SetServiceFlags sets whether each of the runner/API server processes is enabled
func SetServiceFlags(flags ServiceFlags) error {
	return flags.set()
}

// Upsert will update/insert the admin settings document
func Upsert(settings *Config) error {
	update := bson.M{
		"$set": bson.M{
			idKey:                 configDocID,
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
