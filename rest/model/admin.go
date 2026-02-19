package model

import (
	"reflect"
	"sort"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

func NewConfigModel() *APIAdminSettings {
	return &APIAdminSettings{
		Amboy:               &APIAmboyConfig{},
		AmboyDB:             &APIAmboyDBConfig{},
		Api:                 &APIapiConfig{},
		AuthConfig:          &APIAuthConfig{},
		Buckets:             &APIBucketsConfig{},
		Cedar:               &APICedarConfig{},
		ContainerPools:      &APIContainerPoolsConfig{},
		Expansions:          map[string]string{},
		Cost:                &APICostConfig{},
		FWS:                 &APIFWSConfig{},
		Graphite:            &APIGraphiteConfig{},
		HostInit:            &APIHostInitConfig{},
		HostJasper:          &APIHostJasperConfig{},
		Jira:                &APIJiraConfig{},
		JIRANotifications:   &APIJIRANotificationsConfig{},
		LoggerConfig:        &APILoggerConfig{},
		Notify:              &APINotifyConfig{},
		Overrides:           &APIOverridesConfig{},
		ParameterStore:      &APIParameterStoreConfig{},
		Plugins:             map[string]map[string]any{},
		ProjectCreation:     &APIProjectCreationConfig{},
		Providers:           &APICloudProviders{},
		RepoTracker:         &APIRepoTrackerConfig{},
		ReleaseMode:         &APIReleaseModeConfig{},
		RuntimeEnvironments: &APIRuntimeEnvironmentsConfig{},
		Scheduler:           &APISchedulerConfig{},
		ServiceFlags:        &APIServiceFlags{},
		SingleTaskDistro:    &APISingleTaskDistroConfig{},
		Slack:               &APISlackConfig{},
		SleepSchedule:       &APISleepScheduleConfig{},
		Splunk:              &APISplunkConfig{},
		SSH:                 &APISSHConfig{},
		TaskLimits:          &APITaskLimitsConfig{},
		TestSelection:       &APITestSelectionConfig{},
		Triggers:            &APITriggerConfig{},
		Ui:                  &APIUIConfig{},
		Spawnhost:           &APISpawnHostConfig{},
		Tracer:              &APITracerSettings{},
		GitHubCheckRun:      &APIGitHubCheckRunConfig{},
		Sage:                &APISageConfig{},
	}
}

// APIAdminSettings is the structure of a response to the admin route
type APIAdminSettings struct {
	Amboy                   *APIAmboyConfig               `json:"amboy,omitempty"`
	AmboyDB                 *APIAmboyDBConfig             `json:"amboy_db,omitempty"`
	Api                     *APIapiConfig                 `json:"api,omitempty"`
	AWSInstanceRole         *string                       `json:"aws_instance_role,omitempty"`
	AuthConfig              *APIAuthConfig                `json:"auth,omitempty"`
	Banner                  *string                       `json:"banner,omitempty"`
	BannerTheme             *string                       `json:"banner_theme,omitempty"`
	Buckets                 *APIBucketsConfig             `json:"buckets,omitempty"`
	Cedar                   *APICedarConfig               `json:"cedar,omitempty"`
	ConfigDir               *string                       `json:"configdir,omitempty"`
	ContainerPools          *APIContainerPoolsConfig      `json:"container_pools,omitempty"`
	DebugSpawnHosts         *APIDebugSpawnHostsConfig     `json:"debug_spawn_hosts,omitempty"`
	DomainName              *string                       `json:"domain_name,omitempty"`
	Expansions              map[string]string             `json:"expansions,omitempty"`
	Cost                    *APICostConfig                `json:"cost,omitempty"`
	FWS                     *APIFWSConfig                 `json:"fws,omitempty"`
	Graphite                *APIGraphiteConfig            `json:"graphite,omitempty"`
	GithubPRCreatorOrg      *string                       `json:"github_pr_creator_org,omitempty"`
	GithubOrgs              []string                      `json:"github_orgs,omitempty"`
	GithubWebhookSecret     *string                       `json:"github_webhook_secret,omitempty"`
	DisabledGQLQueries      []string                      `json:"disabled_gql_queries"`
	HostInit                *APIHostInitConfig            `json:"hostinit,omitempty"`
	HostJasper              *APIHostJasperConfig          `json:"host_jasper,omitempty"`
	Jira                    *APIJiraConfig                `json:"jira,omitempty"`
	JIRANotifications       *APIJIRANotificationsConfig   `json:"jira_notifications,omitempty"`
	KanopySSHKeyPath        *string                       `json:"kanopy_ssh_key_path,omitempty"`
	LoggerConfig            *APILoggerConfig              `json:"logger_config,omitempty"`
	LogPath                 *string                       `json:"log_path,omitempty"`
	Notify                  *APINotifyConfig              `json:"notify,omitempty"`
	OldestAllowedCLIVersion *string                       `json:"oldest_allowed_cli_version"`
	Overrides               *APIOverridesConfig           `json:"overrides,omitempty"`
	ParameterStore          *APIParameterStoreConfig      `json:"parameter_store,omitempty"`
	PerfMonitoringURL       *string                       `json:"perf_monitoring_url"`
	PerfMonitoringKanopyURL *string                       `json:"perf_monitoring_kanopy_url"`
	Plugins                 map[string]map[string]any     `json:"plugins,omitempty"`
	PprofPort               *string                       `json:"pprof_port,omitempty"`
	ProjectCreation         *APIProjectCreationConfig     `json:"project_creation,omitempty"`
	Providers               *APICloudProviders            `json:"providers,omitempty"`
	RepoTracker             *APIRepoTrackerConfig         `json:"repotracker,omitempty"`
	ReleaseMode             *APIReleaseModeConfig         `json:"release_mode,omitempty"`
	RuntimeEnvironments     *APIRuntimeEnvironmentsConfig `json:"runtime_environments,omitempty"`
	Scheduler               *APISchedulerConfig           `json:"scheduler,omitempty"`
	ServiceFlags            *APIServiceFlags              `json:"service_flags,omitempty"`
	SingleTaskDistro        *APISingleTaskDistroConfig    `json:"single_task_distro,omitempty"`
	Slack                   *APISlackConfig               `json:"slack,omitempty"`
	SleepSchedule           *APISleepScheduleConfig       `json:"sleep_schedule,omitempty"`
	SSH                     *APISSHConfig                 `json:"ssh,omitempty"`
	Splunk                  *APISplunkConfig              `json:"splunk,omitempty"`
	TaskLimits              *APITaskLimitsConfig          `json:"task_limits,omitempty"`
	TestSelection           *APITestSelectionConfig       `json:"test_selection,omitempty"`
	Triggers                *APITriggerConfig             `json:"triggers,omitempty"`
	Ui                      *APIUIConfig                  `json:"ui,omitempty"`
	Spawnhost               *APISpawnHostConfig           `json:"spawnhost,omitempty"`
	Tracer                  *APITracerSettings            `json:"tracer,omitempty"`
	GitHubCheckRun          *APIGitHubCheckRunConfig      `json:"github_check_run,omitempty"`
	ShutdownWaitSeconds     *int                          `json:"shutdown_wait_seconds,omitempty"`
	Sage                    *APISageConfig                `json:"sage,omitempty"`
}

const (
	OktaPreferredType   = "okta"
	NaivePreferredType  = "naive"
	GithubPreferredType = "github"
	MultiPreferredType  = "multi"
	KanopyPreferredType = "kanopy"
)

// BuildFromService builds a model from the service layer
func (as *APIAdminSettings) BuildFromService(h any) error {
	switch v := h.(type) {
	case *evergreen.Settings:
		if v == nil {
			return errors.New("cannot convert nil admin settings to API model")
		}
		apiModelReflect := reflect.ValueOf(*as)
		dbModelReflect := reflect.ValueOf(*v)
		for i := 0; i < apiModelReflect.NumField(); i++ {
			propName := apiModelReflect.Type().Field(i).Name
			val := apiModelReflect.FieldByName(propName)
			if val.IsNil() {
				continue
			}

			// check to see if this property is an API model itself
			interfaceVal := val.Interface()
			model, ok := interfaceVal.(Model)
			if !ok {
				continue
			}
			// build the sub-model from the DB model. assumes that the 2 fields are named the same thing
			if err := model.BuildFromService(dbModelReflect.FieldByName(propName).Interface()); err != nil {
				return errors.Wrapf(err, "converting admin model section '%s' to API model", propName)
			}
		}
		as.AWSInstanceRole = utility.ToStringPtr(v.AWSInstanceRole)
		as.Banner = &v.Banner
		tmp := string(v.BannerTheme)
		as.BannerTheme = &tmp
		as.ConfigDir = &v.ConfigDir
		as.DomainName = utility.ToStringPtr(v.DomainName)
		as.OldestAllowedCLIVersion = utility.ToStringPtr(v.OldestAllowedCLIVersion)
		as.GithubPRCreatorOrg = &v.GithubPRCreatorOrg
		as.LogPath = &v.LogPath
		as.PerfMonitoringURL = &v.PerfMonitoringURL
		as.PerfMonitoringKanopyURL = &v.PerfMonitoringKanopyURL
		as.Plugins = v.Plugins
		as.PprofPort = &v.PprofPort
		as.Expansions = v.Expansions
		as.KanopySSHKeyPath = utility.ToStringPtr(v.KanopySSHKeyPath)
		as.GithubOrgs = v.GithubOrgs
		as.GithubWebhookSecret = utility.ToStringPtr(v.GithubWebhookSecret)
		as.DisabledGQLQueries = v.DisabledGQLQueries
		uiConfig := APIUIConfig{}
		err := uiConfig.BuildFromService(v.Ui)
		if err != nil {
			return errors.Wrap(err, "converting UI config to API model")
		}
		as.Ui = &uiConfig
		jiraConfig := APIJiraConfig{}
		err = jiraConfig.BuildFromService(v.Jira)
		if err != nil {
			return errors.Wrap(err, "converting Jira config to API model")
		}
		as.Jira = &jiraConfig
		runtimeEnvironmentsConfig := APIRuntimeEnvironmentsConfig{}
		err = runtimeEnvironmentsConfig.BuildFromService(v.RuntimeEnvironments)
		if err != nil {
			return errors.Wrap(err, "converting Runtime Environments config to API model")
		}
		as.RuntimeEnvironments = &runtimeEnvironmentsConfig
		cloudProviders := APICloudProviders{}
		err = cloudProviders.BuildFromService(v.Providers)
		if err != nil {
			return errors.Wrap(err, "converting cloud provider config to API model")
		}
		as.Providers = &cloudProviders
		as.ShutdownWaitSeconds = &v.ShutdownWaitSeconds
		spawnHostConfig := APISpawnHostConfig{}
		err = spawnHostConfig.BuildFromService(v.Spawnhost)
		if err != nil {
			return errors.Wrap(err, "converting spawn host config to API model")
		}
		as.Spawnhost = &spawnHostConfig
		debugSpawnHostsConfig := APIDebugSpawnHostsConfig{}
		err = debugSpawnHostsConfig.BuildFromService(v.DebugSpawnHosts)
		if err != nil {
			return errors.Wrap(err, "converting debug spawn hosts config to API model")
		}
		as.DebugSpawnHosts = &debugSpawnHostsConfig
		slackConfig := APISlackConfig{}
		err = slackConfig.BuildFromService(v.Slack)
		if err != nil {
			return errors.Wrap(err, "converting slack config to API model")
		}
		as.Slack = &slackConfig
		splunkConfig := APISplunkConfig{}
		if err = splunkConfig.BuildFromService(v.Splunk); err != nil {
			return errors.Wrap(err, "converting splunk config to API model")
		}
		as.Splunk = &splunkConfig
		containerPoolsConfig := APIContainerPoolsConfig{}
		if err = containerPoolsConfig.BuildFromService(v.ContainerPools); err != nil {
			return errors.Wrap(err, "converting container pools config to API model")
		}
		as.ContainerPools = &containerPoolsConfig
		singleTaskDistroConfig := APISingleTaskDistroConfig{}
		if err = singleTaskDistroConfig.BuildFromService(v.SingleTaskDistro); err != nil {
			return errors.Wrap(err, "converting single task distro config to API model")
		}
		as.SingleTaskDistro = &singleTaskDistroConfig
		releaseModeConfig := APIReleaseModeConfig{}
		if err = releaseModeConfig.BuildFromService(v.ReleaseMode); err != nil {
			return errors.Wrap(err, "converting release mode config to API model")
		}
		as.Cedar = &APICedarConfig{}
		if err = as.Cedar.BuildFromService(v.Cedar); err != nil {
			return errors.Wrap(err, "converting cedar config to API model")
		}
		as.ServiceFlags = &APIServiceFlags{}
		if err = as.ServiceFlags.BuildFromService(v.ServiceFlags); err != nil {
			return errors.Wrap(err, "converting service flags to API model")
		}
		as.ReleaseMode = &releaseModeConfig
	default:
		return errors.Errorf("programmatic error: expected admin settings but got type %T", h)
	}
	return nil
}

// ToService returns a service model from an API model
func (as *APIAdminSettings) ToService() (any, error) {
	settings := evergreen.Settings{
		Expansions:         map[string]string{},
		Plugins:            evergreen.PluginConfig{},
		GithubOrgs:         as.GithubOrgs,
		DisabledGQLQueries: as.DisabledGQLQueries,
	}
	if as.AWSInstanceRole != nil {
		settings.AWSInstanceRole = *as.AWSInstanceRole
	}
	if as.Banner != nil {
		settings.Banner = *as.Banner
	}
	if as.BannerTheme != nil {
		settings.BannerTheme = evergreen.BannerTheme(*as.BannerTheme)
	}
	if as.ConfigDir != nil {
		settings.ConfigDir = *as.ConfigDir
	}
	settings.DomainName = utility.FromStringPtr(as.DomainName)

	if as.GithubPRCreatorOrg != nil {
		settings.GithubPRCreatorOrg = *as.GithubPRCreatorOrg
	}
	settings.GithubWebhookSecret = utility.FromStringPtr(as.GithubWebhookSecret)
	if as.LogPath != nil {
		settings.LogPath = *as.LogPath
	}
	settings.OldestAllowedCLIVersion = utility.FromStringPtr(as.OldestAllowedCLIVersion)
	if as.PprofPort != nil {
		settings.PprofPort = *as.PprofPort
	}
	if as.PerfMonitoringURL != nil {
		settings.PerfMonitoringURL = *as.PerfMonitoringURL
	}
	if as.PerfMonitoringKanopyURL != nil {
		settings.PerfMonitoringKanopyURL = *as.PerfMonitoringKanopyURL
	}
	if as.ServiceFlags != nil {
		sf, err := as.ServiceFlags.ToService()
		if err != nil {
			return nil, errors.Wrap(err, "converting service flags to service model")
		}
		settings.ServiceFlags = sf.(evergreen.ServiceFlags)
	}

	apiModelReflect := reflect.ValueOf(*as)
	dbModelReflect := reflect.ValueOf(&settings).Elem()
	for i := 0; i < apiModelReflect.NumField(); i++ {
		propName := apiModelReflect.Type().Field(i).Name
		val := apiModelReflect.FieldByName(propName)
		if val.IsNil() {
			continue
		}

		// check to see if this property is an API model itself
		interfaceVal := val.Interface()
		model, ok := interfaceVal.(Model)
		if !ok {
			continue
		}
		// set the corresponding DB model field. assumes that the 2 fields are named the same thing
		i, err := model.ToService()
		if err != nil {
			return nil, errors.Wrapf(err, "converting admin model section '%s' to service model", propName)
		}
		valToSet := reflect.ValueOf(i)
		dbModelReflect.FieldByName(propName).Set(valToSet)
	}
	for k, v := range as.Expansions {
		settings.Expansions[k] = v
	}
	settings.KanopySSHKeyPath = utility.FromStringPtr(as.KanopySSHKeyPath)
	for k, v := range as.Plugins {
		settings.Plugins[k] = map[string]any{}
		for k2, v2 := range v {
			settings.Plugins[k][k2] = v2
		}
	}

	if as.ShutdownWaitSeconds != nil {
		settings.ShutdownWaitSeconds = *as.ShutdownWaitSeconds
	}
	return settings, nil
}

type APISESConfig struct {
	SenderAddress *string `json:"sender_address"`
}

func (a *APISESConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.SESConfig:
		a.SenderAddress = utility.ToStringPtr(v.SenderAddress)
	default:
		return errors.Errorf("programmatic error: expected SESConfig but got type %T", h)
	}
	return nil
}

func (a *APISESConfig) ToService() (any, error) {
	if a == nil {
		return nil, nil
	}
	config := evergreen.SESConfig{
		SenderAddress: utility.FromStringPtr(a.SenderAddress),
	}
	return config, nil
}

type APIAmboyConfig struct {
	Name                                  *string                    `json:"name"`
	SingleName                            *string                    `json:"single_name"`
	PoolSizeLocal                         int                        `json:"pool_size_local"`
	PoolSizeRemote                        int                        `json:"pool_size_remote"`
	LocalStorage                          int                        `json:"local_storage_size"`
	GroupDefaultWorkers                   int                        `json:"group_default_workers"`
	GroupBackgroundCreateFrequencyMinutes int                        `json:"group_background_create_frequency"`
	GroupPruneFrequencyMinutes            int                        `json:"group_prune_frequency"`
	GroupTTLMinutes                       int                        `json:"group_ttl"`
	LockTimeoutMinutes                    int                        `json:"lock_timeout_minutes"`
	SampleSize                            int                        `json:"sample_size"`
	Retry                                 APIAmboyRetryConfig        `json:"retry"`
	NamedQueues                           []APIAmboyNamedQueueConfig `json:"named_queues,omitempty"`
}

func (a *APIAmboyConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.AmboyConfig:
		a.Name = utility.ToStringPtr(v.Name)
		a.SingleName = utility.ToStringPtr(v.SingleName)
		a.PoolSizeLocal = v.PoolSizeLocal
		a.PoolSizeRemote = v.PoolSizeRemote
		a.LocalStorage = v.LocalStorage
		a.GroupDefaultWorkers = v.GroupDefaultWorkers
		a.GroupBackgroundCreateFrequencyMinutes = v.GroupBackgroundCreateFrequencyMinutes
		a.GroupPruneFrequencyMinutes = v.GroupPruneFrequencyMinutes
		a.GroupTTLMinutes = v.GroupTTLMinutes
		a.LockTimeoutMinutes = v.LockTimeoutMinutes
		a.SampleSize = v.SampleSize
		if err := a.Retry.BuildFromService(v.Retry); err != nil {
			return errors.Wrap(err, "converting Amboy retry settings to API model")
		}
		for _, dbNamedQueue := range v.NamedQueues {
			var apiNamedQueue APIAmboyNamedQueueConfig
			apiNamedQueue.BuildFromService(dbNamedQueue)
			a.NamedQueues = append(a.NamedQueues, apiNamedQueue)
		}
	default:
		return errors.Errorf("programmatic error: expected Amboy config but got type %T", h)
	}
	return nil
}

func (a *APIAmboyConfig) ToService() (any, error) {
	i, err := a.Retry.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting Amboy retry settings to service model")
	}
	retry, ok := i.(evergreen.AmboyRetryConfig)
	if !ok {
		return nil, errors.Errorf("programmatic error: expected Amboy retry config but got type %T", i)
	}

	var dbNamedQueues []evergreen.AmboyNamedQueueConfig
	for _, apiNamedQueue := range a.NamedQueues {
		dbNamedQueues = append(dbNamedQueues, apiNamedQueue.ToService())
	}
	return evergreen.AmboyConfig{
		Name:                                  utility.FromStringPtr(a.Name),
		SingleName:                            utility.FromStringPtr(a.SingleName),
		PoolSizeLocal:                         a.PoolSizeLocal,
		PoolSizeRemote:                        a.PoolSizeRemote,
		LocalStorage:                          a.LocalStorage,
		GroupDefaultWorkers:                   a.GroupDefaultWorkers,
		GroupBackgroundCreateFrequencyMinutes: a.GroupBackgroundCreateFrequencyMinutes,
		GroupPruneFrequencyMinutes:            a.GroupPruneFrequencyMinutes,
		GroupTTLMinutes:                       a.GroupTTLMinutes,
		LockTimeoutMinutes:                    a.LockTimeoutMinutes,
		SampleSize:                            a.SampleSize,
		Retry:                                 retry,
		NamedQueues:                           dbNamedQueues,
	}, nil
}

type APIAmboyDBConfig struct {
	URL      *string `json:"url"`
	Database *string `json:"database"`
}

func (a *APIAmboyDBConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.AmboyDBConfig:
		a.URL = utility.ToStringPtr(v.URL)
		a.Database = utility.ToStringPtr(v.Database)
		return nil
	default:
		return errors.Errorf("programmatic error: expected Amboy DB config but got type %T", h)
	}
}

func (a *APIAmboyDBConfig) ToService() (any, error) {
	return evergreen.AmboyDBConfig{
		URL:      utility.FromStringPtr(a.URL),
		Database: utility.FromStringPtr(a.Database),
	}, nil
}

type APIAmboyRetryConfig struct {
	NumWorkers                          int `json:"num_workers,omitempty"`
	MaxCapacity                         int `json:"max_capacity,omitempty"`
	MaxRetryAttempts                    int `json:"max_retry_attempts,omitempty"`
	MaxRetryTimeSeconds                 int `json:"max_retry_time_seconds,omitempty"`
	RetryBackoffSeconds                 int `json:"retry_backoff_seconds,omitempty"`
	StaleRetryingMonitorIntervalSeconds int `json:"stale_retrying_monitor_interval_seconds,omitempty"`
}

func (a *APIAmboyRetryConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.AmboyRetryConfig:
		a.NumWorkers = v.NumWorkers
		a.MaxCapacity = v.MaxCapacity
		a.MaxRetryAttempts = v.MaxRetryAttempts
		a.MaxRetryTimeSeconds = v.MaxRetryTimeSeconds
		a.RetryBackoffSeconds = v.RetryBackoffSeconds
		a.StaleRetryingMonitorIntervalSeconds = v.StaleRetryingMonitorIntervalSeconds
		return nil
	default:
		return errors.Errorf("programmatic error: expected Amboy retry config but got type %T", h)
	}
}

func (a *APIAmboyRetryConfig) ToService() (any, error) {
	return evergreen.AmboyRetryConfig{
		NumWorkers:                          a.NumWorkers,
		MaxCapacity:                         a.MaxCapacity,
		MaxRetryAttempts:                    a.MaxRetryAttempts,
		MaxRetryTimeSeconds:                 a.MaxRetryTimeSeconds,
		RetryBackoffSeconds:                 a.RetryBackoffSeconds,
		StaleRetryingMonitorIntervalSeconds: a.StaleRetryingMonitorIntervalSeconds,
	}, nil
}

// APIAmboyNamedQueueConfig is the model for named Amboy queue settings.
type APIAmboyNamedQueueConfig struct {
	Name               *string `json:"name"`
	Regexp             *string `json:"regexp"`
	NumWorkers         int     `json:"num_workers,omitempty"`
	SampleSize         int     `json:"sample_size,omitempty"`
	LockTimeoutSeconds int     `json:"lock_timeout_seconds,omitempty"`
}

func (a *APIAmboyNamedQueueConfig) BuildFromService(h evergreen.AmboyNamedQueueConfig) {
	a.Name = utility.ToStringPtr(h.Name)
	a.Regexp = utility.ToStringPtr(h.Regexp)
	a.NumWorkers = h.NumWorkers
	a.SampleSize = h.SampleSize
	a.LockTimeoutSeconds = h.LockTimeoutSeconds
}

func (a *APIAmboyNamedQueueConfig) ToService() evergreen.AmboyNamedQueueConfig {
	return evergreen.AmboyNamedQueueConfig{
		Name:               utility.FromStringPtr(a.Name),
		Regexp:             utility.FromStringPtr(a.Regexp),
		NumWorkers:         a.NumWorkers,
		SampleSize:         a.SampleSize,
		LockTimeoutSeconds: a.LockTimeoutSeconds,
	}
}

type APIapiConfig struct {
	HttpListenAddr *string `json:"http_listen_addr"`
	URL            *string `json:"url"`
	CorpURL        *string `json:"corp_url"`
}

func (a *APIapiConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.APIConfig:
		a.HttpListenAddr = utility.ToStringPtr(v.HttpListenAddr)
		a.URL = utility.ToStringPtr(v.URL)
		a.CorpURL = utility.ToStringPtr(v.CorpURL)
	default:
		return errors.Errorf("programmatic error: expected REST API config but got type %T", h)
	}
	return nil
}

func (a *APIapiConfig) ToService() (any, error) {
	return evergreen.APIConfig{
		HttpListenAddr: utility.FromStringPtr(a.HttpListenAddr),
		URL:            utility.FromStringPtr(a.URL),
		CorpURL:        utility.FromStringPtr(a.CorpURL),
	}, nil
}

type APIAuthConfig struct {
	Okta                    *APIOktaConfig       `json:"okta"`
	Naive                   *APINaiveAuthConfig  `json:"naive"`
	Github                  *APIGithubAuthConfig `json:"github"`
	Multi                   *APIMultiAuthConfig  `json:"multi"`
	Kanopy                  *APIKanopyAuthConfig `json:"kanopy"`
	OAuth                   *APIOAuthConfig      `json:"oauth"`
	PreferredType           *string              `json:"preferred_type"`
	BackgroundReauthMinutes int                  `json:"background_reauth_minutes"`
	AllowServiceUsers       bool                 `json:"allow_service_users"`
}

func (a *APIAuthConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.AuthConfig:
		if v.Okta != nil {
			a.Okta = &APIOktaConfig{}
			if err := a.Okta.BuildFromService(v.Okta); err != nil {
				return errors.Wrap(err, "converting Okta auth settings to API model")
			}
		}
		if v.Github != nil {
			a.Github = &APIGithubAuthConfig{}
			if err := a.Github.BuildFromService(v.Github); err != nil {
				return errors.Wrap(err, "converting GitHub auth settings to API model")
			}
		}
		if v.Naive != nil {
			a.Naive = &APINaiveAuthConfig{}
			if err := a.Naive.BuildFromService(v.Naive); err != nil {
				return errors.Wrap(err, "converting naive auth settings to API model")
			}
		}
		if v.Multi != nil {
			a.Multi = &APIMultiAuthConfig{}
			if err := a.Multi.BuildFromService(v.Multi); err != nil {
				return errors.Wrap(err, "converting multi auth settings to API model")
			}
		}
		if v.Kanopy != nil {
			a.Kanopy = &APIKanopyAuthConfig{}
			if err := a.Kanopy.BuildFromService(v.Kanopy); err != nil {
				return errors.Wrap(err, "converting Kanopy auth settings to API model")
			}
		}
		if v.OAuth != nil {
			a.OAuth = &APIOAuthConfig{}
			if err := a.OAuth.BuildFromService(v.OAuth); err != nil {
				return errors.Wrap(err, "converting OAuth settings to API model")
			}
		}
		a.PreferredType = utility.ToStringPtr(v.PreferredType)
		a.BackgroundReauthMinutes = v.BackgroundReauthMinutes
		a.AllowServiceUsers = v.AllowServiceUsers
	default:
		return errors.Errorf("programmatic error: expected auth config but got type %T", h)
	}
	return nil
}

func (a *APIAuthConfig) ToService() (any, error) {
	var okta *evergreen.OktaConfig
	var naive *evergreen.NaiveAuthConfig
	var github *evergreen.GithubAuthConfig
	var multi *evergreen.MultiAuthConfig
	var kanopy *evergreen.KanopyAuthConfig
	var oauth *evergreen.OAuthConfig
	var ok bool

	i, err := a.Okta.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting Okta auth config to service model")
	}
	if i != nil {
		okta, ok = i.(*evergreen.OktaConfig)
		if !ok {
			return nil, errors.Errorf("programmatic error: expected Okta auth config but got type %T", i)
		}
	}

	i, err = a.Naive.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting naive auth config to service model")
	}
	if i != nil {
		naive, ok = i.(*evergreen.NaiveAuthConfig)
		if !ok {
			return nil, errors.Errorf("programmatic error: expected naive auth config but got type %T", i)
		}
	}

	i, err = a.Github.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting GitHub auth config to service model")
	}
	if i != nil {
		github, ok = i.(*evergreen.GithubAuthConfig)
		if !ok {
			return nil, errors.Errorf("programmatic error: expected GitHub auth config but got type %T", i)
		}
	}

	i, err = a.Multi.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting multi auth config to service model")
	}
	if i != nil {
		multi, ok = i.(*evergreen.MultiAuthConfig)
		if !ok {
			return nil, errors.Errorf("programmatic error: expected multi auth config but got type %T", i)
		}
	}

	i, err = a.Kanopy.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting Kanopy auth config to service model")
	}
	if i != nil {
		kanopy, ok = i.(*evergreen.KanopyAuthConfig)
		if !ok {
			return nil, errors.Errorf("programmatic error: expected Kanopy auth config but got type %T", i)
		}
	}
	i, err = a.OAuth.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting OAuth config to service model")
	}
	if i != nil {
		oauth, ok = i.(*evergreen.OAuthConfig)
		if !ok {
			return nil, errors.Errorf("programmatic error: expected OAuth config but got type %T", i)
		}
	}

	return evergreen.AuthConfig{
		Okta:                    okta,
		Naive:                   naive,
		Github:                  github,
		Multi:                   multi,
		Kanopy:                  kanopy,
		OAuth:                   oauth,
		PreferredType:           utility.FromStringPtr(a.PreferredType),
		BackgroundReauthMinutes: a.BackgroundReauthMinutes,
		AllowServiceUsers:       a.AllowServiceUsers,
	}, nil
}

type APIBucketsConfig struct {
	LogBucket              APIBucketConfig  `json:"log_bucket"`
	LogBucketLongRetention APIBucketConfig  `json:"log_bucket_long_retention"`
	LogBucketFailedTasks   APIBucketConfig  `json:"log_bucket_failed_tasks"`
	LongRetentionProjects  []string         `json:"long_retention_projects"`
	TestResultsBucket      APIBucketConfig  `json:"test_results_bucket"`
	InternalBuckets        []string         `json:"internal_buckets"`
	Credentials            APIS3Credentials `json:"credentials"`
}

type APIBucketConfig struct {
	Name              *string `json:"name"`
	Type              *string `json:"type"`
	DBName            *string `json:"db_name"`
	TestResultsPrefix *string `json:"test_results_prefix"`
	RoleARN           *string `json:"role_arn"`
}

type APIProjectToPrefixMapping struct {
	ProjectID *string `json:"project_id"`
	Prefix    *string `json:"prefix"`
}

type APIProjectToBucketMapping struct {
	ProjectID *string `json:"project_id"`
	Bucket    *string `json:"bucket"`
	Prefix    *string `json:"prefix"`
}

func (a *APIBucketsConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.BucketsConfig:
		a.LogBucket.Name = utility.ToStringPtr(v.LogBucket.Name)
		a.LogBucket.Type = utility.ToStringPtr(string(v.LogBucket.Type))
		a.LogBucket.DBName = utility.ToStringPtr(v.LogBucket.DBName)

		a.LogBucketLongRetention.Name = utility.ToStringPtr(v.LogBucketLongRetention.Name)
		a.LogBucketLongRetention.Type = utility.ToStringPtr(string(v.LogBucketLongRetention.Type))
		a.LogBucketLongRetention.DBName = utility.ToStringPtr(v.LogBucketLongRetention.DBName)
		a.LogBucketLongRetention.RoleARN = utility.ToStringPtr(v.LogBucketLongRetention.RoleARN)

		a.LogBucketFailedTasks.Name = utility.ToStringPtr(v.LogBucketFailedTasks.Name)
		a.LogBucketFailedTasks.Type = utility.ToStringPtr(string(v.LogBucketFailedTasks.Type))
		a.LogBucketFailedTasks.DBName = utility.ToStringPtr(v.LogBucketFailedTasks.DBName)
		a.LogBucketFailedTasks.RoleARN = utility.ToStringPtr(v.LogBucketFailedTasks.RoleARN)

		a.LongRetentionProjects = v.LongRetentionProjects

		a.TestResultsBucket.Name = utility.ToStringPtr(v.TestResultsBucket.Name)
		a.TestResultsBucket.Type = utility.ToStringPtr(string(v.TestResultsBucket.Type))
		a.TestResultsBucket.DBName = utility.ToStringPtr(v.TestResultsBucket.DBName)
		a.TestResultsBucket.TestResultsPrefix = utility.ToStringPtr(v.TestResultsBucket.TestResultsPrefix)
		a.TestResultsBucket.RoleARN = utility.ToStringPtr(v.TestResultsBucket.RoleARN)

		creds := APIS3Credentials{}
		if err := creds.BuildFromService(v.Credentials); err != nil {
			return errors.Wrap(err, "converting S3 credentials to API model")
		}
		a.Credentials = creds
	default:
		return errors.Errorf("programmatic error: expected bucket config but got type %T", h)
	}
	return nil
}

func (a *APIBucketsConfig) ToService() (any, error) {
	i, err := a.Credentials.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting S3 credentials to service model")
	}
	creds, ok := i.(evergreen.S3Credentials)
	if !ok {
		return nil, errors.Errorf("programmatic error: expected S3 credentials but got type %T", i)
	}

	return evergreen.BucketsConfig{
		LogBucket: evergreen.BucketConfig{
			Name:   utility.FromStringPtr(a.LogBucket.Name),
			Type:   evergreen.BucketType(utility.FromStringPtr(a.LogBucket.Type)),
			DBName: utility.FromStringPtr(a.LogBucket.DBName),
		},
		LogBucketLongRetention: evergreen.BucketConfig{
			Name:    utility.FromStringPtr(a.LogBucketLongRetention.Name),
			Type:    evergreen.BucketType(utility.FromStringPtr(a.LogBucketLongRetention.Type)),
			DBName:  utility.FromStringPtr(a.LogBucketLongRetention.DBName),
			RoleARN: utility.FromStringPtr(a.LogBucketLongRetention.RoleARN),
		},
		LogBucketFailedTasks: evergreen.BucketConfig{
			Name:    utility.FromStringPtr(a.LogBucketFailedTasks.Name),
			Type:    evergreen.BucketType(utility.FromStringPtr(a.LogBucketFailedTasks.Type)),
			DBName:  utility.FromStringPtr(a.LogBucketFailedTasks.DBName),
			RoleARN: utility.FromStringPtr(a.LogBucketFailedTasks.RoleARN),
		},
		LongRetentionProjects: a.LongRetentionProjects,
		TestResultsBucket: evergreen.BucketConfig{
			Name:              utility.FromStringPtr(a.TestResultsBucket.Name),
			Type:              evergreen.BucketType(utility.FromStringPtr(a.TestResultsBucket.Type)),
			DBName:            utility.FromStringPtr(a.TestResultsBucket.DBName),
			RoleARN:           utility.FromStringPtr(a.TestResultsBucket.RoleARN),
			TestResultsPrefix: utility.FromStringPtr(a.TestResultsBucket.TestResultsPrefix),
		},
		Credentials: creds,
	}, nil
}

type APICedarConfig struct {
	DBURL  *string `json:"db_url"`
	DBName *string `json:"db_name"`
}

func (a *APICedarConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.CedarConfig:
		a.DBURL = utility.ToStringPtr(v.DBURL)
		a.DBName = utility.ToStringPtr(v.DBName)
	default:
		return errors.Errorf("programmatic error: expected Cedar config but got type %T", h)
	}
	return nil
}

func (a *APICedarConfig) ToService() (any, error) {
	return evergreen.CedarConfig{
		DBURL:  utility.FromStringPtr(a.DBURL),
		DBName: utility.FromStringPtr(a.DBName),
	}, nil
}

type APIOktaConfig struct {
	ClientID           *string  `json:"client_id"`
	ClientSecret       *string  `json:"client_secret"`
	Issuer             *string  `json:"issuer"`
	Scopes             []string `json:"scopes"`
	UserGroup          *string  `json:"user_group"`
	ExpireAfterMinutes int      `json:"expire_after_minutes"`
}

func (a *APIOktaConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case *evergreen.OktaConfig:
		if v == nil {
			return nil
		}
		a.ClientID = utility.ToStringPtr(v.ClientID)
		a.ClientSecret = utility.ToStringPtr(v.ClientSecret)
		a.Issuer = utility.ToStringPtr(v.Issuer)
		a.Scopes = v.Scopes
		a.UserGroup = utility.ToStringPtr(v.UserGroup)
		a.ExpireAfterMinutes = v.ExpireAfterMinutes
		return nil
	default:
		return errors.Errorf("programmatic error: expected Okta config but got type %T", h)
	}
}

func (a *APIOktaConfig) ToService() (any, error) {
	if a == nil {
		return nil, nil
	}
	return &evergreen.OktaConfig{
		ClientID:           utility.FromStringPtr(a.ClientID),
		ClientSecret:       utility.FromStringPtr(a.ClientSecret),
		Issuer:             utility.FromStringPtr(a.Issuer),
		Scopes:             a.Scopes,
		UserGroup:          utility.FromStringPtr(a.UserGroup),
		ExpireAfterMinutes: a.ExpireAfterMinutes,
	}, nil
}

type APINaiveAuthConfig struct {
	Users []APIAuthUser `json:"users"`
}

func (a *APINaiveAuthConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case *evergreen.NaiveAuthConfig:
		if v == nil {
			return nil
		}
		for _, u := range v.Users {
			apiUser := APIAuthUser{}
			if err := apiUser.BuildFromService(u); err != nil {
				return err
			}
			a.Users = append(a.Users, apiUser)
		}
	default:
		return errors.Errorf("programmatic error: expected naive auth config but got type %T", h)
	}
	return nil
}

func (a *APINaiveAuthConfig) ToService() (any, error) {
	if a == nil {
		return nil, nil
	}
	config := evergreen.NaiveAuthConfig{}
	for _, u := range a.Users {
		i, err := u.ToService()
		if err != nil {
			return nil, err
		}
		user, ok := i.(evergreen.AuthUser)
		if !ok {
			continue
		}
		config.Users = append(config.Users, user)
	}
	return &config, nil
}

type APIAuthUser struct {
	Username    *string `json:"username"`
	DisplayName *string `json:"display_name"`
	Password    *string `json:"password"`
	Email       *string `json:"email"`
}

func (a *APIAuthUser) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.AuthUser:
		a.Username = utility.ToStringPtr(v.Username)
		a.Password = utility.ToStringPtr(v.Password)
		a.DisplayName = utility.ToStringPtr(v.DisplayName)
		a.Email = utility.ToStringPtr(v.Email)
	default:
		return errors.Errorf("programmatic error: expected naive auth user config but got type %T", h)
	}
	return nil
}

func (a *APIAuthUser) ToService() (any, error) {
	if a == nil {
		return nil, nil
	}
	return evergreen.AuthUser{
		Username:    utility.FromStringPtr(a.Username),
		Password:    utility.FromStringPtr(a.Password),
		DisplayName: utility.FromStringPtr(a.DisplayName),
		Email:       utility.FromStringPtr(a.Email),
	}, nil
}

type APIGithubAuthConfig struct {
	AppId        int64     `json:"app_id"`
	ClientId     *string   `json:"client_id"`
	ClientSecret *string   `json:"client_secret"`
	DefaultOwner *string   `json:"default_owner"`
	DefaultRepo  *string   `json:"default_repo"`
	Organization *string   `json:"organization"`
	Users        []*string `json:"users"`
}

func (a *APIGithubAuthConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case *evergreen.GithubAuthConfig:
		if v == nil {
			return nil
		}
		a.ClientId = utility.ToStringPtr(v.ClientId)
		a.ClientSecret = utility.ToStringPtr(v.ClientSecret)
		a.Organization = utility.ToStringPtr(v.Organization)
		a.DefaultOwner = utility.ToStringPtr(v.DefaultOwner)
		a.DefaultRepo = utility.ToStringPtr(v.DefaultRepo)
		a.AppId = v.AppId
		for _, u := range v.Users {
			a.Users = append(a.Users, utility.ToStringPtr(u))
		}
	default:
		return errors.Errorf("programmatic error: expected GitHub auth config but got type %T", h)
	}
	return nil
}

func (a *APIGithubAuthConfig) ToService() (any, error) {
	if a == nil {
		return nil, nil
	}
	config := evergreen.GithubAuthConfig{
		ClientId:     utility.FromStringPtr(a.ClientId),
		ClientSecret: utility.FromStringPtr(a.ClientSecret),
		Organization: utility.FromStringPtr(a.Organization),
		DefaultOwner: utility.FromStringPtr(a.DefaultOwner),
		DefaultRepo:  utility.FromStringPtr(a.DefaultRepo),
		AppId:        a.AppId,
	}
	for _, u := range a.Users {
		config.Users = append(config.Users, utility.FromStringPtr(u))
	}
	return &config, nil
}

type APIMultiAuthConfig struct {
	ReadWrite []string `json:"read_write"`
	ReadOnly  []string `json:"read_only"`
}

func (a *APIMultiAuthConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case *evergreen.MultiAuthConfig:
		if v == nil {
			return nil
		}
		a.ReadWrite = v.ReadWrite
		a.ReadOnly = v.ReadOnly
	default:
		return errors.Errorf("programmatic error: expected multi-auth config but got type %T", h)
	}
	return nil
}

func (a *APIMultiAuthConfig) ToService() (any, error) {
	if a == nil {
		return nil, nil
	}
	return &evergreen.MultiAuthConfig{
		ReadWrite: a.ReadWrite,
		ReadOnly:  a.ReadOnly,
	}, nil
}

type APIKanopyAuthConfig struct {
	HeaderName *string `json:"header_name"`
	Issuer     *string `json:"issuer"`
	KeysetURL  *string `json:"keyset_url"`
}

func (a *APIKanopyAuthConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case *evergreen.KanopyAuthConfig:
		if v == nil {
			return nil
		}
		a.HeaderName = utility.ToStringPtr(v.HeaderName)
		a.Issuer = utility.ToStringPtr(v.Issuer)
		a.KeysetURL = utility.ToStringPtr(v.KeysetURL)
	default:
		return errors.Errorf("programmatic error: expected Kanopy auth config but got type %T", h)
	}
	return nil
}

func (a *APIKanopyAuthConfig) ToService() (any, error) {
	if a == nil {
		return nil, nil
	}
	return &evergreen.KanopyAuthConfig{
		HeaderName: utility.FromStringPtr(a.HeaderName),
		Issuer:     utility.FromStringPtr(a.Issuer),
		KeysetURL:  utility.FromStringPtr(a.KeysetURL),
	}, nil
}

type APIOAuthConfig struct {
	Issuer      *string `json:"issuer"`
	ClientID    *string `json:"client_id"`
	ConnectorID *string `json:"connector_id"`
}

func (a *APIOAuthConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case *evergreen.OAuthConfig:
		if v == nil {
			return nil
		}
		a.Issuer = utility.ToStringPtr(v.Issuer)
		a.ClientID = utility.ToStringPtr(v.ClientID)
		a.ConnectorID = utility.ToStringPtr(v.ConnectorID)
	default:
		return errors.Errorf("programmatic error: expected OAuth auth config but got type %T", h)
	}
	return nil
}

func (a *APIOAuthConfig) ToService() (any, error) {
	if a == nil {
		return nil, nil
	}
	return &evergreen.OAuthConfig{
		Issuer:      utility.FromStringPtr(a.Issuer),
		ClientID:    utility.FromStringPtr(a.ClientID),
		ConnectorID: utility.FromStringPtr(a.ConnectorID),
	}, nil
}

// APIBanner is a public structure representing the banner part of the admin settings
type APIBanner struct {
	Text  *string `json:"banner"`
	Theme *string `json:"theme"`
}

// APIUiV2URL is a public structure representing the new UI url (e.g. Spruce)
type APIUiV2URL struct {
	UIv2Url *string `json:"uiv2_url"`
}

type APIHostInitConfig struct {
	HostThrottle         int `json:"host_throttle"`
	ProvisioningThrottle int `json:"provisioning_throttle"`
	CloudStatusBatchSize int `json:"cloud_batch_size"`
	MaxTotalDynamicHosts int `json:"max_total_dynamic_hosts"`
}

func (a *APIHostInitConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.HostInitConfig:
		a.HostThrottle = v.HostThrottle
		a.ProvisioningThrottle = v.ProvisioningThrottle
		a.CloudStatusBatchSize = v.CloudStatusBatchSize
		a.MaxTotalDynamicHosts = v.MaxTotalDynamicHosts
	default:
		return errors.Errorf("programmatic error: expected host init config but got type %T", h)
	}
	return nil
}

func (a *APIHostInitConfig) ToService() (any, error) {
	return evergreen.HostInitConfig{
		HostThrottle:         a.HostThrottle,
		ProvisioningThrottle: a.ProvisioningThrottle,
		CloudStatusBatchSize: a.CloudStatusBatchSize,
		MaxTotalDynamicHosts: a.MaxTotalDynamicHosts,
	}, nil
}

type APIJiraConfig struct {
	Host                *string `json:"host"`
	DefaultProject      *string `json:"default_project"`
	Email               *string `json:"email"`
	PersonalAccessToken *string `json:"personal_access_token"`
}

func (a *APIJiraConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.JiraConfig:
		a.Host = utility.ToStringPtr(v.Host)
		a.Email = utility.ToStringPtr(v.Email)
		a.PersonalAccessToken = utility.ToStringPtr(v.PersonalAccessToken)
	default:
		return errors.Errorf("programmatic error: expected Jira config but got type %T", h)
	}
	return nil
}

func (a *APIJiraConfig) ToService() (any, error) {
	c := evergreen.JiraConfig{
		Host:                utility.FromStringPtr(a.Host),
		Email:               utility.FromStringPtr(a.Email),
		PersonalAccessToken: utility.FromStringPtr(a.PersonalAccessToken),
	}
	return c, nil
}

type APILoggerConfig struct {
	Buffer         *APILogBuffering `json:"buffer"`
	DefaultLevel   *string          `json:"default_level"`
	ThresholdLevel *string          `json:"threshold_level"`
	LogkeeperURL   *string          `json:"logkeeper_url"`
	RedactKeys     []*string        `json:"redact_keys"`
}

func (a *APILoggerConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.LoggerConfig:
		a.DefaultLevel = utility.ToStringPtr(v.DefaultLevel)
		a.ThresholdLevel = utility.ToStringPtr(v.ThresholdLevel)
		a.LogkeeperURL = utility.ToStringPtr(v.LogkeeperURL)
		a.RedactKeys = utility.ToStringPtrSlice(v.RedactKeys)
		a.Buffer = &APILogBuffering{}
		if err := a.Buffer.BuildFromService(v.Buffer); err != nil {
			return err
		}
	default:
		return errors.Errorf("programmatic error: expected task logging config but got type %T", h)
	}
	return nil
}

func (a *APILoggerConfig) ToService() (any, error) {
	config := evergreen.LoggerConfig{
		DefaultLevel:   utility.FromStringPtr(a.DefaultLevel),
		ThresholdLevel: utility.FromStringPtr(a.ThresholdLevel),
		LogkeeperURL:   utility.FromStringPtr(a.LogkeeperURL),
		RedactKeys:     utility.FromStringPtrSlice(a.RedactKeys),
	}
	i, err := a.Buffer.ToService()
	if err != nil {
		return nil, err
	}
	buffer := i.(evergreen.LogBuffering)
	config.Buffer = buffer
	return config, nil
}

type APILogBuffering struct {
	UseAsync             bool `json:"use_async"`
	DurationSeconds      int  `json:"duration_seconds"`
	Count                int  `json:"count"`
	IncomingBufferFactor int  `json:"incoming_buffer_factor"`
}

func (a *APILogBuffering) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.LogBuffering:
		a.UseAsync = v.UseAsync
		a.DurationSeconds = v.DurationSeconds
		a.Count = v.Count
		a.IncomingBufferFactor = v.IncomingBufferFactor
	default:
		return errors.Errorf("programmatic error: expected logging buffer config but got type %T", h)
	}
	return nil
}

func (a *APILogBuffering) ToService() (any, error) {
	return evergreen.LogBuffering{
		UseAsync:             a.UseAsync,
		DurationSeconds:      a.DurationSeconds,
		Count:                a.Count,
		IncomingBufferFactor: a.IncomingBufferFactor,
	}, nil
}

type APINotifyConfig struct {
	BufferTargetPerInterval int          `json:"buffer_target_per_interval"`
	BufferIntervalSeconds   int          `json:"buffer_interval_seconds"`
	SES                     APISESConfig `json:"ses"`
}

func (a *APINotifyConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.NotifyConfig:
		a.SES = APISESConfig{}
		if err := a.SES.BuildFromService(v.SES); err != nil {
			return err
		}
		a.BufferTargetPerInterval = v.BufferTargetPerInterval
		a.BufferIntervalSeconds = v.BufferIntervalSeconds
	default:
		return errors.Errorf("programmatic error: expected notify config but got type %T", h)
	}
	return nil
}

func (a *APINotifyConfig) ToService() (any, error) {
	ses, err := a.SES.ToService()
	if err != nil {
		return nil, err
	}

	return evergreen.NotifyConfig{
		BufferTargetPerInterval: a.BufferTargetPerInterval,
		BufferIntervalSeconds:   a.BufferIntervalSeconds,
		SES:                     ses.(evergreen.SESConfig),
	}, nil
}

type APIOverridesConfig struct {
	Overrides []APIOverride `json:"overrides"`
}

func (a *APIOverridesConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.OverridesConfig:
		var overrides []APIOverride
		for _, override := range v.Overrides {
			var apiOverride APIOverride
			if err := apiOverride.BuildFromService(override); err != nil {
				return errors.Wrap(err, "building APIOverride from service")
			}
			overrides = append(overrides, apiOverride)
		}
		a.Overrides = overrides
	default:
		return errors.Errorf("programmatic error: expected overrides config but got type %T", h)
	}
	return nil
}

func (a *APIOverridesConfig) ToService() (any, error) {
	var overrides []evergreen.Override
	for _, apiOverride := range a.Overrides {
		overrideInterface, err := apiOverride.ToService()
		if err != nil {
			return nil, errors.Wrap(err, "converting APIOverride to service")
		}
		override, ok := overrideInterface.(evergreen.Override)
		if !ok {
			return nil, errors.Errorf("programmatic error: expected override but got type %T", overrideInterface)
		}
		overrides = append(overrides, override)
	}
	return evergreen.OverridesConfig{
		Overrides: overrides,
	}, nil
}

type APIOverride struct {
	SectionID *string `bson:"section_id" json:"section_id"`
	Field     *string `bson:"field" json:"field"`
	Value     any     `bson:"value" json:"value"`
}

// MarshalJSON is a custom JSON marshaler function to satisfy the [json.Marshaler] interface.
// This is necessary because the regular JSON marshaler doesn't account for all the bson data types.
func (a APIOverride) MarshalJSON() ([]byte, error) {
	return bson.MarshalExtJSON(a, false, false)
}

// UnmarshalJSON is a custom JSON unmarshaler function to satisfy the [json.Unmarshaler] interface.
// This is necessary because the regular JSON unmarshaler doesn't account for all the bson data types.
func (a *APIOverride) UnmarshalJSON(data []byte) error {
	return bson.UnmarshalExtJSON(data, false, a)
}

func (a *APIOverride) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.Override:
		a.SectionID = utility.ToStringPtr(v.SectionID)
		a.Field = utility.ToStringPtr(v.Field)
		a.Value = v.Value
	default:
		return errors.Errorf("programmatic error: expected override but got type %T", h)
	}
	return nil
}

func (a *APIOverride) ToService() (any, error) {
	override := evergreen.Override{
		SectionID: utility.FromStringPtr(a.SectionID),
		Field:     utility.FromStringPtr(a.Field),
		Value:     a.Value,
	}
	return override, nil
}

type APIParameterStoreConfig struct {
	Prefix *string `json:"prefix"`
}

func (a *APIParameterStoreConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.ParameterStoreConfig:
		a.Prefix = utility.ToStringPtr(v.Prefix)
	default:
		return errors.Errorf("programmatic error: expected Parameter Store config but got type %T", h)
	}
	return nil
}

func (a *APIParameterStoreConfig) ToService() (any, error) {
	return evergreen.ParameterStoreConfig{
		Prefix: utility.FromStringPtr(a.Prefix),
	}, nil
}

type APIOwnerRepo struct {
	Owner *string `json:"owner"`
	Repo  *string `json:"repo"`
}

func (a *APIOwnerRepo) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.OwnerRepo:
		a.Owner = utility.ToStringPtr(v.Owner)
		a.Repo = utility.ToStringPtr(v.Repo)
	default:
		return errors.Errorf("programmatic error: expected owner and repo config but got type %T", h)
	}
	return nil
}

func (a *APIOwnerRepo) ToService() (any, error) {
	res := evergreen.OwnerRepo{}
	res.Owner = utility.FromStringPtr(a.Owner)
	res.Repo = utility.FromStringPtr(a.Repo)
	return res, nil
}

type APIProjectCreationConfig struct {
	TotalProjectLimit int            `json:"total_project_limit"`
	RepoProjectLimit  int            `json:"repo_project_limit"`
	RepoExceptions    []APIOwnerRepo `json:"repo_exceptions"`
	JiraProject       string         `json:"jira_project"`
}

func (a *APIProjectCreationConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.ProjectCreationConfig:
		for _, ownerRepo := range v.RepoExceptions {
			apiOwnerRepo := APIOwnerRepo{}
			if err := apiOwnerRepo.BuildFromService(ownerRepo); err != nil {
				return err
			}
			a.RepoExceptions = append(a.RepoExceptions, apiOwnerRepo)
		}
		a.TotalProjectLimit = v.TotalProjectLimit
		a.RepoProjectLimit = v.RepoProjectLimit
		a.JiraProject = v.JiraProject
	default:
		return errors.Errorf("programmatic error: expected Project Creation config but got type %T", h)
	}

	return nil
}

func (a *APIProjectCreationConfig) ToService() (any, error) {
	if a == nil {
		return nil, nil
	}

	config := evergreen.ProjectCreationConfig{
		TotalProjectLimit: a.TotalProjectLimit,
		RepoProjectLimit:  a.RepoProjectLimit,
		JiraProject:       a.JiraProject,
	}

	for _, r := range a.RepoExceptions {
		i, err := r.ToService()
		if err != nil {
			return nil, err
		}
		ownerRepo, ok := i.(evergreen.OwnerRepo)
		if !ok {
			return nil, errors.Errorf("programmatic error: expected owner and repo but got type %T", i)
		}
		config.RepoExceptions = append(config.RepoExceptions, ownerRepo)
	}

	return config, nil
}

type APICloudProviders struct {
	AWS    *APIAWSConfig    `json:"aws"`
	Docker *APIDockerConfig `json:"docker"`
}

func (a *APICloudProviders) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.CloudProviders:
		a.AWS = &APIAWSConfig{}
		a.Docker = &APIDockerConfig{}
		if err := a.AWS.BuildFromService(v.AWS); err != nil {
			return err
		}
		if err := a.Docker.BuildFromService(v.Docker); err != nil {
			return err
		}
	default:
		return errors.Errorf("programmatic error: expected cloud provider config but got type %T", h)
	}
	return nil
}

func (a *APICloudProviders) ToService() (any, error) {
	aws, err := a.AWS.ToService()
	if err != nil {
		return nil, err
	}
	docker, err := a.Docker.ToService()
	if err != nil {
		return nil, err
	}

	config := evergreen.CloudProviders{}

	if aws != nil {
		config.AWS = aws.(evergreen.AWSConfig)
	}

	if docker != nil {
		config.Docker = docker.(evergreen.DockerConfig)
	}

	return config, nil
}

type APIContainerPoolsConfig struct {
	Pools []APIContainerPool `json:"pools"`
}

func (a *APIContainerPoolsConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.ContainerPoolsConfig:
		for _, pool := range v.Pools {
			apiPool := APIContainerPool{}
			if err := apiPool.BuildFromService(pool); err != nil {
				return err
			}
			a.Pools = append(a.Pools, apiPool)
		}
	default:
		return errors.Errorf("programmatic error: expected container pools config but got type %T", h)
	}
	return nil
}

func (a *APIContainerPoolsConfig) ToService() (any, error) {
	if a == nil {
		return nil, nil
	}
	config := evergreen.ContainerPoolsConfig{}
	for _, p := range a.Pools {
		i, err := p.ToService()
		if err != nil {
			return nil, err
		}
		pool := i.(evergreen.ContainerPool)
		config.Pools = append(config.Pools, pool)
	}
	return config, nil
}

type APIContainerPool struct {
	Distro        *string `json:"distro"`
	Id            *string `json:"id"`
	MaxContainers int     `json:"max_containers"`
	Port          uint16  `json:"port"`
}

func (a *APIContainerPool) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.ContainerPool:
		a.Distro = utility.ToStringPtr(v.Distro)
		a.Id = utility.ToStringPtr(v.Id)
		a.MaxContainers = v.MaxContainers
		a.Port = v.Port
	default:
		return errors.Errorf("programmatic error: expected container pool config but got type %T", h)
	}
	return nil
}

func (a *APIContainerPool) ToService() (any, error) {
	return evergreen.ContainerPool{
		Distro:        utility.FromStringPtr(a.Distro),
		Id:            utility.FromStringPtr(a.Id),
		MaxContainers: a.MaxContainers,
		Port:          a.Port,
	}, nil
}

type APIEC2Key struct {
	Name   *string `json:"name"`
	Region *string `json:"region"`
	Key    *string `json:"key"`
	Secret *string `json:"secret"`
}

func (a *APIEC2Key) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.EC2Key:
		a.Name = utility.ToStringPtr(v.Name)
		a.Key = utility.ToStringPtr(v.Key)
		a.Secret = utility.ToStringPtr(v.Secret)
	default:
		return errors.Errorf("programmatic error: expected EC2 key config but got type %T", h)
	}
	return nil
}

func (a *APIEC2Key) ToService() (any, error) {
	res := evergreen.EC2Key{}
	res.Name = utility.FromStringPtr(a.Name)
	res.Key = utility.FromStringPtr(a.Key)
	res.Secret = utility.FromStringPtr(a.Secret)
	return res, nil
}

type APISubnet struct {
	AZ       *string `json:"az"`
	SubnetID *string `json:"subnet_id"`
}

func (a *APISubnet) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.Subnet:
		a.AZ = utility.ToStringPtr(v.AZ)
		a.SubnetID = utility.ToStringPtr(v.SubnetID)
	default:
		return errors.Errorf("programmatic error: expected subnet config but got type %T", h)
	}
	return nil
}

func (a *APISubnet) ToService() (any, error) {
	res := evergreen.Subnet{}
	res.AZ = utility.FromStringPtr(a.AZ)
	res.SubnetID = utility.FromStringPtr(a.SubnetID)
	return res, nil
}

type APIAWSConfig struct {
	EC2Keys                []APIEC2Key                `json:"ec2_keys"`
	Subnets                []APISubnet                `json:"subnets"`
	ParserProject          *APIParserProjectS3Config  `json:"parser_project"`
	PersistentDNS          *APIPersistentDNSConfig    `json:"persistent_dns"`
	DefaultSecurityGroup   *string                    `json:"default_security_group"`
	AllowedInstanceTypes   []*string                  `json:"allowed_instance_types"`
	AlertableInstanceTypes []*string                  `json:"alertable_instance_types"`
	AllowedRegions         []*string                  `json:"allowed_regions"`
	MaxVolumeSizePerUser   *int                       `json:"max_volume_size"`
	AccountRoles           []APIAWSAccountRoleMapping `json:"account_roles"`
	IPAMPoolID             *string                    `json:"ipam_pool_id"`
	ElasticIPUsageRate     *float64                   `json:"elastic_ip_usage_rate"`
}

func (a *APIAWSConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.AWSConfig:
		for _, key := range v.EC2Keys {
			apiKey := APIEC2Key{}
			if err := apiKey.BuildFromService(key); err != nil {
				return err
			}
			a.EC2Keys = append(a.EC2Keys, apiKey)
		}

		for _, subnet := range v.Subnets {
			apiSubnet := APISubnet{}
			if err := apiSubnet.BuildFromService(subnet); err != nil {
				return err
			}
			a.Subnets = append(a.Subnets, apiSubnet)
		}

		parserProject := &APIParserProjectS3Config{}
		if err := parserProject.BuildFromService(v.ParserProject); err != nil {
			return errors.Wrap(err, "converting parser project S3 config to API model")
		}
		a.ParserProject = parserProject

		persistentDNS := &APIPersistentDNSConfig{}
		if err := persistentDNS.BuildFromService(v.PersistentDNS); err != nil {
			return errors.Wrap(err, "converting persistent DNS config to API model")
		}
		a.PersistentDNS = persistentDNS

		a.DefaultSecurityGroup = utility.ToStringPtr(v.DefaultSecurityGroup)
		a.MaxVolumeSizePerUser = &v.MaxVolumeSizePerUser
		a.AllowedInstanceTypes = utility.ToStringPtrSlice(v.AllowedInstanceTypes)
		a.AlertableInstanceTypes = utility.ToStringPtrSlice(v.AlertableInstanceTypes)
		a.AllowedRegions = utility.ToStringPtrSlice(v.AllowedRegions)

		var roleMappings []APIAWSAccountRoleMapping
		for _, m := range v.AccountRoles {
			var api APIAWSAccountRoleMapping
			api.BuildFromService(m)
			roleMappings = append(roleMappings, api)
		}
		a.AccountRoles = roleMappings
		a.IPAMPoolID = utility.ToStringPtr(v.IPAMPoolID)
		a.ElasticIPUsageRate = utility.ToFloat64Ptr(v.ElasticIPUsageRate)
	default:
		return errors.Errorf("programmatic error: expected AWS config but got type %T", h)
	}
	return nil
}

func (a *APIAWSConfig) ToService() (any, error) {
	if a == nil {
		return nil, nil
	}
	config := evergreen.AWSConfig{
		DefaultSecurityGroup: utility.FromStringPtr(a.DefaultSecurityGroup),
		MaxVolumeSizePerUser: evergreen.DefaultMaxVolumeSizePerUser,
	}

	var i any
	var err error
	var ok bool

	i, err = a.ParserProject.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting parser project S3 credentials to service model")
	}
	var parserProject evergreen.ParserProjectS3Config
	if i != nil {
		parserProject, ok = i.(evergreen.ParserProjectS3Config)
		if !ok {
			return nil, errors.Errorf("programmatic error: expected parser project S3 config but got type %T", i)
		}
	}
	config.ParserProject = parserProject

	i, err = a.PersistentDNS.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting persistent DNS config to service model")
	}
	var persistentDNS evergreen.PersistentDNSConfig
	if i != nil {
		persistentDNS, ok = i.(evergreen.PersistentDNSConfig)
		if !ok {
			return nil, errors.Errorf("programmatic error: expected parser project S3 config but got type %T", i)
		}
	}
	config.PersistentDNS = persistentDNS

	if a.MaxVolumeSizePerUser != nil {
		config.MaxVolumeSizePerUser = *a.MaxVolumeSizePerUser
	}

	for _, k := range a.EC2Keys {
		i, err := k.ToService()
		if err != nil {
			return nil, err
		}
		key, ok := i.(evergreen.EC2Key)
		if !ok {
			return nil, errors.Errorf("programmatic error: expected EC2 key but got type %T", i)
		}
		config.EC2Keys = append(config.EC2Keys, key)
	}

	for _, s := range a.Subnets {
		i, err := s.ToService()
		if err != nil {
			return nil, err
		}
		subnet, ok := i.(evergreen.Subnet)
		if !ok {
			return nil, errors.Errorf("programmatic error: expected EC2 subnet but got type %T", i)
		}
		config.Subnets = append(config.Subnets, subnet)
	}

	config.AllowedInstanceTypes = utility.FromStringPtrSlice(a.AllowedInstanceTypes)
	config.AlertableInstanceTypes = utility.FromStringPtrSlice(a.AlertableInstanceTypes)
	config.AllowedRegions = utility.FromStringPtrSlice(a.AllowedRegions)

	var roleMappings []evergreen.AWSAccountRoleMapping
	for _, m := range a.AccountRoles {
		roleMappings = append(roleMappings, m.ToService())
	}
	config.AccountRoles = roleMappings

	config.IPAMPoolID = utility.FromStringPtr(a.IPAMPoolID)
	config.ElasticIPUsageRate = utility.FromFloat64Ptr(a.ElasticIPUsageRate)

	return config, nil
}

type APIS3Credentials struct {
	Key    *string `json:"key"`
	Secret *string `json:"secret"`
	Bucket *string `json:"bucket"`
}

func (a *APIS3Credentials) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.S3Credentials:
		a.Key = utility.ToStringPtr(v.Key)
		a.Secret = utility.ToStringPtr(v.Secret)
		a.Bucket = utility.ToStringPtr(v.Bucket)
		return nil
	default:
		return errors.Errorf("programmatic error: expected S3 credentials but got type %T", h)
	}
}

func (a *APIS3Credentials) ToService() (any, error) {
	if a == nil {
		return nil, nil
	}
	return evergreen.S3Credentials{
		Key:    utility.FromStringPtr(a.Key),
		Secret: utility.FromStringPtr(a.Secret),
		Bucket: utility.FromStringPtr(a.Bucket),
	}, nil
}

// APIParserProjectS3Config represents configuration options for storing and
// accessing parser projects in S3.
type APIParserProjectS3Config struct {
	APIS3Credentials
	Prefix              *string `json:"prefix"`
	GeneratedJSONPrefix *string `json:"generated_json_prefix"`
}

func (a *APIParserProjectS3Config) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.ParserProjectS3Config:
		a.Key = utility.ToStringPtr(v.Key)
		a.Secret = utility.ToStringPtr(v.Secret)
		a.Bucket = utility.ToStringPtr(v.Bucket)
		a.Prefix = utility.ToStringPtr(v.Prefix)
		a.GeneratedJSONPrefix = utility.ToStringPtr(v.GeneratedJSONPrefix)
		return nil
	default:
		return errors.Errorf("programmatic error: expected parser project S3 config but got type %T", h)
	}
}

func (a *APIParserProjectS3Config) ToService() (any, error) {
	if a == nil {
		return nil, nil
	}
	return evergreen.ParserProjectS3Config{
		S3Credentials: evergreen.S3Credentials{
			Key:    utility.FromStringPtr(a.Key),
			Secret: utility.FromStringPtr(a.Secret),
			Bucket: utility.FromStringPtr(a.Bucket),
		},
		Prefix:              utility.FromStringPtr(a.Prefix),
		GeneratedJSONPrefix: utility.FromStringPtr(a.GeneratedJSONPrefix),
	}, nil
}

// APIPersistentDNSConfig represents configuration options for supporting
// persistent DNS names for hosts.
type APIPersistentDNSConfig struct {
	HostedZoneID *string `json:"hosted_zone_id"`
	Domain       *string `json:"domain"`
}

func (a *APIPersistentDNSConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.PersistentDNSConfig:
		a.HostedZoneID = utility.ToStringPtr(v.HostedZoneID)
		a.Domain = utility.ToStringPtr(v.Domain)
		return nil
	default:
		return errors.Errorf("programmatic error: expected parser project S3 config but got type %T", h)
	}
}

func (a *APIPersistentDNSConfig) ToService() (any, error) {
	if a == nil {
		return nil, nil
	}
	return evergreen.PersistentDNSConfig{
		HostedZoneID: utility.FromStringPtr(a.HostedZoneID),
		Domain:       utility.FromStringPtr(a.Domain),
	}, nil
}

// APIAWSVPCConfig represents configuration options for tasks in ECS using
// AWSVPC networking.
type APIAWSVPCConfig struct {
	Subnets        []string `json:"subnets,omitempty"`
	SecurityGroups []string `json:"security_groups,omitempty"`
}

func (a *APIAWSVPCConfig) BuildFromService(conf evergreen.AWSVPCConfig) {
	a.Subnets = conf.Subnets
	a.SecurityGroups = conf.SecurityGroups
}

func (a *APIAWSVPCConfig) ToService() evergreen.AWSVPCConfig {
	if a == nil {
		return evergreen.AWSVPCConfig{}
	}
	return evergreen.AWSVPCConfig{
		Subnets:        a.Subnets,
		SecurityGroups: a.SecurityGroups,
	}
}

type APIDockerConfig struct {
	APIVersion *string `json:"api_version"`
}

func (a *APIDockerConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.DockerConfig:
		a.APIVersion = utility.ToStringPtr(v.APIVersion)
	default:
		return errors.Errorf("programmatic error: expected Docker config but got type %T", h)
	}
	return nil
}

func (a *APIDockerConfig) ToService() (any, error) {
	if a == nil {
		return nil, nil
	}
	return evergreen.DockerConfig{
		APIVersion: utility.FromStringPtr(a.APIVersion),
	}, nil
}

type APIRepoTrackerConfig struct {
	NumNewRepoRevisionsToFetch int `json:"revs_to_fetch"`
	MaxRepoRevisionsToSearch   int `json:"max_revs_to_search"`
	MaxConcurrentRequests      int `json:"max_con_requests"`
}

func (a *APIRepoTrackerConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.RepoTrackerConfig:
		a.NumNewRepoRevisionsToFetch = v.NumNewRepoRevisionsToFetch
		a.MaxConcurrentRequests = v.MaxConcurrentRequests
		a.MaxRepoRevisionsToSearch = v.MaxRepoRevisionsToSearch
	default:
		return errors.Errorf("programmatic error: expected repotracker config but got type %T", h)
	}
	return nil
}

func (a *APIRepoTrackerConfig) ToService() (any, error) {
	return evergreen.RepoTrackerConfig{
		NumNewRepoRevisionsToFetch: a.NumNewRepoRevisionsToFetch,
		MaxConcurrentRequests:      a.MaxConcurrentRequests,
		MaxRepoRevisionsToSearch:   a.MaxRepoRevisionsToSearch,
	}, nil
}

type APIReleaseModeConfig struct {
	DistroMaxHostsFactor      float64 `json:"distro_max_hosts_factor"`
	TargetTimeSecondsOverride int     `json:"target_time_seconds_override"`
	IdleTimeSecondsOverride   int     `json:"idle_time_seconds_override"`
}

func (a *APIReleaseModeConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.ReleaseModeConfig:
		a.DistroMaxHostsFactor = v.DistroMaxHostsFactor
		a.TargetTimeSecondsOverride = v.TargetTimeSecondsOverride
		a.IdleTimeSecondsOverride = v.IdleTimeSecondsOverride
	default:
		return errors.Errorf("programmatic error: expected ReleaseModeConfig but got type %T", h)
	}
	return nil
}

func (a *APIReleaseModeConfig) ToService() (any, error) {
	return evergreen.ReleaseModeConfig{
		DistroMaxHostsFactor:      a.DistroMaxHostsFactor,
		TargetTimeSecondsOverride: a.TargetTimeSecondsOverride,
		IdleTimeSecondsOverride:   a.IdleTimeSecondsOverride,
	}, nil
}

type APISchedulerConfig struct {
	TaskFinder                    *string `json:"task_finder"`
	HostAllocator                 *string `json:"host_allocator"`
	HostAllocatorRoundingRule     *string `json:"host_allocator_rounding_rule"`
	HostAllocatorFeedbackRule     *string `json:"host_allocator_feedback_rule"`
	HostsOverallocatedRule        *string `json:"hosts_overallocated_rule"`
	FutureHostFraction            float64 `json:"free_host_fraction"`
	CacheDurationSeconds          int     `json:"cache_duration_seconds"`
	TargetTimeSeconds             int     `json:"target_time_seconds"`
	AcceptableHostIdleTimeSeconds int     `json:"acceptable_host_idle_time_seconds"`
	GroupVersions                 bool    `json:"group_versions"`
	PatchFactor                   int64   `json:"patch_factor"`
	PatchTimeInQueueFactor        int64   `json:"patch_time_in_queue_factor"`
	CommitQueueFactor             int64   `json:"commit_queue_factor"`
	MainlineTimeInQueueFactor     int64   `json:"mainline_time_in_queue_factor"`
	ExpectedRuntimeFactor         int64   `json:"expected_runtime_factor"`
	GenerateTaskFactor            int64   `json:"generate_task_factor"`
	NumDependentsFactor           float64 `json:"num_dependents_factor"`
	StepbackTaskFactor            int64   `json:"stepback_task_factor"`
}

func (a *APISchedulerConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.SchedulerConfig:
		a.TaskFinder = utility.ToStringPtr(v.TaskFinder)
		a.HostAllocator = utility.ToStringPtr(v.HostAllocator)
		a.HostAllocatorRoundingRule = utility.ToStringPtr(v.HostAllocatorRoundingRule)
		a.HostAllocatorFeedbackRule = utility.ToStringPtr(v.HostAllocatorFeedbackRule)
		a.HostsOverallocatedRule = utility.ToStringPtr(v.HostsOverallocatedRule)
		a.FutureHostFraction = v.FutureHostFraction
		a.CacheDurationSeconds = v.CacheDurationSeconds
		a.TargetTimeSeconds = v.TargetTimeSeconds
		a.AcceptableHostIdleTimeSeconds = v.AcceptableHostIdleTimeSeconds
		a.GroupVersions = v.GroupVersions
		a.PatchFactor = v.PatchFactor
		a.PatchTimeInQueueFactor = v.PatchTimeInQueueFactor
		a.CommitQueueFactor = v.CommitQueueFactor
		a.MainlineTimeInQueueFactor = v.MainlineTimeInQueueFactor
		a.ExpectedRuntimeFactor = v.ExpectedRuntimeFactor
		a.GenerateTaskFactor = v.GenerateTaskFactor
		a.NumDependentsFactor = v.NumDependentsFactor
		a.StepbackTaskFactor = v.StepbackTaskFactor
	default:
		return errors.Errorf("programmatic error: expected host scheduler config but got type %T", h)
	}
	return nil
}

func (a *APISchedulerConfig) ToService() (any, error) {
	return evergreen.SchedulerConfig{
		TaskFinder:                    utility.FromStringPtr(a.TaskFinder),
		HostAllocator:                 utility.FromStringPtr(a.HostAllocator),
		HostAllocatorRoundingRule:     utility.FromStringPtr(a.HostAllocatorRoundingRule),
		HostAllocatorFeedbackRule:     utility.FromStringPtr(a.HostAllocatorFeedbackRule),
		HostsOverallocatedRule:        utility.FromStringPtr(a.HostsOverallocatedRule),
		FutureHostFraction:            a.FutureHostFraction,
		CacheDurationSeconds:          a.CacheDurationSeconds,
		TargetTimeSeconds:             a.TargetTimeSeconds,
		AcceptableHostIdleTimeSeconds: a.AcceptableHostIdleTimeSeconds,
		GroupVersions:                 a.GroupVersions,
		PatchFactor:                   a.PatchFactor,
		ExpectedRuntimeFactor:         a.ExpectedRuntimeFactor,
		PatchTimeInQueueFactor:        a.PatchTimeInQueueFactor,
		CommitQueueFactor:             a.CommitQueueFactor,
		MainlineTimeInQueueFactor:     a.MainlineTimeInQueueFactor,
		GenerateTaskFactor:            a.GenerateTaskFactor,
		NumDependentsFactor:           a.NumDependentsFactor,
		StepbackTaskFactor:            a.StepbackTaskFactor,
	}, nil
}

// APIServiceFlags is a public structure representing the admin service flags
type APIServiceFlags struct {
	TaskDispatchDisabled        bool `json:"task_dispatch_disabled"`
	HostInitDisabled            bool `json:"host_init_disabled"`
	LargeParserProjectsDisabled bool `json:"large_parser_projects_disabled"`
	MonitorDisabled             bool `json:"monitor_disabled"`
	AlertsDisabled              bool `json:"alerts_disabled"`
	AgentStartDisabled          bool `json:"agent_start_disabled"`
	RepotrackerDisabled         bool `json:"repotracker_disabled"`
	SchedulerDisabled           bool `json:"scheduler_disabled"`
	CheckBlockedTasksDisabled   bool `json:"check_blocked_tasks_disabled"`
	GithubPRTestingDisabled     bool `json:"github_pr_testing_disabled"`
	CLIUpdatesDisabled          bool `json:"cli_updates_disabled"`
	BackgroundStatsDisabled     bool `json:"background_stats_disabled"`
	TaskLoggingDisabled         bool `json:"task_logging_disabled"`
	CacheStatsJobDisabled       bool `json:"cache_stats_job_disabled"`
	CacheStatsEndpointDisabled  bool `json:"cache_stats_endpoint_disabled"`
	TaskReliabilityDisabled     bool `json:"task_reliability_disabled"`
	HostAllocatorDisabled       bool `json:"host_allocator_disabled"`
	BackgroundReauthDisabled    bool `json:"background_reauth_disabled"`
	CloudCleanupDisabled        bool `json:"cloud_cleanup_disabled"`
	SleepScheduleDisabled       bool `json:"sleep_schedule_disabled"`
	StaticAPIKeysDisabled       bool `json:"static_api_keys_disabled"`
	// JWTTokenForCLIDisabled disables the use of OAuth tokens for the CLI.
	JWTTokenForCLIDisabled             bool `json:"jwt_token_for_cli_disabled"`
	SystemFailedTaskRestartDisabled    bool `json:"system_failed_task_restart_disabled"`
	DegradedModeDisabled               bool `json:"cpu_degraded_mode_disabled"`
	ElasticIPsDisabled                 bool `json:"elastic_ips_disabled"`
	ReleaseModeDisabled                bool `json:"release_mode_disabled"`
	LegacyUIAdminPageDisabled          bool `json:"legacy_ui_admin_page_disabled"`
	DebugSpawnHostDisabled             bool `json:"debug_spawn_host_disabled"`
	S3LifecycleSyncDisabled            bool `json:"s3_lifecycle_sync_disabled"`
	UseMergeQueuePathFilteringDisabled bool `json:"use_merge_queue_path_filtering_disabled"`
	PSLoggingDisabled                  bool `json:"ps_logging_disabled"`

	// Notifications Flags
	EventProcessingDisabled      bool `json:"event_processing_disabled"`
	JIRANotificationsDisabled    bool `json:"jira_notifications_disabled"`
	SlackNotificationsDisabled   bool `json:"slack_notifications_disabled"`
	EmailNotificationsDisabled   bool `json:"email_notifications_disabled"`
	WebhookNotificationsDisabled bool `json:"webhook_notifications_disabled"`
	GithubStatusAPIDisabled      bool `json:"github_status_api_disabled"`
}

type APIProjectTasksPair struct {
	ProjectID    string   `json:"project_id"`
	AllowedTasks []string `json:"allowed_tasks"`
	AllowedBVs   []string `json:"allowed_bvs"`
}

func (a *APIProjectTasksPair) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.ProjectTasksPair:
		a.ProjectID = v.ProjectID
		a.AllowedTasks = v.AllowedTasks
		a.AllowedBVs = v.AllowedBVs
	default:
		return errors.Errorf("programmatic error: expected project tasks pair but got type %T", h)
	}
	return nil
}

func (a *APIProjectTasksPair) ToService() (any, error) {
	return evergreen.ProjectTasksPair{
		ProjectID:    a.ProjectID,
		AllowedTasks: a.AllowedTasks,
		AllowedBVs:   a.AllowedBVs,
	}, nil
}

type APISingleTaskDistroConfig struct {
	ProjectTasksPairs []APIProjectTasksPair `json:"project_tasks_pair"`
}

func (a *APISingleTaskDistroConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.SingleTaskDistroConfig:
		apiPairs := []APIProjectTasksPair{}
		for _, pair := range v.ProjectTasksPairs {
			apiPair := APIProjectTasksPair{}
			if err := apiPair.BuildFromService(pair); err != nil {
				return errors.Wrap(err, "converting project tasks pair to API model")
			}
			apiPairs = append(apiPairs, apiPair)
		}
		a.ProjectTasksPairs = apiPairs
	default:
		return errors.Errorf("programmatic error: expected single task distro config but got type %T", h)
	}
	return nil
}

func (a *APISingleTaskDistroConfig) ToService() (any, error) {
	pairs := []evergreen.ProjectTasksPair{}
	for _, pair := range a.ProjectTasksPairs {
		p, err := pair.ToService()
		if err != nil {
			return nil, errors.Wrap(err, "converting project tasks pair to service model")
		}
		pairs = append(pairs, p.(evergreen.ProjectTasksPair))
	}
	return evergreen.SingleTaskDistroConfig{
		ProjectTasksPairs: pairs,
	}, nil
}

type APISlackConfig struct {
	Options *APISlackOptions `json:"options"`
	Token   *string          `json:"token"`
	Level   *string          `json:"level"`
	Name    *string          `json:"name"`
}

func (a *APISlackConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.SlackConfig:
		a.Token = utility.ToStringPtr(v.Token)
		a.Level = utility.ToStringPtr(v.Level)
		a.Name = utility.ToStringPtr(v.Name)
		if v.Options != nil {
			a.Options = &APISlackOptions{}
			if err := a.Options.BuildFromService(*v.Options); err != nil { //nolint:govet
				return errors.Wrap(err, "converting Slack options to API model")
			}
		}
	default:
		return errors.Errorf("programmatic error: expected Slack config but got type %T", h)
	}
	return nil
}

func (a *APISlackConfig) ToService() (any, error) {
	i, err := a.Options.ToService()
	if err != nil {
		return nil, err
	}
	options := i.(send.SlackOptions) //nolint:govet
	return evergreen.SlackConfig{
		Token:   utility.FromStringPtr(a.Token),
		Level:   utility.FromStringPtr(a.Level),
		Name:    utility.FromStringPtr(a.Name),
		Options: &options,
	}, nil
}

type APISlackOptions struct {
	Channel       *string         `json:"channel"`
	Hostname      *string         `json:"hostname"`
	Name          *string         `json:"name"`
	Username      *string         `json:"username"`
	BasicMetadata bool            `json:"add_basic_metadata"`
	Fields        bool            `json:"use_fields"`
	AllFields     bool            `json:"all_fields"`
	FieldsSet     map[string]bool `json:"fields"`
}

func (a *APISlackOptions) BuildFromService(h any) error {
	switch v := h.(type) {
	case send.SlackOptions:
		a.Channel = utility.ToStringPtr(v.Channel)
		a.Hostname = utility.ToStringPtr(v.Hostname)
		a.Name = utility.ToStringPtr(v.Name)
		a.Username = utility.ToStringPtr(v.Username)
		a.BasicMetadata = v.BasicMetadata
		a.Fields = v.Fields
		a.AllFields = v.AllFields
		a.FieldsSet = v.FieldsSet
	default:
		return errors.Errorf("programmatic error: expected Slack options but got type %T", h)
	}
	return nil
}

func (a *APISlackOptions) ToService() (any, error) {
	if a == nil {
		return send.SlackOptions{}, nil
	}
	return send.SlackOptions{
		Channel:       utility.FromStringPtr(a.Channel),
		Hostname:      utility.FromStringPtr(a.Hostname),
		Name:          utility.FromStringPtr(a.Name),
		Username:      utility.FromStringPtr(a.Username),
		BasicMetadata: a.BasicMetadata,
		Fields:        a.Fields,
		AllFields:     a.AllFields,
		FieldsSet:     a.FieldsSet,
	}, nil
}

type APISleepScheduleConfig struct {
	PermanentlyExemptHosts []string `json:"permanently_exempt_hosts"`
}

func (a *APISleepScheduleConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.SleepScheduleConfig:
		a.PermanentlyExemptHosts = v.PermanentlyExemptHosts
	default:
		return errors.Errorf("programmatic error: expected sleep schedule config but got type %T", h)
	}
	return nil
}

func (a *APISleepScheduleConfig) ToService() (any, error) {
	if a == nil {
		return evergreen.SleepScheduleConfig{}, nil
	}
	return evergreen.SleepScheduleConfig{
		PermanentlyExemptHosts: a.PermanentlyExemptHosts,
	}, nil
}

type APISplunkConfig struct {
	SplunkConnectionInfo *APISplunkConnectionInfo `json:"splunk_connection_info"`
}

func (a *APISplunkConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.SplunkConfig:
		a.SplunkConnectionInfo = &APISplunkConnectionInfo{}
		a.SplunkConnectionInfo.BuildFromService(v.SplunkConnectionInfo)
	default:
		return errors.Errorf("programmatic error: expected Splunk config but got type '%T'", h)
	}
	return nil
}

func (a *APISplunkConfig) ToService() (any, error) {
	c := evergreen.SplunkConfig{}
	if a.SplunkConnectionInfo != nil {
		c.SplunkConnectionInfo = a.SplunkConnectionInfo.ToService()
	}
	return c, nil
}

type APISplunkConnectionInfo struct {
	ServerURL *string `json:"url"`
	Token     *string `json:"token"`
	Channel   *string `json:"channel"`
}

func (a *APISplunkConnectionInfo) BuildFromService(s send.SplunkConnectionInfo) {
	a.ServerURL = utility.ToStringPtr(s.ServerURL)
	a.Token = utility.ToStringPtr(s.Token)
	a.Channel = utility.ToStringPtr(s.Channel)
}

func (a *APISplunkConnectionInfo) ToService() send.SplunkConnectionInfo {
	return send.SplunkConnectionInfo{
		ServerURL: utility.FromStringPtr(a.ServerURL),
		Token:     utility.FromStringPtr(a.Token),
		Channel:   utility.FromStringPtr(a.Channel),
	}
}

type APISSHConfig struct {
	TaskHostKey  APISSHKeyPair `json:"task_host_key"`
	SpawnHostKey APISSHKeyPair `json:"spawn_host_key"`
}

func (a *APISSHConfig) BuildFromService(h any) error {
	catcher := grip.NewBasicCatcher()
	switch v := h.(type) {
	case evergreen.SSHConfig:
		catcher.Wrap(a.TaskHostKey.BuildFromService(v.TaskHostKey), "building task host key from service")
		catcher.Wrap(a.SpawnHostKey.BuildFromService(v.SpawnHostKey), "building spawn host key from service")
	default:
		return errors.Errorf("programmatic error: expected SSH Config but got type %T", h)
	}
	return catcher.Resolve()
}

func (a *APISSHConfig) ToService() (any, error) {
	if a == nil {
		return evergreen.SSHConfig{}, nil
	}

	catcher := grip.NewBasicCatcher()
	taskHostIface, err := a.TaskHostKey.ToService()
	catcher.Wrap(err, "converting task host key to service")
	spawnHostIface, err := a.SpawnHostKey.ToService()
	catcher.Wrap(err, "converting spawn host key to service")
	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}
	return evergreen.SSHConfig{
		TaskHostKey:  taskHostIface.(evergreen.SSHKeyPair),
		SpawnHostKey: spawnHostIface.(evergreen.SSHKeyPair),
	}, nil
}

type APISSHKeyPair struct {
	Name      *string `json:"name"`
	SecretARN *string `json:"secret_arn"`
}

func (a *APISSHKeyPair) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.SSHKeyPair:
		a.Name = utility.ToStringPtr(v.Name)
		a.SecretARN = utility.ToStringPtr(v.SecretARN)
	default:
		return errors.Errorf("programmatic error: expected SSH Key Pair but got type %T", h)
	}
	return nil
}

func (a *APISSHKeyPair) ToService() (any, error) {
	if a == nil {
		return evergreen.SSHKeyPair{}, nil
	}
	return evergreen.SSHKeyPair{
		Name:      utility.FromStringPtr(a.Name),
		SecretARN: utility.FromStringPtr(a.SecretARN),
	}, nil
}

type APIUIConfig struct {
	Url                       *string         `json:"url"`
	UIv2Url                   *string         `json:"uiv2_url"`
	ParsleyUrl                *string         `json:"parsley_url"`
	HttpListenAddr            *string         `json:"http_listen_addr"`
	Secret                    *string         `json:"secret"`
	DefaultProject            *string         `json:"default_project"`
	CacheTemplates            bool            `json:"cache_templates"`
	CsrfKey                   *string         `json:"csrf_key"`
	CORSOrigins               []string        `json:"cors_origins"`
	FileStreamingContentTypes []string        `json:"file_streaming_content_types"`
	LoginDomain               *string         `json:"login_domain"`
	UserVoice                 *string         `json:"userVoice"`
	BetaFeatures              APIBetaFeatures `json:"beta_features"`
	StagingEnvironment        *string         `json:"staging_environment"`
}

func (a *APIUIConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.UIConfig:
		a.Url = utility.ToStringPtr(v.Url)
		a.UIv2Url = utility.ToStringPtr(v.UIv2Url)
		a.ParsleyUrl = utility.ToStringPtr(v.ParsleyUrl)
		a.HttpListenAddr = utility.ToStringPtr(v.HttpListenAddr)
		a.Secret = utility.ToStringPtr(v.Secret)
		a.DefaultProject = utility.ToStringPtr(v.DefaultProject)
		a.CacheTemplates = v.CacheTemplates
		a.CsrfKey = utility.ToStringPtr(v.CsrfKey)
		a.CORSOrigins = v.CORSOrigins
		a.LoginDomain = utility.ToStringPtr(v.LoginDomain)
		a.UserVoice = utility.ToStringPtr(v.UserVoice)
		a.FileStreamingContentTypes = v.FileStreamingContentTypes
		a.StagingEnvironment = utility.ToStringPtr(v.StagingEnvironment)

		betaFeatures := APIBetaFeatures{}
		betaFeatures.BuildFromService(v.BetaFeatures)
		a.BetaFeatures = betaFeatures
	default:
		return errors.Errorf("programmatic error: expected UI config but got type %T", h)
	}
	return nil
}

func (a *APIUIConfig) ToService() (any, error) {
	return evergreen.UIConfig{
		Url:                       utility.FromStringPtr(a.Url),
		UIv2Url:                   utility.FromStringPtr(a.UIv2Url),
		ParsleyUrl:                utility.FromStringPtr(a.ParsleyUrl),
		HttpListenAddr:            utility.FromStringPtr(a.HttpListenAddr),
		Secret:                    utility.FromStringPtr(a.Secret),
		DefaultProject:            utility.FromStringPtr(a.DefaultProject),
		CacheTemplates:            a.CacheTemplates,
		CsrfKey:                   utility.FromStringPtr(a.CsrfKey),
		CORSOrigins:               a.CORSOrigins,
		FileStreamingContentTypes: a.FileStreamingContentTypes,
		LoginDomain:               utility.FromStringPtr(a.LoginDomain),
		UserVoice:                 utility.FromStringPtr(a.UserVoice),
		BetaFeatures:              a.BetaFeatures.ToService(),
		StagingEnvironment:        utility.FromStringPtr(a.StagingEnvironment),
	}, nil
}

// RestartTasksResponse is the response model returned from the /admin/restart route
type RestartResponse struct {
	ItemsRestarted []string `json:"items_restarted"`
	ItemsErrored   []string `json:"items_errored"`
}

// BuildFromService builds a model from the service layer
func (ab *APIBanner) BuildFromService(h any) error {
	switch v := h.(type) {
	case APIBanner:
		ab.Text = v.Text
		ab.Theme = v.Theme
	default:
		return errors.Errorf("programmatic error: expected banner config but got type %T", h)
	}
	return nil
}

// ToService is not yet implemented
func (ab *APIBanner) ToService() (any, error) {
	return ab, nil
}

type APIFWSConfig struct {
	URL *string `json:"url"`
}

func (a *APIFWSConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.FWSConfig:
		a.URL = utility.ToStringPtr(v.URL)
	default:
		return errors.Errorf("programmatic error: expected FWS config but got type %T", h)
	}
	return nil
}

func (a *APIFWSConfig) ToService() (interface{}, error) {
	return evergreen.FWSConfig{
		URL: utility.FromStringPtr(a.URL),
	}, nil
}

type APIGraphiteConfig struct {
	CIOptimizationToken *string `json:"ci_optimization_token"`
	ServerURL           *string `json:"server_url"`
}

func (a *APIGraphiteConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.GraphiteConfig:
		a.CIOptimizationToken = utility.ToStringPtr(v.CIOptimizationToken)
		a.ServerURL = utility.ToStringPtr(v.ServerURL)
	default:
		return errors.Errorf("programmatic error: expected Graphite config but got type %T", h)
	}
	return nil
}

func (a *APIGraphiteConfig) ToService() (interface{}, error) {
	return evergreen.GraphiteConfig{
		CIOptimizationToken: utility.FromStringPtr(a.CIOptimizationToken),
		ServerURL:           utility.FromStringPtr(a.ServerURL),
	}, nil
}

// BuildFromService builds a model from the service layer
func (as *APIServiceFlags) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.ServiceFlags:
		as.TaskDispatchDisabled = v.TaskDispatchDisabled
		as.HostInitDisabled = v.HostInitDisabled
		as.LargeParserProjectsDisabled = v.LargeParserProjectsDisabled
		as.MonitorDisabled = v.MonitorDisabled
		as.AlertsDisabled = v.AlertsDisabled
		as.AgentStartDisabled = v.AgentStartDisabled
		as.RepotrackerDisabled = v.RepotrackerDisabled
		as.SchedulerDisabled = v.SchedulerDisabled
		as.CheckBlockedTasksDisabled = v.CheckBlockedTasksDisabled
		as.GithubPRTestingDisabled = v.GithubPRTestingDisabled
		as.CLIUpdatesDisabled = v.CLIUpdatesDisabled
		as.EventProcessingDisabled = v.EventProcessingDisabled
		as.JIRANotificationsDisabled = v.JIRANotificationsDisabled
		as.SlackNotificationsDisabled = v.SlackNotificationsDisabled
		as.EmailNotificationsDisabled = v.EmailNotificationsDisabled
		as.WebhookNotificationsDisabled = v.WebhookNotificationsDisabled
		as.GithubStatusAPIDisabled = v.GithubStatusAPIDisabled
		as.BackgroundStatsDisabled = v.BackgroundStatsDisabled
		as.TaskLoggingDisabled = v.TaskLoggingDisabled
		as.CacheStatsJobDisabled = v.CacheStatsJobDisabled
		as.CacheStatsEndpointDisabled = v.CacheStatsEndpointDisabled
		as.TaskReliabilityDisabled = v.TaskReliabilityDisabled
		as.HostAllocatorDisabled = v.HostAllocatorDisabled
		as.BackgroundReauthDisabled = v.BackgroundReauthDisabled
		as.CloudCleanupDisabled = v.CloudCleanupDisabled
		as.SleepScheduleDisabled = v.SleepScheduleDisabled
		as.StaticAPIKeysDisabled = v.StaticAPIKeysDisabled
		as.JWTTokenForCLIDisabled = v.JWTTokenForCLIDisabled
		as.SystemFailedTaskRestartDisabled = v.SystemFailedTaskRestartDisabled
		as.DegradedModeDisabled = v.CPUDegradedModeDisabled
		as.ElasticIPsDisabled = v.ElasticIPsDisabled
		as.ReleaseModeDisabled = v.ReleaseModeDisabled
		as.LegacyUIAdminPageDisabled = v.LegacyUIAdminPageDisabled
		as.DebugSpawnHostDisabled = v.DebugSpawnHostDisabled
		as.S3LifecycleSyncDisabled = v.S3LifecycleSyncDisabled
		as.PSLoggingDisabled = v.PSLoggingDisabled
		as.UseMergeQueuePathFilteringDisabled = v.UseMergeQueuePathFilteringDisabled
	default:
		return errors.Errorf("programmatic error: expected service flags config but got type %T", h)
	}
	return nil
}

// ToService returns a service model from an API model
func (as *APIServiceFlags) ToService() (any, error) {
	return evergreen.ServiceFlags{
		TaskDispatchDisabled:               as.TaskDispatchDisabled,
		HostInitDisabled:                   as.HostInitDisabled,
		LargeParserProjectsDisabled:        as.LargeParserProjectsDisabled,
		MonitorDisabled:                    as.MonitorDisabled,
		AlertsDisabled:                     as.AlertsDisabled,
		AgentStartDisabled:                 as.AgentStartDisabled,
		RepotrackerDisabled:                as.RepotrackerDisabled,
		SchedulerDisabled:                  as.SchedulerDisabled,
		CheckBlockedTasksDisabled:          as.CheckBlockedTasksDisabled,
		GithubPRTestingDisabled:            as.GithubPRTestingDisabled,
		CLIUpdatesDisabled:                 as.CLIUpdatesDisabled,
		EventProcessingDisabled:            as.EventProcessingDisabled,
		JIRANotificationsDisabled:          as.JIRANotificationsDisabled,
		SlackNotificationsDisabled:         as.SlackNotificationsDisabled,
		EmailNotificationsDisabled:         as.EmailNotificationsDisabled,
		WebhookNotificationsDisabled:       as.WebhookNotificationsDisabled,
		GithubStatusAPIDisabled:            as.GithubStatusAPIDisabled,
		BackgroundStatsDisabled:            as.BackgroundStatsDisabled,
		TaskLoggingDisabled:                as.TaskLoggingDisabled,
		CacheStatsJobDisabled:              as.CacheStatsJobDisabled,
		CacheStatsEndpointDisabled:         as.CacheStatsEndpointDisabled,
		TaskReliabilityDisabled:            as.TaskReliabilityDisabled,
		HostAllocatorDisabled:              as.HostAllocatorDisabled,
		BackgroundReauthDisabled:           as.BackgroundReauthDisabled,
		CloudCleanupDisabled:               as.CloudCleanupDisabled,
		SleepScheduleDisabled:              as.SleepScheduleDisabled,
		StaticAPIKeysDisabled:              as.StaticAPIKeysDisabled,
		JWTTokenForCLIDisabled:             as.JWTTokenForCLIDisabled,
		SystemFailedTaskRestartDisabled:    as.SystemFailedTaskRestartDisabled,
		CPUDegradedModeDisabled:            as.DegradedModeDisabled,
		ElasticIPsDisabled:                 as.ElasticIPsDisabled,
		ReleaseModeDisabled:                as.ReleaseModeDisabled,
		LegacyUIAdminPageDisabled:          as.LegacyUIAdminPageDisabled,
		DebugSpawnHostDisabled:             as.DebugSpawnHostDisabled,
		S3LifecycleSyncDisabled:            as.S3LifecycleSyncDisabled,
		UseMergeQueuePathFilteringDisabled: as.UseMergeQueuePathFilteringDisabled,
		PSLoggingDisabled:                  as.PSLoggingDisabled,
	}, nil
}

// BuildFromService builds a model from the service layer
func (rtr *RestartResponse) BuildFromService(h any) error {
	switch v := h.(type) {
	case *RestartResponse:
		rtr.ItemsRestarted = v.ItemsRestarted
		rtr.ItemsErrored = v.ItemsErrored
	default:
		return errors.Errorf("programmatic error: expected restart response but got type %T", h)
	}
	return nil
}

// ToService is not implemented for /admin/restart
func (rtr *RestartResponse) ToService() (any, error) {
	return nil, errors.New("ToService not implemented for RestartTasksResponse")
}

func AdminDbToRestModel(in evergreen.ConfigSection) (Model, error) {
	id := in.SectionId()
	var out Model
	if id == evergreen.ConfigDocID {
		out = &APIAdminSettings{}
		err := out.BuildFromService(reflect.ValueOf(in).Interface())
		if err != nil {
			return nil, err
		}
	} else {
		structVal := reflect.ValueOf(*NewConfigModel())
		for i := 0; i < structVal.NumField(); i++ {
			// this assumes that the json tag is the same as the section ID
			tag := strings.Split(structVal.Type().Field(i).Tag.Get("json"), ",")[0]
			if tag != id {
				continue
			}

			propName := structVal.Type().Field(i).Name
			propVal := structVal.FieldByName(propName)
			propInterface := propVal.Interface()
			apiModel, ok := propInterface.(Model)
			if !ok {
				return nil, errors.Errorf("could not convert section '%s' to API model interface", id)
			}
			out = apiModel
		}
		if out == nil {
			return nil, errors.Errorf("section '%s' is not defined in the API admin settings", id)
		}
		err := out.BuildFromService(reflect.Indirect(reflect.ValueOf(in)).Interface())
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

type APIJIRANotificationsConfig struct {
	CustomFields map[string]APIJIRANotificationsProject `json:"custom_fields,omitempty"`
}

type APIJIRANotificationsProject struct {
	Fields     map[string]string `json:"fields,omitempty"`
	Components []string          `json:"components,omitempty"`
	Labels     []string          `json:"labels,omitempty"`
}

func (j *APIJIRANotificationsConfig) BuildFromService(h any) error {
	var config *evergreen.JIRANotificationsConfig
	switch v := h.(type) {
	case *evergreen.JIRANotificationsConfig:
		config = v
	case evergreen.JIRANotificationsConfig:
		config = &v
	default:
		return errors.Errorf("programmatic error: expected Jira notifications config but got type %T", h)
	}

	j.CustomFields = make(map[string]APIJIRANotificationsProject)
	for _, project := range config.CustomFields {
		apiProject := APIJIRANotificationsProject{}
		if err := apiProject.BuildFromService(project); err != nil {
			return errors.Wrapf(err, "converting project '%s' to API model", project.Project)
		}

		j.CustomFields[project.Project] = apiProject
	}

	return nil
}

func (j *APIJIRANotificationsConfig) ToService() (any, error) {
	service := evergreen.JIRANotificationsConfig{}
	if len(j.CustomFields) == 0 {
		return service, nil
	}

	// Sort project names alphabetically
	projectNames := make([]string, 0, len(j.CustomFields))
	for projectName := range j.CustomFields {
		projectNames = append(projectNames, projectName)
	}
	sort.Strings(projectNames)

	for _, projectName := range projectNames {
		fields := j.CustomFields[projectName]
		projectIface, err := fields.ToService()
		if err != nil {
			return nil, errors.Errorf("converting project '%s' to service model", projectName)
		}
		project := projectIface.(evergreen.JIRANotificationsProject)

		project.Project = projectName
		service.CustomFields = append(service.CustomFields, project)
	}

	return service, nil
}

func (j *APIJIRANotificationsProject) BuildFromService(h any) error {
	serviceProject, ok := h.(evergreen.JIRANotificationsProject)
	if !ok {
		return errors.Errorf("programmatic error: expected Jira project notifications config but got type %T", h)
	}

	apiFields := make(map[string]string)
	for _, field := range serviceProject.Fields {
		apiFields[field.Field] = field.Template
	}
	j.Fields = apiFields
	j.Components = serviceProject.Components
	j.Labels = serviceProject.Labels

	return nil
}

func (j *APIJIRANotificationsProject) ToService() (any, error) {
	service := evergreen.JIRANotificationsProject{}

	fieldKeys := make([]string, 0, len(j.Fields))
	for key := range j.Fields {
		fieldKeys = append(fieldKeys, key)
	}
	sort.Strings(fieldKeys)

	for _, fieldKey := range fieldKeys {
		template := j.Fields[fieldKey]
		service.Fields = append(service.Fields, evergreen.JIRANotificationsCustomField{Field: fieldKey, Template: template})
	}

	sort.Strings(j.Components)
	service.Components = j.Components

	sort.Strings(j.Labels)
	service.Labels = j.Labels

	return service, nil
}

type APITriggerConfig struct {
	GenerateTaskDistro *string `json:"generate_distro"`
}

func (c *APITriggerConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.TriggerConfig:
		c.GenerateTaskDistro = utility.ToStringPtr(v.GenerateTaskDistro)
	default:
		return errors.Errorf("programmatic error: expected downstream task trigger config but got type %T", h)
	}
	return nil
}
func (c *APITriggerConfig) ToService() (any, error) {
	return evergreen.TriggerConfig{
		GenerateTaskDistro: utility.FromStringPtr(c.GenerateTaskDistro),
	}, nil
}

type APIHostJasperConfig struct {
	BinaryName       *string `json:"binary_name,omitempty"`
	DownloadFileName *string `json:"download_file_name,omitempty"`
	Port             int     `json:"port,omitempty"`
	URL              *string `json:"url,omitempty"`
	Version          *string `json:"version,omitempty"`
}

func (c *APIHostJasperConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.HostJasperConfig:
		c.BinaryName = utility.ToStringPtr(v.BinaryName)
		c.DownloadFileName = utility.ToStringPtr(v.DownloadFileName)
		c.Port = v.Port
		c.URL = utility.ToStringPtr(v.URL)
		c.Version = utility.ToStringPtr(v.Version)
	default:
		return errors.Errorf("programmatic error: expected host Jasper config but got type %T", h)
	}
	return nil
}

func (c *APIHostJasperConfig) ToService() (any, error) {
	return evergreen.HostJasperConfig{
		BinaryName:       utility.FromStringPtr(c.BinaryName),
		DownloadFileName: utility.FromStringPtr(c.DownloadFileName),
		Port:             c.Port,
		URL:              utility.FromStringPtr(c.URL),
		Version:          utility.FromStringPtr(c.Version),
	}, nil
}

type APISpawnHostConfig struct {
	UnexpirableHostsPerUser   *int `json:"unexpirable_hosts_per_user"`
	UnexpirableVolumesPerUser *int `json:"unexpirable_volumes_per_user"`
	SpawnHostsPerUser         *int `json:"spawn_hosts_per_user"`
}

func (c *APISpawnHostConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.SpawnHostConfig:
		c.UnexpirableHostsPerUser = &v.UnexpirableHostsPerUser
		c.UnexpirableVolumesPerUser = &v.UnexpirableVolumesPerUser
		c.SpawnHostsPerUser = &v.SpawnHostsPerUser
	default:
		return errors.Errorf("programmatic error: expected spawn host config but got type %T", h)
	}
	return nil
}

func (c *APISpawnHostConfig) ToService() (any, error) {
	config := evergreen.SpawnHostConfig{
		UnexpirableHostsPerUser:   evergreen.DefaultUnexpirableHostsPerUser,
		UnexpirableVolumesPerUser: evergreen.DefaultUnexpirableVolumesPerUser,
		SpawnHostsPerUser:         evergreen.DefaultMaxSpawnHostsPerUser,
	}
	if c.UnexpirableHostsPerUser != nil {
		config.UnexpirableHostsPerUser = *c.UnexpirableHostsPerUser
	}
	if c.UnexpirableVolumesPerUser != nil {
		config.UnexpirableVolumesPerUser = *c.UnexpirableVolumesPerUser
	}
	if c.SpawnHostsPerUser != nil {
		config.SpawnHostsPerUser = *c.SpawnHostsPerUser
	}

	return config, nil
}

type APIDebugSpawnHostsConfig struct {
	SetupScript *string `json:"setup_script"`
}

func (c *APIDebugSpawnHostsConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.DebugSpawnHostsConfig:
		c.SetupScript = utility.ToStringPtr(v.SetupScript)
	default:
		return errors.Errorf("programmatic error: expected debug spawn hosts config but got type %T", h)
	}
	return nil
}

func (c *APIDebugSpawnHostsConfig) ToService() (any, error) {
	config := evergreen.DebugSpawnHostsConfig{
		SetupScript: utility.FromStringPtr(c.SetupScript),
	}
	return config, nil
}

type APITracerSettings struct {
	Enabled                   *bool   `json:"enabled"`
	CollectorEndpoint         *string `json:"collector_endpoint"`
	CollectorInternalEndpoint *string `json:"collector_internal_endpoint"`
	CollectorAPIKey           *string `json:"collector_api_key"`
}

func (c *APITracerSettings) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.TracerConfig:
		c.Enabled = &v.Enabled
		c.CollectorEndpoint = &v.CollectorEndpoint
		c.CollectorInternalEndpoint = &v.CollectorInternalEndpoint
		c.CollectorAPIKey = &v.CollectorAPIKey
	default:
		return errors.Errorf("programmatic error: expected tracer config but got type %T", h)
	}
	return nil
}

func (c *APITracerSettings) ToService() (any, error) {
	config := evergreen.TracerConfig{
		Enabled:                   utility.FromBoolPtr(c.Enabled),
		CollectorEndpoint:         utility.FromStringPtr(c.CollectorEndpoint),
		CollectorInternalEndpoint: utility.FromStringPtr(c.CollectorInternalEndpoint),
		CollectorAPIKey:           utility.FromStringPtr(c.CollectorAPIKey),
	}

	return config, nil
}

type APIGitHubCheckRunConfig struct {
	CheckRunLimit *int `json:"check_run_limit"`
}

func (c *APIGitHubCheckRunConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.GitHubCheckRunConfig:
		c.CheckRunLimit = utility.ToIntPtr(v.CheckRunLimit)
	default:
		return errors.Errorf("programmatic error: expected GitHub check run config but got type %T", h)
	}
	return nil
}

func (c *APIGitHubCheckRunConfig) ToService() (any, error) {
	config := evergreen.GitHubCheckRunConfig{
		CheckRunLimit: utility.FromIntPtr(c.CheckRunLimit),
	}

	return config, nil
}

type APITaskLimitsConfig struct {
	// MaxTasksPerVersion is the maximum number of tasks that a single version
	// can have.
	MaxTasksPerVersion *int `json:"max_tasks_per_version"`
	// MaxIncludesPerVersion is the maximum number of includes that a single
	// version can have.
	MaxIncludesPerVersion *int `json:"max_includes_per_version"`
	// MaxHourlyPatchTasks is the maximum number of patch tasks a single user can
	// schedule per hour.
	MaxHourlyPatchTasks *int `json:"max_hourly_patch_tasks"`
	// MaxPendingGeneratedTasks is the maximum number of tasks that can be created
	// by all generated task at once.
	MaxPendingGeneratedTasks *int `json:"max_pending_generated_tasks"`
	// MaxGenerateTaskJSONSize is the maximum size of a JSON file in MB that can be specified in the GenerateTasks command.
	MaxGenerateTaskJSONSize *int `json:"max_generate_task_json_size"`
	// MaxConcurrentLargeParserProjectTasks is the maximum number of tasks with parser projects stored in S3 that can be running at once.
	MaxConcurrentLargeParserProjectTasks *int `json:"max_concurrent_large_parser_project_tasks"`
	// MaxDegradedModeConcurrentLargeParserProjectTasks is the maximum number of tasks with parser projects stored in S3 that can be running at once during CPU degraded mode.
	MaxDegradedModeConcurrentLargeParserProjectTasks *int `json:"max_degraded_mode_concurrent_large_parser_project_tasks"`
	// MaxDegradedModeParserProjectSize is the maximum parser project size in MB during CPU degraded mode.
	MaxDegradedModeParserProjectSize *int `json:"max_degraded_mode_parser_project_size"`
	// MaxParserProjectSize is the maximum allowed size in MB for parser projects that are stored in S3.
	MaxParserProjectSize *int `json:"max_parser_project_size"`
	// MaxExecTimeoutSecs is the maximum number of seconds a task can run and set their timeout to.
	MaxExecTimeoutSecs *int `json:"max_exec_timeout_secs"`
	// MaxTaskExecution is the maximum task (zero based) execution number.
	MaxTaskExecution *int `json:"max_task_execution"`
	// MaxDailyAutomaticRestarts is the maximum number of times a project can automatically restart a task within a 24-hour period.
	MaxDailyAutomaticRestarts *int `json:"max_daily_automatic_restarts"`
}

func (c *APITaskLimitsConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.TaskLimitsConfig:
		c.MaxTasksPerVersion = utility.ToIntPtr(v.MaxTasksPerVersion)
		c.MaxIncludesPerVersion = utility.ToIntPtr(v.MaxIncludesPerVersion)
		c.MaxPendingGeneratedTasks = utility.ToIntPtr(v.MaxPendingGeneratedTasks)
		c.MaxHourlyPatchTasks = utility.ToIntPtr(v.MaxHourlyPatchTasks)
		c.MaxGenerateTaskJSONSize = utility.ToIntPtr(v.MaxGenerateTaskJSONSize)
		c.MaxConcurrentLargeParserProjectTasks = utility.ToIntPtr(v.MaxConcurrentLargeParserProjectTasks)
		c.MaxDegradedModeConcurrentLargeParserProjectTasks = utility.ToIntPtr(v.MaxDegradedModeConcurrentLargeParserProjectTasks)
		c.MaxDegradedModeParserProjectSize = utility.ToIntPtr(v.MaxDegradedModeParserProjectSize)
		c.MaxParserProjectSize = utility.ToIntPtr(v.MaxParserProjectSize)
		c.MaxExecTimeoutSecs = utility.ToIntPtr(v.MaxExecTimeoutSecs)
		c.MaxTaskExecution = utility.ToIntPtr(v.MaxTaskExecution)
		c.MaxDailyAutomaticRestarts = utility.ToIntPtr(v.MaxDailyAutomaticRestarts)
		return nil
	default:
		return errors.Errorf("programmatic error: expected task limits config but got type %T", h)
	}
}

func (c *APITaskLimitsConfig) ToService() (any, error) {
	return evergreen.TaskLimitsConfig{
		MaxTasksPerVersion:                               utility.FromIntPtr(c.MaxTasksPerVersion),
		MaxIncludesPerVersion:                            utility.FromIntPtr(c.MaxIncludesPerVersion),
		MaxHourlyPatchTasks:                              utility.FromIntPtr(c.MaxHourlyPatchTasks),
		MaxPendingGeneratedTasks:                         utility.FromIntPtr(c.MaxPendingGeneratedTasks),
		MaxGenerateTaskJSONSize:                          utility.FromIntPtr(c.MaxGenerateTaskJSONSize),
		MaxConcurrentLargeParserProjectTasks:             utility.FromIntPtr(c.MaxConcurrentLargeParserProjectTasks),
		MaxDegradedModeParserProjectSize:                 utility.FromIntPtr(c.MaxDegradedModeParserProjectSize),
		MaxParserProjectSize:                             utility.FromIntPtr(c.MaxParserProjectSize),
		MaxExecTimeoutSecs:                               utility.FromIntPtr(c.MaxExecTimeoutSecs),
		MaxDegradedModeConcurrentLargeParserProjectTasks: utility.FromIntPtr(c.MaxDegradedModeConcurrentLargeParserProjectTasks),
		MaxTaskExecution:                                 utility.FromIntPtr(c.MaxTaskExecution),
		MaxDailyAutomaticRestarts:                        utility.FromIntPtr(c.MaxDailyAutomaticRestarts),
	}, nil
}

type APITestSelectionConfig struct {
	URL *string `json:"url"`
}

func (c *APITestSelectionConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.TestSelectionConfig:
		c.URL = utility.ToStringPtr(v.URL)
		return nil
	default:
		return errors.Errorf("programmatic error: expected test selection config but got type %T", h)
	}
}

func (c *APITestSelectionConfig) ToService() (any, error) {
	return evergreen.TestSelectionConfig{
		URL: utility.FromStringPtr(c.URL),
	}, nil
}

type APIRuntimeEnvironmentsConfig struct {
	BaseURL *string `json:"base_url"`
	APIKey  *string `json:"api_key"`
}

func (a *APIRuntimeEnvironmentsConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.RuntimeEnvironmentsConfig:
		a.BaseURL = utility.ToStringPtr(v.BaseURL)
		a.APIKey = utility.ToStringPtr(v.APIKey)
	default:
		return errors.Errorf("programmatic error: expected Runtime Environments config but got type %T", h)
	}
	return nil
}

func (a *APIRuntimeEnvironmentsConfig) ToService() (any, error) {
	return evergreen.RuntimeEnvironmentsConfig{
		BaseURL: utility.FromStringPtr(a.BaseURL),
		APIKey:  utility.FromStringPtr(a.APIKey),
	}, nil
}

type APIAWSAccountRoleMapping struct {
	Account *string `json:"account"`
	Role    *string `json:"role"`
}

func (a *APIAWSAccountRoleMapping) BuildFromService(v evergreen.AWSAccountRoleMapping) {
	a.Account = utility.ToStringPtr(v.Account)
	a.Role = utility.ToStringPtr(v.Role)
}

func (a *APIAWSAccountRoleMapping) ToService() evergreen.AWSAccountRoleMapping {
	return evergreen.AWSAccountRoleMapping{
		Account: utility.FromStringPtr(a.Account),
		Role:    utility.FromStringPtr(a.Role),
	}
}

type APICostConfig struct {
	FinanceFormula      *float64         `json:"finance_formula"`
	SavingsPlanDiscount *float64         `json:"savings_plan_discount"`
	OnDemandDiscount    *float64         `json:"on_demand_discount"`
	S3Cost              *APIS3CostConfig `json:"s3_cost"`
}

func (a *APICostConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case *evergreen.CostConfig:
		a.FinanceFormula = &v.FinanceFormula
		a.SavingsPlanDiscount = &v.SavingsPlanDiscount
		a.OnDemandDiscount = &v.OnDemandDiscount
		a.S3Cost = &APIS3CostConfig{}
		if err := a.S3Cost.BuildFromService(v.S3Cost); err != nil {
			return errors.Wrap(err, "building S3 cost config")
		}
	case evergreen.CostConfig:
		a.FinanceFormula = &v.FinanceFormula
		a.SavingsPlanDiscount = &v.SavingsPlanDiscount
		a.OnDemandDiscount = &v.OnDemandDiscount
		a.S3Cost = &APIS3CostConfig{}
		if err := a.S3Cost.BuildFromService(v.S3Cost); err != nil {
			return errors.Wrap(err, "building S3 cost config")
		}
	default:
		return errors.Errorf("incorrect type %T", v)
	}
	return nil
}

func (a *APICostConfig) ToService() (any, error) {
	s3Cost := evergreen.S3CostConfig{}
	if a.S3Cost != nil {
		s3CostInterface, err := a.S3Cost.ToService()
		if err != nil {
			return nil, errors.Wrap(err, "converting S3 cost config")
		}
		s3Cost = s3CostInterface.(evergreen.S3CostConfig)
	}
	return evergreen.CostConfig{
		FinanceFormula:      utility.FromFloat64Ptr(a.FinanceFormula),
		SavingsPlanDiscount: utility.FromFloat64Ptr(a.SavingsPlanDiscount),
		OnDemandDiscount:    utility.FromFloat64Ptr(a.OnDemandDiscount),
		S3Cost:              s3Cost,
	}, nil
}

type APIS3CostConfig struct {
	Upload  APIS3UploadCostConfig  `json:"upload"`
	Storage APIS3StorageCostConfig `json:"storage"`
}

func (a *APIS3CostConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.S3CostConfig:
		if err := a.Upload.BuildFromService(v.Upload); err != nil {
			return errors.Wrap(err, "building upload config")
		}
		if err := a.Storage.BuildFromService(v.Storage); err != nil {
			return errors.Wrap(err, "building storage config")
		}
	default:
		return errors.Errorf("incorrect type %T", v)
	}
	return nil
}

func (a *APIS3CostConfig) ToService() (any, error) {
	upload, err := a.Upload.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting upload config")
	}
	storage, err := a.Storage.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting storage config")
	}
	return evergreen.S3CostConfig{
		Upload:  upload.(evergreen.S3UploadCostConfig),
		Storage: storage.(evergreen.S3StorageCostConfig),
	}, nil
}

type APIS3UploadCostConfig struct {
	UploadCostDiscount float64 `json:"upload_cost_discount"`
}

func (a *APIS3UploadCostConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.S3UploadCostConfig:
		a.UploadCostDiscount = v.UploadCostDiscount
		return nil
	default:
		return errors.Errorf("incorrect type %T", v)
	}
}

func (a *APIS3UploadCostConfig) ToService() (any, error) {
	return evergreen.S3UploadCostConfig{
		UploadCostDiscount: a.UploadCostDiscount,
	}, nil
}

type APIS3StorageCostConfig struct {
	StandardStorageCostDiscount float64 `json:"standard_storage_cost_discount"`
	IAStorageCostDiscount       float64 `json:"i_a_storage_cost_discount"`
}

func (a *APIS3StorageCostConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.S3StorageCostConfig:
		a.StandardStorageCostDiscount = v.StandardStorageCostDiscount
		a.IAStorageCostDiscount = v.IAStorageCostDiscount
		return nil
	default:
		return errors.Errorf("incorrect type %T", v)
	}
}

func (a *APIS3StorageCostConfig) ToService() (any, error) {
	return evergreen.S3StorageCostConfig{
		StandardStorageCostDiscount: a.StandardStorageCostDiscount,
		IAStorageCostDiscount:       a.IAStorageCostDiscount,
	}, nil
}

// APISageConfig is the API model for the Sage configuration.
type APISageConfig struct {
	BaseURL *string `json:"base_url"`
}

func (a *APISageConfig) BuildFromService(h any) error {
	switch v := h.(type) {
	case evergreen.SageConfig:
		a.BaseURL = utility.ToStringPtr(v.BaseURL)
	default:
		return errors.Errorf("programmatic error: expected Sage config but got type %T", h)
	}
	return nil
}

func (a *APISageConfig) ToService() (any, error) {
	return evergreen.SageConfig{
		BaseURL: utility.FromStringPtr(a.BaseURL),
	}, nil
}
