package evergreen

import (
	"context"
	"reflect"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ConfigCollection = "admin"
	ConfigDocID      = "global"
)

//nolint:unused
var (
	idKey                      = bsonutil.MustHaveTag(Settings{}, "Id")
	bannerKey                  = bsonutil.MustHaveTag(Settings{}, "Banner")
	bannerThemeKey             = bsonutil.MustHaveTag(Settings{}, "BannerTheme")
	serviceFlagsKey            = bsonutil.MustHaveTag(Settings{}, "ServiceFlags")
	configDirKey               = bsonutil.MustHaveTag(Settings{}, "ConfigDir")
	awsInstanceRoleKey         = bsonutil.MustHaveTag(Settings{}, "AWSInstanceRole")
	hostJasperKey              = bsonutil.MustHaveTag(Settings{}, "HostJasper")
	domainNameKey              = bsonutil.MustHaveTag(Settings{}, "DomainName")
	jiraKey                    = bsonutil.MustHaveTag(Settings{}, "Jira")
	splunkKey                  = bsonutil.MustHaveTag(Settings{}, "Splunk")
	slackKey                   = bsonutil.MustHaveTag(Settings{}, "Slack")
	providersKey               = bsonutil.MustHaveTag(Settings{}, "Providers")
	kanopySSHKeyPathKey        = bsonutil.MustHaveTag(Settings{}, "KanopySSHKeyPath")
	authConfigKey              = bsonutil.MustHaveTag(Settings{}, "AuthConfig")
	repoTrackerConfigKey       = bsonutil.MustHaveTag(Settings{}, "RepoTracker")
	apiKey                     = bsonutil.MustHaveTag(Settings{}, "Api")
	uiKey                      = bsonutil.MustHaveTag(Settings{}, "Ui")
	hostInitConfigKey          = bsonutil.MustHaveTag(Settings{}, "HostInit")
	notifyKey                  = bsonutil.MustHaveTag(Settings{}, "Notify")
	schedulerConfigKey         = bsonutil.MustHaveTag(Settings{}, "Scheduler")
	amboyKey                   = bsonutil.MustHaveTag(Settings{}, "Amboy")
	expansionsKey              = bsonutil.MustHaveTag(Settings{}, "Expansions")
	expansionsNewKey           = bsonutil.MustHaveTag(Settings{}, "ExpansionsNew")
	pluginsKey                 = bsonutil.MustHaveTag(Settings{}, "Plugins")
	pluginsNewKey              = bsonutil.MustHaveTag(Settings{}, "PluginsNew")
	loggerConfigKey            = bsonutil.MustHaveTag(Settings{}, "LoggerConfig")
	logPathKey                 = bsonutil.MustHaveTag(Settings{}, "LogPath")
	oldestAllowedCLIVersionKey = bsonutil.MustHaveTag(Settings{}, "OldestAllowedCLIVersion")
	pprofPortKey               = bsonutil.MustHaveTag(Settings{}, "PprofPort")
	perfMonitoringURLKey       = bsonutil.MustHaveTag(Settings{}, "PerfMonitoringURL")
	perfMonitoringKanopyURLKey = bsonutil.MustHaveTag(Settings{}, "PerfMonitoringKanopyURL")
	githubPRCreatorOrgKey      = bsonutil.MustHaveTag(Settings{}, "GithubPRCreatorOrg")
	githubOrgsKey              = bsonutil.MustHaveTag(Settings{}, "GithubOrgs")
	githubWebhookSecretKey     = bsonutil.MustHaveTag(Settings{}, "GithubWebhookSecret")
	disabledGQLQueriesKey      = bsonutil.MustHaveTag(Settings{}, "DisabledGQLQueries")
	spawnhostKey               = bsonutil.MustHaveTag(Settings{}, "Spawnhost")
	shutdownWaitKey            = bsonutil.MustHaveTag(Settings{}, "ShutdownWaitSeconds")
	sshKey                     = bsonutil.MustHaveTag(Settings{}, "SSH")

	// degraded mode flags
	taskDispatchKey                       = bsonutil.MustHaveTag(ServiceFlags{}, "TaskDispatchDisabled")
	hostInitKey                           = bsonutil.MustHaveTag(ServiceFlags{}, "HostInitDisabled")
	largeParserProjectsDisabledKey        = bsonutil.MustHaveTag(ServiceFlags{}, "LargeParserProjectsDisabled")
	monitorKey                            = bsonutil.MustHaveTag(ServiceFlags{}, "MonitorDisabled")
	alertsKey                             = bsonutil.MustHaveTag(ServiceFlags{}, "AlertsDisabled")
	agentStartKey                         = bsonutil.MustHaveTag(ServiceFlags{}, "AgentStartDisabled")
	repotrackerKey                        = bsonutil.MustHaveTag(ServiceFlags{}, "RepotrackerDisabled")
	schedulerKey                          = bsonutil.MustHaveTag(ServiceFlags{}, "SchedulerDisabled")
	checkBlockedTasksKey                  = bsonutil.MustHaveTag(ServiceFlags{}, "CheckBlockedTasksDisabled")
	githubPRTestingDisabledKey            = bsonutil.MustHaveTag(ServiceFlags{}, "GithubPRTestingDisabled")
	cliUpdatesDisabledKey                 = bsonutil.MustHaveTag(ServiceFlags{}, "CLIUpdatesDisabled")
	backgroundStatsDisabledKey            = bsonutil.MustHaveTag(ServiceFlags{}, "BackgroundStatsDisabled")
	eventProcessingDisabledKey            = bsonutil.MustHaveTag(ServiceFlags{}, "EventProcessingDisabled")
	jiraNotificationsDisabledKey          = bsonutil.MustHaveTag(ServiceFlags{}, "JIRANotificationsDisabled")
	slackNotificationsDisabledKey         = bsonutil.MustHaveTag(ServiceFlags{}, "SlackNotificationsDisabled")
	emailNotificationsDisabledKey         = bsonutil.MustHaveTag(ServiceFlags{}, "EmailNotificationsDisabled")
	webhookNotificationsDisabledKey       = bsonutil.MustHaveTag(ServiceFlags{}, "WebhookNotificationsDisabled")
	githubStatusAPIDisabledKey            = bsonutil.MustHaveTag(ServiceFlags{}, "GithubStatusAPIDisabled")
	taskLoggingDisabledKey                = bsonutil.MustHaveTag(ServiceFlags{}, "TaskLoggingDisabled")
	cacheStatsJobDisabledKey              = bsonutil.MustHaveTag(ServiceFlags{}, "CacheStatsJobDisabled")
	cacheStatsEndpointDisabledKey         = bsonutil.MustHaveTag(ServiceFlags{}, "CacheStatsEndpointDisabled")
	taskReliabilityDisabledKey            = bsonutil.MustHaveTag(ServiceFlags{}, "TaskReliabilityDisabled")
	hostAllocatorDisabledKey              = bsonutil.MustHaveTag(ServiceFlags{}, "HostAllocatorDisabled")
	backgroundReauthDisabledKey           = bsonutil.MustHaveTag(ServiceFlags{}, "BackgroundReauthDisabled")
	cloudCleanupDisabledKey               = bsonutil.MustHaveTag(ServiceFlags{}, "CloudCleanupDisabled")
	sleepScheduleDisabledKey              = bsonutil.MustHaveTag(ServiceFlags{}, "SleepScheduleDisabled")
	staticAPIKeysDisabledKey              = bsonutil.MustHaveTag(ServiceFlags{}, "StaticAPIKeysDisabled")
	JWTTokenForCLIDisabledKey             = bsonutil.MustHaveTag(ServiceFlags{}, "JWTTokenForCLIDisabled")
	systemFailedTaskRestartDisabledKey    = bsonutil.MustHaveTag(ServiceFlags{}, "SystemFailedTaskRestartDisabled")
	cpuDegradedModeDisabledKey            = bsonutil.MustHaveTag(ServiceFlags{}, "CPUDegradedModeDisabled")
	elasticIPsDisabledKey                 = bsonutil.MustHaveTag(ServiceFlags{}, "ElasticIPsDisabled")
	releaseModeDisabledKey                = bsonutil.MustHaveTag(ServiceFlags{}, "ReleaseModeDisabled")
	legacyUIAdminPageDisabledKey          = bsonutil.MustHaveTag(ServiceFlags{}, "LegacyUIAdminPageDisabled")
	debugSpawnHostDisabledKey             = bsonutil.MustHaveTag(ServiceFlags{}, "DebugSpawnHostDisabled")
	s3LifecycleSyncDisabledKey            = bsonutil.MustHaveTag(ServiceFlags{}, "S3LifecycleSyncDisabled")
	useMergeQueuePathFilteringDisabledKey = bsonutil.MustHaveTag(ServiceFlags{}, "UseMergeQueuePathFilteringDisabled")
	psLoggingDisabledKey                  = bsonutil.MustHaveTag(ServiceFlags{}, "PSLoggingDisabled")

	// ContainerPoolsConfig keys
	poolsKey = bsonutil.MustHaveTag(ContainerPoolsConfig{}, "Pools")

	// ContainerPool keys
	ContainerPoolIdKey = bsonutil.MustHaveTag(ContainerPool{}, "Id")

	hostInitHostThrottleKey         = bsonutil.MustHaveTag(HostInitConfig{}, "HostThrottle")
	hostInitProvisioningThrottleKey = bsonutil.MustHaveTag(HostInitConfig{}, "ProvisioningThrottle")
	hostInitCloudStatusBatchSizeKey = bsonutil.MustHaveTag(HostInitConfig{}, "CloudStatusBatchSize")
	hostInitMaxTotalDynamicHostsKey = bsonutil.MustHaveTag(HostInitConfig{}, "MaxTotalDynamicHosts")

	// Spawnhost keys
	unexpirableHostsPerUserKey   = bsonutil.MustHaveTag(SpawnHostConfig{}, "UnexpirableHostsPerUser")
	unexpirableVolumesPerUserKey = bsonutil.MustHaveTag(SpawnHostConfig{}, "UnexpirableVolumesPerUser")
	spawnhostsPerUserKey         = bsonutil.MustHaveTag(SpawnHostConfig{}, "SpawnHostsPerUser")

	tracerEnabledKey                   = bsonutil.MustHaveTag(TracerConfig{}, "Enabled")
	tracerCollectorEndpointKey         = bsonutil.MustHaveTag(TracerConfig{}, "CollectorEndpoint")
	tracerCollectorInternalEndpointKey = bsonutil.MustHaveTag(TracerConfig{}, "CollectorInternalEndpoint")
	tracerCollectorAPIKeyKey           = bsonutil.MustHaveTag(TracerConfig{}, "CollectorAPIKey")

	// GithubCheckRun keys
	checkRunLimitKey = bsonutil.MustHaveTag(GitHubCheckRunConfig{}, "CheckRunLimit")

	// SingleTaskDistro Keys
	ProjectTasksPairsKey = bsonutil.MustHaveTag(SingleTaskDistroConfig{}, "ProjectTasksPairs")

	// GraphiteConfig keys
	graphiteCIOptimizationTokenKey = bsonutil.MustHaveTag(GraphiteConfig{}, "CIOptimizationToken")
	graphiteServerURLKey           = bsonutil.MustHaveTag(GraphiteConfig{}, "ServerURL")

	// DebugSpawnHostsConfig keys
	setupScriptKey = bsonutil.MustHaveTag(DebugSpawnHostsConfig{}, "SetupScript")
)

func byId(id string) bson.M {
	return bson.M{idKey: id}
}

func byIDs(ids []string) bson.M {
	return bson.M{idKey: bson.M{"$in": ids}}
}

// getSectionsBSON returns the config documents from the database as a slice of [bson.Raw].
// If no shared database exists all configuration is fetched from the [DB] database. If a
// [SharedDB] database exists, configuration is fetched from the [SharedDB] database, save for the
// overrides config document which is fetched from the [DB] database. If [SharedDB] database exists and
// [includeOverrides], the overrides in the [OverridesConfig] section are applied to the rest of the
// configuration.
func getSectionsBSON(ctx context.Context, ids []string, includeOverrides bool) ([]bson.Raw, error) {
	db := GetEnvironment().DB()
	if sharedDB := GetEnvironment().SharedDB(); sharedDB != nil {
		db = sharedDB
	}

	cur, err := db.Collection(ConfigCollection).Find(ctx, byIDs(ids))
	if err != nil {
		return nil, errors.Wrap(err, "finding local configuration sections")
	}
	docs := make([]bson.Raw, 0, len(ids))
	if err := cur.All(ctx, &docs); err != nil {
		return nil, errors.Wrap(err, "iterating cursor for local configuration sections")
	}

	if GetEnvironment().SharedDB() != nil {
		docs, err = overrideConfig(ctx, docs, includeOverrides)
		if err != nil {
			return nil, errors.Wrap(err, "overriding config")
		}
	}

	return docs, nil
}

// overrideConfig replaces the content of the [OverridesConfig] section with the contents
// of the overrides document from the [DB] database. If [applyOverrides] is true overrides
// from the [OverridesConfig] are applied to all the supplied sections.
func overrideConfig(ctx context.Context, sections []bson.Raw, applyOverrides bool) ([]bson.Raw, error) {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(overridesSectionID))
	if err := res.Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return sections, nil
		}
		return nil, errors.Wrap(err, "getting overrides document")
	}
	rawOverrides, err := res.Raw()
	if err != nil {
		return nil, errors.Wrap(err, "getting raw overrides config")
	}

	sections, err = replaceOverridesConfigSection(sections, rawOverrides)
	if err != nil {
		return nil, errors.Wrap(err, "overrideing overrides config")
	}

	if applyOverrides {
		var overrides OverridesConfig
		if err := bson.Unmarshal(rawOverrides, &overrides); err != nil {
			return nil, errors.Wrap(err, "unmarshalling overrides config")
		}
		sections, err = overrides.overrideRawConfiguration(sections)
		if err != nil {
			return nil, errors.Wrap(err, "applying configuration overrides")
		}
	}

	return sections, nil
}

// replaceOverridesConfigSection replaces the [OverridesConfig] document in [sections] with
// the provided [overrides].
func replaceOverridesConfigSection(sections []bson.Raw, overrides bson.Raw) ([]bson.Raw, error) {
	res := make([]bson.Raw, 0, len(sections))
	for _, section := range sections {
		idVal, err := section.LookupErr("_id")
		if err != nil {
			return nil, errors.Wrap(err, "getting document id")
		}
		id, ok := idVal.StringValueOK()
		if !ok {
			return nil, errors.New("config document id isn't a string")
		}
		if id == overridesSectionID {
			res = append(res, overrides)
		} else {
			res = append(res, section)
		}
	}

	return res, nil
}

// getConfigSection fetches a section from the database and deserializes it into the provided
// section. If there's a [SharedDB] database the configuration will come from there and
// overrides from the [OverridesConfig] section in the [DB] database will be applied.
// If a document is missing the value of its section is reset to its zero value.
func getConfigSection(ctx context.Context, section ConfigSection) error {
	db := GetEnvironment().DB()
	if sharedDB := GetEnvironment().SharedDB(); sharedDB != nil {
		db = sharedDB
	}
	res := db.Collection(ConfigCollection).FindOne(ctx, byId(section.SectionId()))
	if err := res.Err(); err != nil {
		if err != mongo.ErrNoDocuments {
			return errors.Wrapf(err, "getting config section '%s'", section.SectionId())
		}
		// Reset the section to its zero value.
		reflect.ValueOf(section).Elem().Set(reflect.New(reflect.ValueOf(section).Elem().Type()).Elem())
		return nil
	}

	raw, err := res.Raw()
	if err != nil {
		return errors.Wrap(err, "getting raw result")
	}
	if GetEnvironment().SharedDB() != nil {
		overridden, err := overrideConfig(ctx, []bson.Raw{raw}, true)
		if err != nil {
			return errors.Wrap(err, "overriding configuration section")
		}
		if len(overridden) != 1 {
			return errors.Errorf("overridden config had unexpected length %d", len(overridden))
		}
		raw = overridden[0]
	}

	if err := bson.Unmarshal(raw, section); err != nil {
		return errors.Wrapf(err, "unmarshalling config section '%s'", section.SectionId())
	}

	return nil
}

// setConfigSection applies [update] to the specified configuration section. If there's a shared
// database the update is applied there.
func setConfigSection(ctx context.Context, sectionID string, update bson.M) error {
	db := GetEnvironment().SharedDB()
	if db == nil || sectionID == overridesSectionID {
		db = GetEnvironment().DB()
	}
	_, err := db.Collection(ConfigCollection).UpdateOne(
		ctx,
		byId(sectionID),
		update,
		options.Update().SetUpsert(true),
	)

	return errors.Wrapf(err, "updating config section '%s'", sectionID)
}

// SetBanner sets the text of the Evergreen site-wide banner. Setting a blank
// string here means that there is no banner
func SetBanner(ctx context.Context, bannerText string) error {
	coll := GetEnvironment().DB().Collection(ConfigCollection)
	_, err := coll.UpdateOne(ctx, byId(ConfigDocID), bson.M{
		"$set": bson.M{bannerKey: bannerText},
	}, options.Update().SetUpsert(true))

	return errors.WithStack(err)
}

// SetBannerTheme sets the text of the Evergreen site-wide banner. Setting a blank
// string here means that there is no banner
func SetBannerTheme(ctx context.Context, theme BannerTheme) error {
	coll := GetEnvironment().DB().Collection(ConfigCollection)
	_, err := coll.UpdateOne(ctx, byId(ConfigDocID), bson.M{
		"$set": bson.M{bannerThemeKey: theme},
	}, options.Update().SetUpsert(true))

	return errors.WithStack(err)
}

func GetServiceFlags(ctx context.Context) (*ServiceFlags, error) {
	flags := &ServiceFlags{}
	if err := flags.Get(ctx); err != nil {
		return nil, errors.Wrapf(err, "getting section '%s'", flags.SectionId())
	}
	return flags, nil
}

// SetServiceFlags sets whether each of the runner/API server processes is enabled.
func SetServiceFlags(ctx context.Context, flags ServiceFlags) error {
	return flags.Set(ctx)
}
