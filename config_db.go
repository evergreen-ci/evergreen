package evergreen

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"gopkg.in/mgo.v2/bson"
)

var (
	ConfigCollection = "admin"
	ConfigDocID      = "global"
)

//nolint: megacheck, deadcode, unused
var (
	idKey                 = bsonutil.MustHaveTag(Settings{}, "Id")
	bannerKey             = bsonutil.MustHaveTag(Settings{}, "Banner")
	bannerThemeKey        = bsonutil.MustHaveTag(Settings{}, "BannerTheme")
	serviceFlagsKey       = bsonutil.MustHaveTag(Settings{}, "ServiceFlags")
	configDirKey          = bsonutil.MustHaveTag(Settings{}, "ConfigDir")
	apiUrlKey             = bsonutil.MustHaveTag(Settings{}, "ApiUrl")
	clientBinariesDirKey  = bsonutil.MustHaveTag(Settings{}, "ClientBinariesDir")
	superUsersKey         = bsonutil.MustHaveTag(Settings{}, "SuperUsers")
	jiraKey               = bsonutil.MustHaveTag(Settings{}, "Jira")
	splunkKey             = bsonutil.MustHaveTag(Settings{}, "Splunk")
	slackKey              = bsonutil.MustHaveTag(Settings{}, "Slack")
	providersKey          = bsonutil.MustHaveTag(Settings{}, "Providers")
	keysKey               = bsonutil.MustHaveTag(Settings{}, "Keys")
	keysNewKey            = bsonutil.MustHaveTag(Settings{}, "KeysNew")
	credentialsKey        = bsonutil.MustHaveTag(Settings{}, "Credentials")
	credentialsNewKey     = bsonutil.MustHaveTag(Settings{}, "CredentialsNew")
	authConfigKey         = bsonutil.MustHaveTag(Settings{}, "AuthConfig")
	repoTrackerConfigKey  = bsonutil.MustHaveTag(Settings{}, "RepoTracker")
	apiKey                = bsonutil.MustHaveTag(Settings{}, "Api")
	alertsConfigKey       = bsonutil.MustHaveTag(Settings{}, "Alerts")
	uiKey                 = bsonutil.MustHaveTag(Settings{}, "Ui")
	hostInitConfigKey     = bsonutil.MustHaveTag(Settings{}, "HostInit")
	notifyKey             = bsonutil.MustHaveTag(Settings{}, "Notify")
	schedulerConfigKey    = bsonutil.MustHaveTag(Settings{}, "Scheduler")
	amboyKey              = bsonutil.MustHaveTag(Settings{}, "Amboy")
	expansionsKey         = bsonutil.MustHaveTag(Settings{}, "Expansions")
	expansionsNewKey      = bsonutil.MustHaveTag(Settings{}, "ExpansionsNew")
	pluginsKey            = bsonutil.MustHaveTag(Settings{}, "Plugins")
	pluginsNewKey         = bsonutil.MustHaveTag(Settings{}, "PluginsNew")
	loggerConfigKey       = bsonutil.MustHaveTag(Settings{}, "LoggerConfig")
	logPathKey            = bsonutil.MustHaveTag(Settings{}, "LogPath")
	pprofPortKey          = bsonutil.MustHaveTag(Settings{}, "PprofPort")
	githubPRCreatorOrgKey = bsonutil.MustHaveTag(Settings{}, "GithubPRCreatorOrg")
	containerPoolsKey     = bsonutil.MustHaveTag(Settings{}, "ContainerPools")

	// degraded mode flags
	taskDispatchKey                 = bsonutil.MustHaveTag(ServiceFlags{}, "TaskDispatchDisabled")
	hostInitKey                     = bsonutil.MustHaveTag(ServiceFlags{}, "HostInitDisabled")
	monitorKey                      = bsonutil.MustHaveTag(ServiceFlags{}, "MonitorDisabled")
	alertsKey                       = bsonutil.MustHaveTag(ServiceFlags{}, "AlertsDisabled")
	agentStartKey                   = bsonutil.MustHaveTag(ServiceFlags{}, "AgentStartDisabled")
	repotrackerKey                  = bsonutil.MustHaveTag(ServiceFlags{}, "RepotrackerDisabled")
	schedulerKey                    = bsonutil.MustHaveTag(ServiceFlags{}, "SchedulerDisabled")
	githubPRTestingDisabledKey      = bsonutil.MustHaveTag(ServiceFlags{}, "GithubPRTestingDisabled")
	cliUpdatesDisabledKey           = bsonutil.MustHaveTag(ServiceFlags{}, "CLIUpdatesDisabled")
	backgroundStatsDisabledKey      = bsonutil.MustHaveTag(ServiceFlags{}, "BackgroundStatsDisabled")
	eventProcessingDisabledKey      = bsonutil.MustHaveTag(ServiceFlags{}, "EventProcessingDisabled")
	jiraNotificationsDisabledKey    = bsonutil.MustHaveTag(ServiceFlags{}, "JIRANotificationsDisabled")
	slackNotificationsDisabledKey   = bsonutil.MustHaveTag(ServiceFlags{}, "SlackNotificationsDisabled")
	emailNotificationsDisabledKey   = bsonutil.MustHaveTag(ServiceFlags{}, "EmailNotificationsDisabled")
	webhookNotificationsDisabledKey = bsonutil.MustHaveTag(ServiceFlags{}, "WebhookNotificationsDisabled")
	githubStatusAPIDisabledKey      = bsonutil.MustHaveTag(ServiceFlags{}, "GithubStatusAPIDisabled")
	taskLoggingDisabledKey          = bsonutil.MustHaveTag(ServiceFlags{}, "TaskLoggingDisabled")
	cacheStatsJobDisabledKey        = bsonutil.MustHaveTag(ServiceFlags{}, "CacheStatsJobDisabled")
	cacheStatsEndpointDisabledKey   = bsonutil.MustHaveTag(ServiceFlags{}, "CacheStatsEndpointDisabled")
	commitQueueDisabledKey          = bsonutil.MustHaveTag(ServiceFlags{}, "CommitQueueDisabled")

	// ContainerPoolsConfig keys
	poolsKey = bsonutil.MustHaveTag(ContainerPoolsConfig{}, "Pools")

	// ContainerPool keys
	ContainerPoolIdKey = bsonutil.MustHaveTag(ContainerPool{}, "Id")
)

func byId(id string) bson.M {
	return bson.M{idKey: id}
}

// SetBanner sets the text of the Evergreen site-wide banner. Setting a blank
// string here means that there is no banner
func SetBanner(bannerText string) error {
	_, err := db.Upsert(
		ConfigCollection,
		byId(ConfigDocID),
		bson.M{
			"$set": bson.M{bannerKey: bannerText},
		},
	)

	return err
}

// SetBanner sets the text of the Evergreen site-wide banner. Setting a blank
// string here means that there is no banner
func SetBannerTheme(theme BannerTheme) error {
	_, err := db.Upsert(
		ConfigCollection,
		byId(ConfigDocID),
		bson.M{
			"$set": bson.M{bannerThemeKey: theme},
		},
	)

	return err
}

// SetServiceFlags sets whether each of the runner/API server processes is enabled
func SetServiceFlags(flags ServiceFlags) error {
	return flags.Set()
}
