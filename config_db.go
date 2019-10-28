package evergreen

import (
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	hostJasperKey         = bsonutil.MustHaveTag(Settings{}, "HostJasper")
	domainNameKey         = bsonutil.MustHaveTag(Settings{}, "DomainName")
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
	bugsnagKey            = bsonutil.MustHaveTag(Settings{}, "Bugsnag")
	githubPRCreatorOrgKey = bsonutil.MustHaveTag(Settings{}, "GithubPRCreatorOrg")
	githubOrgsKey         = bsonutil.MustHaveTag(Settings{}, "GithubOrgs")
	containerPoolsKey     = bsonutil.MustHaveTag(Settings{}, "ContainerPools")
	commitQueueKey        = bsonutil.MustHaveTag(Settings{}, "CommitQueue")

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
	cacheStatsOldTasksDisabledKey   = bsonutil.MustHaveTag(ServiceFlags{}, "CacheStatsOldTasksDisabled")
	taskReliabilityDisabledKey      = bsonutil.MustHaveTag(ServiceFlags{}, "TaskReliabilityDisabled")
	commitQueueDisabledKey          = bsonutil.MustHaveTag(ServiceFlags{}, "CommitQueueDisabled")
	plannerDisabledKey              = bsonutil.MustHaveTag(ServiceFlags{}, "PlannerDisabled")
	hostAllocatorDisabledKey        = bsonutil.MustHaveTag(ServiceFlags{}, "HostAllocatorDisabled")

	// ContainerPoolsConfig keys
	poolsKey = bsonutil.MustHaveTag(ContainerPoolsConfig{}, "Pools")

	// ContainerPool keys
	ContainerPoolIdKey = bsonutil.MustHaveTag(ContainerPool{}, "Id")

	// HostJasperConfig keys
	hostJasperBinaryNameKey       = bsonutil.MustHaveTag(HostJasperConfig{}, "BinaryName")
	hostJasperDownloadFileNameKey = bsonutil.MustHaveTag(HostJasperConfig{}, "DownloadFileName")
	hostJasperPortKey             = bsonutil.MustHaveTag(HostJasperConfig{}, "Port")
	hostJasperURLKey              = bsonutil.MustHaveTag(HostJasperConfig{}, "URL")
	hostJasperVersionKey          = bsonutil.MustHaveTag(HostJasperConfig{}, "Version")
)

func byId(id string) bson.M {
	return bson.M{idKey: id}
}

// SetBanner sets the text of the Evergreen site-wide banner. Setting a blank
// string here means that there is no banner
func SetBanner(bannerText string) error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(ConfigDocID), bson.M{
		"$set": bson.M{bannerKey: bannerText},
	}, options.Update().SetUpsert(true))

	return errors.WithStack(err)
}

// SetBanner sets the text of the Evergreen site-wide banner. Setting a blank
// string here means that there is no banner
func SetBannerTheme(theme BannerTheme) error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(ConfigDocID), bson.M{
		"$set": bson.M{bannerThemeKey: theme},
	}, options.Update().SetUpsert(true))

	return errors.WithStack(err)
}

// SetServiceFlags sets whether each of the runner/API server processes is enabled
func SetServiceFlags(flags ServiceFlags) error {
	return flags.Set()
}
