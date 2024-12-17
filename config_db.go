package evergreen

import (
	"context"
	"reflect"

	"github.com/evergreen-ci/utility"
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
	idKey                  = bsonutil.MustHaveTag(Settings{}, "Id")
	bannerKey              = bsonutil.MustHaveTag(Settings{}, "Banner")
	bannerThemeKey         = bsonutil.MustHaveTag(Settings{}, "BannerTheme")
	serviceFlagsKey        = bsonutil.MustHaveTag(Settings{}, "ServiceFlags")
	configDirKey           = bsonutil.MustHaveTag(Settings{}, "ConfigDir")
	awsInstanceRoleKey     = bsonutil.MustHaveTag(Settings{}, "AWSInstanceRole")
	cedarKey               = bsonutil.MustHaveTag(Settings{}, "Cedar")
	hostJasperKey          = bsonutil.MustHaveTag(Settings{}, "HostJasper")
	domainNameKey          = bsonutil.MustHaveTag(Settings{}, "DomainName")
	jiraKey                = bsonutil.MustHaveTag(Settings{}, "Jira")
	splunkKey              = bsonutil.MustHaveTag(Settings{}, "Splunk")
	slackKey               = bsonutil.MustHaveTag(Settings{}, "Slack")
	providersKey           = bsonutil.MustHaveTag(Settings{}, "Providers")
	kanopySSHKeyPathKey    = bsonutil.MustHaveTag(Settings{}, "KanopySSHKeyPath")
	authConfigKey          = bsonutil.MustHaveTag(Settings{}, "AuthConfig")
	repoTrackerConfigKey   = bsonutil.MustHaveTag(Settings{}, "RepoTracker")
	apiKey                 = bsonutil.MustHaveTag(Settings{}, "Api")
	uiKey                  = bsonutil.MustHaveTag(Settings{}, "Ui")
	hostInitConfigKey      = bsonutil.MustHaveTag(Settings{}, "HostInit")
	notifyKey              = bsonutil.MustHaveTag(Settings{}, "Notify")
	schedulerConfigKey     = bsonutil.MustHaveTag(Settings{}, "Scheduler")
	amboyKey               = bsonutil.MustHaveTag(Settings{}, "Amboy")
	expansionsKey          = bsonutil.MustHaveTag(Settings{}, "Expansions")
	expansionsNewKey       = bsonutil.MustHaveTag(Settings{}, "ExpansionsNew")
	pluginsKey             = bsonutil.MustHaveTag(Settings{}, "Plugins")
	pluginsNewKey          = bsonutil.MustHaveTag(Settings{}, "PluginsNew")
	loggerConfigKey        = bsonutil.MustHaveTag(Settings{}, "LoggerConfig")
	logPathKey             = bsonutil.MustHaveTag(Settings{}, "LogPath")
	pprofPortKey           = bsonutil.MustHaveTag(Settings{}, "PprofPort")
	githubPRCreatorOrgKey  = bsonutil.MustHaveTag(Settings{}, "GithubPRCreatorOrg")
	githubOrgsKey          = bsonutil.MustHaveTag(Settings{}, "GithubOrgs")
	githubWebhookSecretKey = bsonutil.MustHaveTag(Settings{}, "GithubWebhookSecret")
	disabledGQLQueriesKey  = bsonutil.MustHaveTag(Settings{}, "DisabledGQLQueries")
	containerPoolsKey      = bsonutil.MustHaveTag(Settings{}, "ContainerPools")
	commitQueueKey         = bsonutil.MustHaveTag(Settings{}, "CommitQueue")
	sshKeyDirectoryKey     = bsonutil.MustHaveTag(Settings{}, "SSHKeyDirectory")
	sshKeyPairsKey         = bsonutil.MustHaveTag(Settings{}, "SSHKeyPairs")
	spawnhostKey           = bsonutil.MustHaveTag(Settings{}, "Spawnhost")
	shutdownWaitKey        = bsonutil.MustHaveTag(Settings{}, "ShutdownWaitSeconds")

	sshKeyPairNameKey       = bsonutil.MustHaveTag(SSHKeyPair{}, "Name")
	sshKeyPairEC2RegionsKey = bsonutil.MustHaveTag(SSHKeyPair{}, "EC2Regions")

	// degraded mode flags
	taskDispatchKey                    = bsonutil.MustHaveTag(ServiceFlags{}, "TaskDispatchDisabled")
	hostInitKey                        = bsonutil.MustHaveTag(ServiceFlags{}, "HostInitDisabled")
	podInitDisabledKey                 = bsonutil.MustHaveTag(ServiceFlags{}, "PodInitDisabled")
	largeParserProjectsDisabledKey     = bsonutil.MustHaveTag(ServiceFlags{}, "LargeParserProjectsDisabled")
	monitorKey                         = bsonutil.MustHaveTag(ServiceFlags{}, "MonitorDisabled")
	alertsKey                          = bsonutil.MustHaveTag(ServiceFlags{}, "AlertsDisabled")
	agentStartKey                      = bsonutil.MustHaveTag(ServiceFlags{}, "AgentStartDisabled")
	repotrackerKey                     = bsonutil.MustHaveTag(ServiceFlags{}, "RepotrackerDisabled")
	schedulerKey                       = bsonutil.MustHaveTag(ServiceFlags{}, "SchedulerDisabled")
	checkBlockedTasksKey               = bsonutil.MustHaveTag(ServiceFlags{}, "CheckBlockedTasksDisabled")
	githubPRTestingDisabledKey         = bsonutil.MustHaveTag(ServiceFlags{}, "GithubPRTestingDisabled")
	cliUpdatesDisabledKey              = bsonutil.MustHaveTag(ServiceFlags{}, "CLIUpdatesDisabled")
	backgroundStatsDisabledKey         = bsonutil.MustHaveTag(ServiceFlags{}, "BackgroundStatsDisabled")
	eventProcessingDisabledKey         = bsonutil.MustHaveTag(ServiceFlags{}, "EventProcessingDisabled")
	jiraNotificationsDisabledKey       = bsonutil.MustHaveTag(ServiceFlags{}, "JIRANotificationsDisabled")
	slackNotificationsDisabledKey      = bsonutil.MustHaveTag(ServiceFlags{}, "SlackNotificationsDisabled")
	emailNotificationsDisabledKey      = bsonutil.MustHaveTag(ServiceFlags{}, "EmailNotificationsDisabled")
	webhookNotificationsDisabledKey    = bsonutil.MustHaveTag(ServiceFlags{}, "WebhookNotificationsDisabled")
	githubStatusAPIDisabledKey         = bsonutil.MustHaveTag(ServiceFlags{}, "GithubStatusAPIDisabled")
	taskLoggingDisabledKey             = bsonutil.MustHaveTag(ServiceFlags{}, "TaskLoggingDisabled")
	cacheStatsJobDisabledKey           = bsonutil.MustHaveTag(ServiceFlags{}, "CacheStatsJobDisabled")
	cacheStatsEndpointDisabledKey      = bsonutil.MustHaveTag(ServiceFlags{}, "CacheStatsEndpointDisabled")
	taskReliabilityDisabledKey         = bsonutil.MustHaveTag(ServiceFlags{}, "TaskReliabilityDisabled")
	commitQueueDisabledKey             = bsonutil.MustHaveTag(ServiceFlags{}, "CommitQueueDisabled")
	hostAllocatorDisabledKey           = bsonutil.MustHaveTag(ServiceFlags{}, "HostAllocatorDisabled")
	podAllocatorDisabledKey            = bsonutil.MustHaveTag(ServiceFlags{}, "PodAllocatorDisabled")
	backgroundReauthDisabledKey        = bsonutil.MustHaveTag(ServiceFlags{}, "BackgroundReauthDisabled")
	backgroundCleanupDisabledKey       = bsonutil.MustHaveTag(ServiceFlags{}, "BackgroundCleanupDisabled")
	cloudCleanupDisabledKey            = bsonutil.MustHaveTag(ServiceFlags{}, "CloudCleanupDisabled")
	globalGitHubTokenDisabledKey       = bsonutil.MustHaveTag(ServiceFlags{}, "GlobalGitHubTokenDisabled")
	unrecognizedPodCleanupDisabledKey  = bsonutil.MustHaveTag(ServiceFlags{}, "UnrecognizedPodCleanupDisabled")
	sleepScheduleDisabledKey           = bsonutil.MustHaveTag(ServiceFlags{}, "SleepScheduleDisabled")
	systemFailedTaskRestartDisabledKey = bsonutil.MustHaveTag(ServiceFlags{}, "SystemFailedTaskRestartDisabled")
	cpuDegradedModeDisabledKey         = bsonutil.MustHaveTag(ServiceFlags{}, "CPUDegradedModeDisabled")
	parameterStoreDisabledKey          = bsonutil.MustHaveTag(ServiceFlags{}, "ParameterStoreDisabled")

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
)

func byId(id string) bson.M {
	return bson.M{idKey: id}
}

func byIDs(ids []string) bson.M {
	return bson.M{idKey: bson.M{"$in": ids}}
}

// getSectionsBSON returns the config documents from the database as a slice of [bson.Raw].
// Configuration is always fetched from the shared database. If includeOverrides is true
// the config documents are first fetched from the local database and only fetched from
// the shared database when they're missing from the local database. This means that a
// local documents will override the same document from the shared database.
func getSectionsBSON(ctx context.Context, ids []string, includeOverrides bool) ([]bson.Raw, error) {
	missingIDs := ids
	docs := make([]bson.Raw, 0, len(ids))
	if includeOverrides {
		cur, err := GetEnvironment().DB().Collection(ConfigCollection).Find(ctx, byIDs(ids))
		if err != nil {
			return nil, errors.Wrap(err, "finding local configuration sections")
		}

		if err := cur.All(ctx, &docs); err != nil {
			return nil, errors.Wrap(err, "iterating cursor for local configuration sections")
		}

		var docIDs []string
		for _, doc := range docs {
			id, err := doc.LookupErr("_id")
			if err != nil {
				continue
			}
			idString, ok := id.StringValueOK()
			if !ok {
				continue
			}
			docIDs = append(docIDs, idString)
		}

		missingIDs, _ = utility.StringSliceSymmetricDifference(ids, docIDs)
	}

	if GetEnvironment().SharedDB() == nil || len(missingIDs) == 0 {
		return docs, nil
	}

	cur, err := GetEnvironment().SharedDB().Collection(ConfigCollection).Find(ctx, byIDs(missingIDs))
	if err != nil {
		return nil, errors.Wrap(err, "finding shared configuration sections")
	}

	missingDocs := make([]bson.Raw, 0, len(missingIDs))
	if err := cur.All(ctx, &missingDocs); err != nil {
		return nil, errors.Wrap(err, "iterating cursor for shared configuration sections")
	}

	return append(docs, missingDocs...), nil
}

// getConfigSection fetches a section from the database and deserializes it into the provided
// section. If the document is present in the local database its value is used. Otherwise,
// the document is fetched from the shared database. If the document is missing the value
// of section is reset to its zero value.
func getConfigSection(ctx context.Context, section ConfigSection) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(section.SectionId()))
	if err := res.Err(); err != nil {
		if err != mongo.ErrNoDocuments {
			return errors.Wrapf(err, "getting local config section '%s'", section.SectionId())
		}
		// No document is present in the local database and the shared database is not configured.
		if GetEnvironment().SharedDB() == nil {
			// Reset the section to its zero value.
			reflect.ValueOf(section).Elem().Set(reflect.New(reflect.ValueOf(section).Elem().Type()).Elem())
			return nil
		}
	} else {
		if err := res.Decode(section); err != nil {
			return errors.Wrapf(err, "decoding local config section '%s'", section.SectionId())
		}
		return nil
	}

	res = GetEnvironment().SharedDB().Collection(ConfigCollection).FindOne(ctx, byId(section.SectionId()))
	if err := res.Err(); err != nil {
		if err != mongo.ErrNoDocuments {
			return errors.Wrapf(err, "getting shared config section '%s'", section.SectionId())
		}
		// Reset the section to its zero value.
		reflect.ValueOf(section).Elem().Set(reflect.New(reflect.ValueOf(section).Elem().Type()).Elem())
		return nil
	}

	return errors.Wrapf(res.Decode(section), "decoding shared config section '%s'", section.SectionId())
}

func setConfigSection(ctx context.Context, sectionID string, update bson.M) error {
	db := GetEnvironment().SharedDB()
	if db == nil {
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
