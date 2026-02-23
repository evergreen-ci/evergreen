package evergreen

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen/cloud/parameterstore"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy/logger"
	"github.com/mongodb/anser/apm"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"gopkg.in/yaml.v3"
)

var (
	// BuildRevision should be specified with -ldflags at build time
	BuildRevision = ""

	// ClientVersion is the commandline version string used to control updating
	// the CLI. The format is the calendar date (YYYY-MM-DD).
	ClientVersion = "2026-02-18"

	// Agent version to control agent rollover. The format is the calendar date
	// (YYYY-MM-DD).
	AgentVersion = "2026-02-23a"
)

const (
	mongoTimeout          = 5 * time.Minute
	mongoConnectTimeout   = 5 * time.Second
	parameterStoreTimeout = 30 * time.Second
)

// ConfigSection defines a sub-document in the evergreen config
// any config sections must also be added to the registry in config_sections.go.
type ConfigSection interface {
	// SectionId returns the ID of the section to be used in the database document and struct tag
	SectionId() string
	// Get populates the section from the DB
	Get(context.Context) error
	// Set upserts the section document into the DB
	Set(context.Context) error
	// ValidateAndDefault validates input and sets defaults
	ValidateAndDefault() error
}

// Settings contains all configuration settings for running Evergreen. Settings
// with the "id" struct tag should implement the ConfigSection interface.
type Settings struct {
	Id                  string                  `bson:"_id" json:"id" yaml:"id"`
	Amboy               AmboyConfig             `yaml:"amboy" bson:"amboy" json:"amboy" id:"amboy"`
	AmboyDB             AmboyDBConfig           `yaml:"amboy_db" bson:"amboy_db" json:"amboy_db" id:"amboy_db"`
	Api                 APIConfig               `yaml:"api" bson:"api" json:"api" id:"api"`
	AuthConfig          AuthConfig              `yaml:"auth" bson:"auth" json:"auth" id:"auth"`
	AWSInstanceRole     string                  `yaml:"aws_instance_role" bson:"aws_instance_role" json:"aws_instance_role"`
	Banner              string                  `bson:"banner" json:"banner" yaml:"banner"`
	BannerTheme         BannerTheme             `bson:"banner_theme" json:"banner_theme" yaml:"banner_theme"`
	Buckets             BucketsConfig           `bson:"buckets" json:"buckets" yaml:"buckets" id:"buckets"`
	Cedar               CedarConfig             `bson:"cedar" json:"cedar" yaml:"cedar" id:"cedar"`
	ConfigDir           string                  `yaml:"configdir" bson:"configdir" json:"configdir"`
	ContainerPools      ContainerPoolsConfig    `yaml:"container_pools" bson:"container_pools" json:"container_pools" id:"container_pools"`
	Database            DBSettings              `yaml:"database" json:"database" bson:"database"`
	DebugSpawnHosts     DebugSpawnHostsConfig   `yaml:"debug_spawn_hosts" bson:"debug_spawn_hosts" json:"debug_spawn_hosts" id:"debug_spawn_hosts"`
	DomainName          string                  `yaml:"domain_name" bson:"domain_name" json:"domain_name"`
	Expansions          map[string]string       `yaml:"expansions" bson:"expansions" json:"expansions" secret:"true"`
	ExpansionsNew       util.KeyValuePairSlice  `yaml:"expansions_new" bson:"expansions_new" json:"expansions_new"`
	Cost                CostConfig              `yaml:"cost" bson:"cost" json:"cost" id:"cost"`
	FWS                 FWSConfig               `yaml:"fws" bson:"fws" json:"fws" id:"fws"`
	GithubPRCreatorOrg  string                  `yaml:"github_pr_creator_org" bson:"github_pr_creator_org" json:"github_pr_creator_org"`
	GitHubCheckRun      GitHubCheckRunConfig    `yaml:"github_check_run" bson:"github_check_run" json:"github_check_run" id:"github_check_run"`
	GithubOrgs          []string                `yaml:"github_orgs" bson:"github_orgs" json:"github_orgs"`
	GithubWebhookSecret string                  `yaml:"github_webhook_secret" bson:"github_webhook_secret" json:"github_webhook_secret" secret:"true"`
	Graphite            GraphiteConfig          `yaml:"graphite" bson:"graphite" json:"graphite" id:"graphite"`
	DisabledGQLQueries  []string                `yaml:"disabled_gql_queries" bson:"disabled_gql_queries" json:"disabled_gql_queries"`
	HostInit            HostInitConfig          `yaml:"hostinit" bson:"hostinit" json:"hostinit" id:"hostinit"`
	HostJasper          HostJasperConfig        `yaml:"host_jasper" bson:"host_jasper" json:"host_jasper" id:"host_jasper"`
	Jira                JiraConfig              `yaml:"jira" bson:"jira" json:"jira" id:"jira"`
	JIRANotifications   JIRANotificationsConfig `yaml:"jira_notifications" json:"jira_notifications" bson:"jira_notifications" id:"jira_notifications"`
	// TODO (DEVPROD-15898): remove this key path.
	KanopySSHKeyPath string       `yaml:"kanopy_ssh_key_path" bson:"kanopy_ssh_key_path" json:"kanopy_ssh_key_path"`
	LoggerConfig     LoggerConfig `yaml:"logger_config" bson:"logger_config" json:"logger_config" id:"logger_config"`
	LogPath          string       `yaml:"log_path" bson:"log_path" json:"log_path"`
	Notify           NotifyConfig `yaml:"notify" bson:"notify" json:"notify" id:"notify"`
	// OldestAllowedCLIVersion represents the oldest CLI version that a user can have installed locally. If this field is non-empty, and a user's
	// binary is older than this version, their CLI will prompt them to update before they can continue.
	OldestAllowedCLIVersion string                    `yaml:"oldest_allowed_cli_version" bson:"oldest_allowed_cli_version" json:"oldest_allowed_cli_version"`
	Overrides               OverridesConfig           `yaml:"overrides" bson:"overrides" json:"overrides" id:"overrides"`
	ParameterStore          ParameterStoreConfig      `yaml:"parameter_store" bson:"parameter_store" json:"parameter_store" id:"parameter_store"`
	PerfMonitoringURL       string                    `yaml:"perf_monitoring_url" bson:"perf_monitoring_url" json:"perf_monitoring_url"`
	PerfMonitoringKanopyURL string                    `yaml:"perf_monitoring_kanopy_url" bson:"perf_monitoring_kanopy_url" json:"perf_monitoring_kanopy_url"`
	Plugins                 PluginConfig              `yaml:"plugins" bson:"plugins" json:"plugins"`
	PluginsNew              util.KeyValuePairSlice    `yaml:"plugins_new" bson:"plugins_new" json:"plugins_new"`
	PodLifecycle            PodLifecycleConfig        `yaml:"pod_lifecycle" bson:"pod_lifecycle" json:"pod_lifecycle" id:"pod_lifecycle"`
	PprofPort               string                    `yaml:"pprof_port" bson:"pprof_port" json:"pprof_port"`
	ProjectCreation         ProjectCreationConfig     `yaml:"project_creation" bson:"project_creation" json:"project_creation" id:"project_creation"`
	Providers               CloudProviders            `yaml:"providers" bson:"providers" json:"providers" id:"providers"`
	ReleaseMode             ReleaseModeConfig         `yaml:"release_mode" bson:"release_mode" json:"release_mode" id:"release_mode"`
	RepoTracker             RepoTrackerConfig         `yaml:"repotracker" bson:"repotracker" json:"repotracker" id:"repotracker"`
	RuntimeEnvironments     RuntimeEnvironmentsConfig `yaml:"runtime_environments" bson:"runtime_environments" json:"runtime_environments" id:"runtime_environments"`
	Scheduler               SchedulerConfig           `yaml:"scheduler" bson:"scheduler" json:"scheduler" id:"scheduler"`
	ServiceFlags            ServiceFlags              `bson:"service_flags" json:"service_flags" id:"service_flags" yaml:"service_flags"`
	ShutdownWaitSeconds     int                       `yaml:"shutdown_wait_seconds" bson:"shutdown_wait_seconds" json:"shutdown_wait_seconds"`
	SingleTaskDistro        SingleTaskDistroConfig    `yaml:"single_task_distro" bson:"single_task_distro" json:"single_task_distro" id:"single_task_distro"`
	Slack                   SlackConfig               `yaml:"slack" bson:"slack" json:"slack" id:"slack"`
	SleepSchedule           SleepScheduleConfig       `yaml:"sleep_schedule" bson:"sleep_schedule" json:"sleep_schedule" id:"sleep_schedule"`
	Spawnhost               SpawnHostConfig           `yaml:"spawnhost" bson:"spawnhost" json:"spawnhost" id:"spawnhost"`
	Splunk                  SplunkConfig              `yaml:"splunk" bson:"splunk" json:"splunk" id:"splunk"`
	SSH                     SSHConfig                 `yaml:"ssh" bson:"ssh" json:"ssh" id:"ssh"`
	TaskLimits              TaskLimitsConfig          `yaml:"task_limits" bson:"task_limits" json:"task_limits" id:"task_limits"`
	TestSelection           TestSelectionConfig       `yaml:"test_selection" bson:"test_selection" json:"test_selection" id:"test_selection"`
	Tracer                  TracerConfig              `yaml:"tracer" bson:"tracer" json:"tracer" id:"tracer"`
	Triggers                TriggerConfig             `yaml:"triggers" bson:"triggers" json:"triggers" id:"triggers"`
	Ui                      UIConfig                  `yaml:"ui" bson:"ui" json:"ui" id:"ui"`
	Sage                    SageConfig                `yaml:"sage" bson:"sage" json:"sage" id:"sage"`
}

func (c *Settings) SectionId() string { return ConfigDocID }

func (c *Settings) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

// Set saves the global fields in the configuration (i.e. those that are not
// ConfigSections).
func (c *Settings) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			awsInstanceRoleKey:         c.AWSInstanceRole,
			bannerKey:                  c.Banner,
			bannerThemeKey:             c.BannerTheme,
			configDirKey:               c.ConfigDir,
			domainNameKey:              c.DomainName,
			expansionsKey:              c.Expansions,
			expansionsNewKey:           c.ExpansionsNew,
			githubPRCreatorOrgKey:      c.GithubPRCreatorOrg,
			githubOrgsKey:              c.GithubOrgs,
			githubWebhookSecretKey:     c.GithubWebhookSecret,
			disabledGQLQueriesKey:      c.DisabledGQLQueries,
			kanopySSHKeyPathKey:        c.KanopySSHKeyPath,
			logPathKey:                 c.LogPath,
			oldestAllowedCLIVersionKey: c.OldestAllowedCLIVersion,
			perfMonitoringURLKey:       c.PerfMonitoringURL,
			perfMonitoringKanopyURLKey: c.PerfMonitoringKanopyURL,
			pprofPortKey:               c.PprofPort,
			pluginsKey:                 c.Plugins,
			pluginsNewKey:              c.PluginsNew,
			splunkKey:                  c.Splunk,
			sshKey:                     c.SSH,
			spawnhostKey:               c.Spawnhost,
			shutdownWaitKey:            c.ShutdownWaitSeconds,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *Settings) ValidateAndDefault() error {
	var err error
	catcher := grip.NewSimpleCatcher()
	if c.ConfigDir == "" {
		catcher.Add(errors.New("config directory must not be empty"))
	}

	if len(c.ExpansionsNew) > 0 {
		if c.Expansions, err = c.ExpansionsNew.Map(); err != nil {
			catcher.Add(errors.Wrap(err, "parsing expansions"))
		}
	}

	// Validate that expansion values are not empty
	for key, value := range c.Expansions {
		if value == "" {
			catcher.Add(errors.Errorf("expansion '%s' cannot have an empty value", key))
		}
	}

	if len(c.PluginsNew) > 0 {
		tempPlugins, err := c.PluginsNew.NestedMap()
		if err != nil {
			catcher.Add(errors.Wrap(err, "parsing plugins"))
		}
		c.Plugins = map[string]map[string]any{}
		for k1, v1 := range tempPlugins {
			c.Plugins[k1] = map[string]any{}
			for k2, v2 := range v1 {
				c.Plugins[k1][k2] = v2
			}
		}
	}

	if catcher.HasErrors() {
		return catcher.Resolve()
	}
	if c.LogPath == "" {
		c.LogPath = localLoggingOverride
	}
	if c.ShutdownWaitSeconds < 0 {
		c.ShutdownWaitSeconds = DefaultShutdownWaitSeconds
	}

	return nil
}

// NewSettings builds an in-memory representation of the given settings file.
func NewSettings(filename string) (*Settings, error) {
	configData, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	settings := &Settings{}
	err = yaml.Unmarshal(configData, settings)
	if err != nil {
		return nil, err
	}

	return settings, nil
}

// GetConfig returns the complete Evergreen configuration with overrides applied from
// the [ConfigCollection] collection in the [DB] database. Use [GetRawConfig] to get
// a configuration that doesn't reflect overrides.
func GetConfig(ctx context.Context) (*Settings, error) {
	return getSettings(ctx, true, true)
}

// GetRawConfig returns only the raw Evergreen configuration without applying overrides. Use
// [GetConfig] to get a complete configuration that includes overrides from the [DB] database.
// If there is no [SharedDB] there are no overrides and [GetConfig] and [GetRawConfig] are
// functionally equivalent.
func GetRawConfig(ctx context.Context) (*Settings, error) {
	return getSettings(ctx, false, true)
}

// GetConfigWithoutSecrets returns the Evergreen configuration without secrets, should
// only be used for logging or displaying settings without sensitive information.
func GetConfigWithoutSecrets(ctx context.Context) (*Settings, error) {
	return getSettings(ctx, true, false)
}

func getSettings(ctx context.Context, includeOverrides, includeParameterStore bool) (*Settings, error) {
	config := NewConfigSections()
	if err := config.populateSections(ctx, includeOverrides); err != nil {
		return nil, errors.Wrap(err, "populating sections")
	}

	catcher := grip.NewSimpleCatcher()
	baseConfig := config.Sections[ConfigDocID].(*Settings)
	valConfig := reflect.ValueOf(*baseConfig)
	//iterate over each field in the config struct
	for i := 0; i < valConfig.NumField(); i++ {
		// retrieve the 'id' struct tag
		sectionId := valConfig.Type().Field(i).Tag.Get("id")
		if sectionId == "" { // no 'id' tag means this is a simple field that we can skip
			continue
		}

		// get the property name and find its corresponding section in the registry
		propName := valConfig.Type().Field(i).Name
		section, ok := config.Sections[sectionId]
		if !ok {
			catcher.Add(fmt.Errorf("config section '%s' not found in registry", sectionId))
			continue
		}

		// set the value of the section struct to the value of the corresponding field in the config
		sectionVal := reflect.ValueOf(section).Elem()
		propVal := reflect.ValueOf(baseConfig).Elem().FieldByName(propName)
		if !propVal.CanSet() {
			catcher.Errorf("unable to set field '%s' in section '%s'", propName, sectionId)
			continue
		}
		propVal.Set(sectionVal)
	}

	// If we aren't getting secrets for initialization, read secrets from the parameter store.
	// If it fails, log the error and ignore changes made from the parameter store.
	if includeParameterStore {
		paramConfig := baseConfig
		paramMgr := GetEnvironment().ParameterManager()
		if paramMgr == nil {
			grip.Errorf("parameter manager is nil, cannot read admin secrets from parameter store")
			return baseConfig, nil
		}
		settingsValue := reflect.ValueOf(paramConfig).Elem()
		settingsType := reflect.TypeOf(*paramConfig)
		adminCatcher := grip.NewBasicCatcher()

		paramCache := map[string]string{}
		params, err := paramMgr.Get(ctx, collectSecretPaths(settingsValue, settingsType, "")...)
		if ctx.Err() != nil {
			return nil, errors.Wrap(ctx.Err(), "context is cancelled, cannot get settings")
		} else if err != nil {
			grip.Error(errors.Wrap(err, "getting all admin secrets from parameter store"))
		} else {
			for _, param := range params {
				paramCache[param.Name] = param.Value
			}
		}

		readAdminSecrets(ctx, paramMgr, settingsValue, settingsType, "", paramCache, adminCatcher)
		if adminCatcher.HasErrors() && ctx.Err() == nil {
			grip.Error(errors.Wrap(adminCatcher.Resolve(), "reading admin settings in parameter store"))
		} else {
			baseConfig = paramConfig
		}
	}

	// The context may be cancelled while getting settings.
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "context is cancelled, cannot get settings")
	}
	if catcher.HasErrors() {
		return nil, errors.WithStack(catcher.Resolve())
	}
	return baseConfig, nil
}

func readAdminSecrets(ctx context.Context, paramMgr *parameterstore.ParameterManager, value reflect.Value, typ reflect.Type, path string, paramCache map[string]string, catcher grip.Catcher) {
	if paramMgr == nil {
		catcher.New("parameter manager is nil")
		return
	}
	// No need to go through the recursive loop if we already have errors.
	if catcher.HasErrors() {
		return
	}
	// No need to go through the recursive loop if the context is cancelled.
	if ctx.Err() != nil {
		catcher.Wrap(ctx.Err(), "context is cancelled, cannot read admin secrets")
		return
	}
	// Handle different kinds of values
	switch value.Kind() {
	case reflect.Struct:
		structName := typ.Name()
		currentPath := path
		if structName != "" {
			if currentPath != "" {
				currentPath = currentPath + "/" + structName
			} else {
				currentPath = structName
			}
		}

		// Iterate through all fields in the struct.
		for i := 0; i < value.NumField(); i++ {
			field := typ.Field(i)
			fieldValue := value.Field(i)

			fieldPath := currentPath
			if fieldPath != "" {
				fieldPath = fieldPath + "/" + field.Name
			} else {
				fieldPath = field.Name
			}

			// Check if this field has the secret:"true" tag.
			if secretTag := field.Tag.Get("secret"); secretTag == "true" {
				// If the field is a string, store in parameter manager and update struct with path.
				if fieldValue.Kind() == reflect.String {
					// Check if the field path is already in the cache.
					if cachedValue, ok := paramCache[fieldPath]; ok {
						fieldValue.SetString(cachedValue)
					} else {
						// We don't defer the cancel() and instead cancel it immediately
						// after the parameter store read to avoid context leaks.
						// This is because the recursive calls can create many contexts,
						// and we want to ensure they are all cleaned up properly.
						paramCtx, cancel := context.WithTimeout(ctx, parameterStoreTimeout)
						param, err := paramMgr.Get(paramCtx, fieldPath)
						cancel()
						if err != nil {
							catcher.Wrapf(err, "Failed to read secret field '%s' in parameter store", fieldPath)
						} else if len(param) > 0 {
							// Update the value with the path from the parameter store if it exists.
							fieldValue.SetString(param[0].Value)
						}
					}
					// if the field is a map[string]string, store each key-value pair individually
				} else if fieldValue.Kind() == reflect.Map && fieldValue.Type().Key().Kind() == reflect.String && fieldValue.Type().Elem().Kind() == reflect.String {
					// Create a new map to store the paths
					newMap := reflect.MakeMap(fieldValue.Type())
					for _, key := range fieldValue.MapKeys() {
						mapFieldPath := fmt.Sprintf("%s/%s", fieldPath, key.String())
						// Check if the field path is already in the cache.
						if cachedValue, ok := paramCache[mapFieldPath]; ok {
							newMap.SetMapIndex(key, reflect.ValueOf(cachedValue))
						} else {
							// We don't defer the cancel() and instead cancel it immediately
							// after the parameter store read to avoid context leaks.
							// This is because the recursive calls can create many contexts,
							// and we want to ensure they are all cleaned up properly.
							paramCtx, cancel := context.WithTimeout(ctx, parameterStoreTimeout)
							param, err := paramMgr.Get(paramCtx, mapFieldPath)
							cancel()
							if err != nil {
								catcher.Wrapf(err, "Failed to read secret map field '%s' in parameter store", mapFieldPath)
								continue
							} else if len(param) > 0 {
								// Set the map value to the parameter store value
								newMap.SetMapIndex(key, reflect.ValueOf(param[0].Value))
							}
						}
					}
					// Update the struct field with the new map containing paths
					if len(newMap.MapKeys()) == len(fieldValue.MapKeys()) {
						fieldValue.Set(newMap)
					} else {
						grip.ErrorWhen(ctx.Err() == nil, message.Fields{
							"message":  "readAdminSecrets did not find all map keys in parameter store",
							"path":     fieldPath,
							"keys":     fieldValue.MapKeys(),
							"new_keys": newMap.MapKeys(),
						})
					}
				}
			}

			// Recursively check nested structs, pointers, slices, and maps.
			readAdminSecrets(ctx, paramMgr, fieldValue, field.Type, currentPath, paramCache, catcher)
		}
	case reflect.Ptr:
		// Dereference pointer if not nil.
		if !value.IsNil() {
			readAdminSecrets(ctx, paramMgr, value.Elem(), typ.Elem(), path, paramCache, catcher)
		}
	case reflect.Slice, reflect.Array:
		// Check each element in slice/array.
		for i := 0; i < value.Len(); i++ {
			readAdminSecrets(ctx, paramMgr, value.Index(i), value.Index(i).Type(), fmt.Sprintf("%s/%d", path, i), paramCache, catcher)
		}
	}
}

// collectSecretPaths recursively traverses a struct and collects the paths of all fields
// tagged with "secret":"true". It returns a slice of strings containing these paths.
func collectSecretPaths(value reflect.Value, typ reflect.Type, path string) []string {
	var secretPaths []string

	// Handle different kinds of values
	switch value.Kind() {
	case reflect.Struct:
		structName := typ.Name()
		currentPath := path
		if structName != "" {
			if currentPath != "" {
				currentPath = currentPath + "/" + structName
			} else {
				currentPath = structName
			}
		}

		// Iterate through all fields in the struct.
		for i := 0; i < value.NumField(); i++ {
			field := typ.Field(i)
			fieldValue := value.Field(i)

			fieldPath := currentPath
			if fieldPath != "" {
				fieldPath = fieldPath + "/" + field.Name
			} else {
				fieldPath = field.Name
			}

			// Check if this field has the secret:"true" tag.
			if secretTag := field.Tag.Get("secret"); secretTag == "true" {
				// If the field is a string, add the path to our list.
				if fieldValue.Kind() == reflect.String {
					secretPaths = append(secretPaths, fieldPath)
				} else if fieldValue.Kind() == reflect.Map && fieldValue.Type().Key().Kind() == reflect.String && fieldValue.Type().Elem().Kind() == reflect.String {
					// If the field is a map[string]string, add each key path individually.
					for _, key := range fieldValue.MapKeys() {
						mapFieldPath := fmt.Sprintf("%s/%s", fieldPath, key.String())
						secretPaths = append(secretPaths, mapFieldPath)
					}
				}
			}

			// Recursively check nested structs, pointers, slices, and maps.
			nestedPaths := collectSecretPaths(fieldValue, field.Type, currentPath)
			secretPaths = append(secretPaths, nestedPaths...)
		}
	case reflect.Ptr:
		// Dereference pointer if not nil.
		if !value.IsNil() {
			nestedPaths := collectSecretPaths(value.Elem(), typ.Elem(), path)
			secretPaths = append(secretPaths, nestedPaths...)
		}
	case reflect.Slice, reflect.Array:
		// Check each element in slice/array.
		for i := 0; i < value.Len(); i++ {
			nestedPaths := collectSecretPaths(value.Index(i), value.Index(i).Type(), fmt.Sprintf("%s/%d", path, i))
			secretPaths = append(secretPaths, nestedPaths...)
		}
	}

	return secretPaths
}

// UpdateConfig updates all evergreen settings documents in the DB.
func UpdateConfig(ctx context.Context, config *Settings) error {
	// update the root config document
	if err := config.Set(ctx); err != nil {
		return err
	}

	// update the other config sub-documents
	catcher := grip.NewSimpleCatcher()
	valConfig := reflect.ValueOf(*config)

	//iterate over each field in the config struct
	for i := 0; i < valConfig.NumField(); i++ {
		// retrieve the 'id' struct tag
		sectionId := valConfig.Type().Field(i).Tag.Get("id")
		if sectionId == "" { // no 'id' tag means this is a simple field that we can skip
			continue
		}

		// get the property name and find its value within the settings struct
		propName := valConfig.Type().Field(i).Name
		propVal := valConfig.FieldByName(propName)

		// create a reflective copy of the struct
		valPointer := reflect.Indirect(reflect.New(propVal.Type()))
		valPointer.Set(propVal)

		// convert the pointer to that struct to an empty interface
		propInterface := valPointer.Addr().Interface()

		// type assert to the ConfigSection interface
		section, ok := propInterface.(ConfigSection)
		if !ok {
			catcher.Errorf("unable to convert config section '%s'", propName)
			continue
		}

		catcher.Add(section.Set(ctx))
	}

	return errors.WithStack(catcher.Resolve())
}

// Validate checks the settings and returns nil if the config is valid,
// or an error with a message explaining why otherwise.
func (settings *Settings) Validate() error {
	catcher := grip.NewSimpleCatcher()

	// validate the root-level settings struct
	catcher.Add(settings.ValidateAndDefault())

	// validate each sub-document
	valConfig := reflect.ValueOf(*settings)
	// iterate over each field in the config struct
	for i := 0; i < valConfig.NumField(); i++ {
		// retrieve the 'id' struct tag
		sectionId := valConfig.Type().Field(i).Tag.Get("id")
		if sectionId == "" { // no 'id' tag means this is a simple field that we can skip
			continue
		}

		// get the property name and find its value within the settings struct
		propName := valConfig.Type().Field(i).Name
		propVal := valConfig.FieldByName(propName)

		// the goal is to convert this struct which we know implements ConfigSection
		// from a reflection data structure back to the interface
		// the below creates a copy and takes the address of it as a workaround because
		// you can't take the address of it via reflection for some reason
		// (and all interface methods on the struct have pointer receivers)

		// create a reflective copy of the struct
		valPointer := reflect.Indirect(reflect.New(propVal.Type()))
		valPointer.Set(propVal)

		// convert the pointer to that struct to an empty interface
		propInterface := valPointer.Addr().Interface()

		// type assert to the ConfigSection interface
		section, ok := propInterface.(ConfigSection)
		if !ok {
			catcher.Errorf("unable to convert config section '%s'", propName)
			continue
		}
		err := section.ValidateAndDefault()
		if err != nil {
			catcher.Add(fmt.Errorf("validation failed for section '%s' (field '%s'): %w", sectionId, propName, err))
			continue
		}

		// set the value of the section struct in case there was any defaulting done
		sectionVal := reflect.ValueOf(section).Elem()
		propAddr := reflect.ValueOf(settings).Elem().FieldByName(propName)
		if !propAddr.CanSet() {
			catcher.Errorf("unable to set field '%s' 'in' %s", propName, sectionId)
			continue
		}
		propAddr.Set(sectionVal)
	}
	return errors.WithStack(catcher.Resolve())
}

// GetSender returns the global application-wide loggers. These are special
// universal loggers (distinct from other senders like the GitHub status sender
// or email notification sender) and will be used when grip is invoked to log
// messages in the application (e.g. grip.Info, grip.Error, etc). Because these
// loggers are the main way to send logs in the application, these are essential
// to monitoring the application and therefore have to be set up very early
// during application startup.
func (s *Settings) GetSender(ctx context.Context, env Environment) (send.Sender, error) {
	var (
		sender   send.Sender
		fallback send.Sender
		err      error
		senders  []send.Sender
	)

	levelInfo := s.LoggerConfig.Info()

	fallback, err = send.NewErrorLogger("evergreen.err",
		send.LevelInfo{Default: level.Info, Threshold: level.Debug})
	if err != nil {
		return nil, errors.Wrap(err, "configuring error fallback logger")
	}
	if disableLocalLogging, err := strconv.ParseBool(os.Getenv(disableLocalLoggingEnvVar)); err != nil || !disableLocalLogging {
		// setup the base/default logger (generally direct to systemd
		// or standard output)
		switch s.LogPath {
		case localLoggingOverride:
			// log directly to systemd if possible, and log to
			// standard output otherwise.
			sender = getSystemLogger()
		case standardOutputLoggingOverride, "":
			sender = send.MakeNative()
		default:
			sender, err = send.MakeFileLogger(s.LogPath)
			if err != nil {
				return nil, errors.Wrap(err, "configuring file logger")
			}
		}

		if err = sender.SetLevel(levelInfo); err != nil {
			return nil, errors.Wrap(err, "setting level")
		}
		if err = sender.SetErrorHandler(send.ErrorHandlerFromSender(fallback)); err != nil {
			return nil, errors.Wrap(err, "setting error handler")
		}
		senders = append(senders, sender)
	}

	// set up external log aggregation services:
	//
	if s.Splunk.SplunkConnectionInfo.Populated() {
		retryConf := utility.NewDefaultHTTPRetryConf()
		retryConf.MaxDelay = time.Second
		retryConf.BaseDelay = 10 * time.Millisecond
		retryConf.MaxRetries = 10
		client := utility.GetHTTPRetryableClient(retryConf)

		splunkSender, err := s.makeSplunkSender(ctx, client, levelInfo, fallback)
		if err != nil {
			utility.PutHTTPClient(client)
			return nil, errors.Wrap(err, "configuring splunk logger")
		}

		env.RegisterCloser("splunk-http-client", false, func(_ context.Context) error {
			utility.PutHTTPClient(client)
			return nil
		})
		senders = append(senders, splunkSender)
	}

	// the slack logging service is only for logging very high level alerts.
	if s.Slack.Token != "" && level.FromString(s.Slack.Level).IsValid() {
		sender, err = send.NewSlackLogger(s.Slack.Options, s.Slack.Token,
			send.LevelInfo{Default: level.Critical, Threshold: level.FromString(s.Slack.Level)})
		if err == nil {
			var slackFallback send.Sender

			switch len(senders) {
			case 0:
				slackFallback = fallback
			case 1:
				slackFallback = senders[0]
			default:
				slackFallback = send.NewConfiguredMultiSender(senders...)
			}

			if err = sender.SetErrorHandler(send.ErrorHandlerFromSender(slackFallback)); err != nil {
				return nil, errors.Wrap(err, "setting error handler")
			}

			senders = append(senders, logger.MakeQueueSender(ctx, env.LocalQueue(), sender))
		}
		grip.Warning(errors.Wrap(err, "setting up Slack alert logger"))
	}

	return send.NewConfiguredMultiSender(senders...), nil
}

func (s *Settings) makeSplunkSender(ctx context.Context, client *http.Client, levelInfo send.LevelInfo, fallback send.Sender) (send.Sender, error) {
	sender, err := send.NewSplunkLoggerWithClient("", s.Splunk.SplunkConnectionInfo, grip.GetSender().Level(), client)
	if err != nil {
		return nil, errors.Wrap(err, "making splunk logger")
	}

	if err = sender.SetLevel(levelInfo); err != nil {
		return nil, errors.Wrap(err, "setting Splunk level")
	}

	if err = sender.SetErrorHandler(send.ErrorHandlerFromSender(fallback)); err != nil {
		return nil, errors.Wrap(err, "setting Splunk error handler")
	}

	opts := send.BufferedSenderOptions{
		FlushInterval: time.Duration(s.LoggerConfig.Buffer.DurationSeconds) * time.Second,
		BufferSize:    s.LoggerConfig.Buffer.Count,
	}
	if s.LoggerConfig.Buffer.UseAsync {
		if sender, err = send.NewBufferedAsyncSender(ctx,
			sender,
			send.BufferedAsyncSenderOptions{
				BufferedSenderOptions: opts,
				IncomingBufferFactor:  s.LoggerConfig.Buffer.IncomingBufferFactor,
			}); err != nil {
			return nil, errors.Wrap(err, "making Splunk async buffered sender")
		}
	} else {
		if sender, err = send.NewBufferedSender(ctx, sender, opts); err != nil {
			return nil, errors.Wrap(err, "making Splunk buffered sender")
		}
	}

	return sender, nil
}

// PluginConfig holds plugin-specific settings, which are handled.
// manually by their respective plugins
type PluginConfig map[string]map[string]any

type WriteConcern struct {
	W        int    `yaml:"w"`
	WMode    string `yaml:"wmode"`
	WTimeout int    `yaml:"wtimeout"`
	J        bool   `yaml:"j"`
}

func (wc WriteConcern) Resolve() *writeconcern.WriteConcern {
	concern := &writeconcern.WriteConcern{}
	if wc.WMode == "majority" {
		concern = writeconcern.Majority()
	} else if wc.W > 0 {
		concern.W = wc.W
	}

	if wc.J {
		concern.Journal = utility.TruePtr()
	}

	return concern
}

type ReadConcern struct {
	Level string `yaml:"level"`
}

func (rc ReadConcern) Resolve() *readconcern.ReadConcern {

	if rc.Level == "majority" {
		return readconcern.Majority()
	} else if rc.Level == "local" {
		return readconcern.Local()
	} else if rc.Level == "" {
		return readconcern.Majority()
	} else {
		grip.Error(message.Fields{
			"error":   "ReadConcern Level is not majority or local, setting to majority",
			"rcLevel": rc.Level})
		return readconcern.Majority()
	}
}

type DBSettings struct {
	Url                  string       `yaml:"url"`
	SharedURL            string       `yaml:"shared_url"`
	DB                   string       `yaml:"db"`
	WriteConcernSettings WriteConcern `yaml:"write_concern"`
	ReadConcernSettings  ReadConcern  `yaml:"read_concern"`
	AWSAuthEnabled       bool         `yaml:"aws_auth_enabled"`
}

func (s *DBSettings) mongoOptions(url string) *options.ClientOptions {
	opts := options.Client().ApplyURI(url).SetWriteConcern(s.WriteConcernSettings.Resolve()).
		SetReadConcern(s.ReadConcernSettings.Resolve()).
		SetTimeout(mongoTimeout).
		SetConnectTimeout(mongoConnectTimeout).
		SetSocketTimeout(mongoTimeout).
		SetMonitor(apm.NewMonitor(apm.WithCommandAttributeDisabled(false), apm.WithCommandAttributeTransformer(redactSensitiveCollections)))

	if s.AWSAuthEnabled {
		opts.SetAuth(options.Credential{
			AuthMechanism: awsAuthMechanism,
			AuthSource:    mongoExternalAuthSource,
		})
	}
	return opts
}

// Supported banner themes in Evergreen.
// Empty is a valid banner theme that should not be deleted.
type BannerTheme string

const (
	Announcement BannerTheme = "ANNOUNCEMENT"
	Information  BannerTheme = "INFORMATION"
	Warning      BannerTheme = "WARNING"
	Important    BannerTheme = "IMPORTANT"
	Empty        BannerTheme = ""
)

func IsValidBannerTheme(input string) (bool, BannerTheme) {
	switch input {
	case "":
		return true, ""
	case "ANNOUNCEMENT":
		return true, Announcement
	case "INFORMATION":
		return true, Information
	case "WARNING":
		return true, Warning
	case "IMPORTANT":
		return true, Important
	default:
		return false, ""
	}
}

// StoreAdminSecrets recursively finds all fields tagged with "secret:true"
// and stores them in the parameter manager
// The function has section commented out because all the functionality does not currently exist.
// It is currently only uncommented during the testing to ensure that the function works as expected.
// Those sections will be uncommented/deleted when the functionality is implemented.
func StoreAdminSecrets(ctx context.Context, paramMgr *parameterstore.ParameterManager, value reflect.Value, typ reflect.Type, path string, catcher grip.Catcher) {
	if paramMgr == nil {
		catcher.New("parameter manager is nil")
		return
	}

	// Handle different kinds of values
	switch value.Kind() {
	case reflect.Struct:
		structName := typ.Name()
		currentPath := path
		if structName != "" {
			if currentPath != "" {
				currentPath = currentPath + "/" + structName
			} else {
				currentPath = structName
			}
		}

		// Iterate through all fields in the struct.
		for i := 0; i < value.NumField(); i++ {
			field := typ.Field(i)
			fieldValue := value.Field(i)

			fieldPath := currentPath
			if fieldPath != "" {
				fieldPath = fieldPath + "/" + field.Name
			} else {
				fieldPath = field.Name
			}

			// Check if this field has the secret:"true" tag.
			if secretTag := field.Tag.Get("secret"); secretTag == "true" {
				// If the field is a string, store in parameter manager and update struct with path.
				if fieldValue.Kind() == reflect.String {
					secretValue := fieldValue.String()
					if secretValue == "" {
						continue
					}
					redactedValue, err := putSecretValue(ctx, paramMgr, fieldPath, secretValue)
					if err != nil {
						catcher.Wrapf(err, "Failed to store secret field '%s' in parameter store", fieldPath)
					}
					fieldValue.SetString(redactedValue)
					// if the field is a map[string]string, store each key-value pair individually
				} else if fieldValue.Kind() == reflect.Map && fieldValue.Type().Key().Kind() == reflect.String && fieldValue.Type().Elem().Kind() == reflect.String {
					// Create a new map to store the paths
					newMap := reflect.MakeMap(fieldValue.Type())
					for _, key := range fieldValue.MapKeys() {
						mapFieldPath := fmt.Sprintf("%s/%s", fieldPath, key.String())
						secretValue := fieldValue.MapIndex(key).String()

						redactedValue, err := putSecretValue(ctx, paramMgr, mapFieldPath, secretValue)
						if err != nil {
							catcher.Wrapf(err, "Failed to store secret map field '%s' in parameter store", mapFieldPath)
							continue
						}
						newMap.SetMapIndex(key, reflect.ValueOf(redactedValue))
					}
					fieldValue.Set(newMap)
				}
			}

			// Recursively check nested structs, pointers, slices, and maps.
			StoreAdminSecrets(ctx, paramMgr, fieldValue, field.Type, currentPath, catcher)
		}
	case reflect.Ptr:
		// Dereference pointer if not nil.
		if !value.IsNil() {
			StoreAdminSecrets(ctx, paramMgr, value.Elem(), typ.Elem(), path, catcher)
		}
	case reflect.Slice, reflect.Array:
		// Check each element in slice/array.
		for i := 0; i < value.Len(); i++ {
			StoreAdminSecrets(ctx, paramMgr, value.Index(i), value.Index(i).Type(), fmt.Sprintf("%s/%d", path, i), catcher)
		}
	}
}

// putSecretValue only updates the parameter's value if it is different from the
// current value in Parameter Store. Returns last updated time of the value.
// Necessary to log events correctly in the event log.
func putSecretValue(ctx context.Context, pm *parameterstore.ParameterManager, name, value string) (string, error) {
	// If the parameter already exists and its value matches the new value,
	// return the last updated time without updating it.
	param, err := pm.Get(ctx, name)
	if err != nil {
		return "", errors.Wrapf(err, "getting parameter '%s'", name)
	}
	if len(param) > 0 && param[0].Value == value {
		record, err := parameterstore.FindOneName(ctx, pm.DB, param[0].Name)
		if err != nil {
			return "", errors.Wrapf(err, "finding parameter record for '%s'", param[0].Name)
		}
		if record == nil {
			return "", errors.Errorf("parameter record '%s' not found after put", param[0].Name)
		}
		return record.LastUpdated.String(), nil
	}

	// If the parameter does not exist or has a new value, update it.
	updatedParam, err := pm.Put(ctx, name, value)
	if err != nil {
		return "", errors.Wrapf(err, "putting parameter '%s'", name)
	}
	if updatedParam == nil {
		return "", errors.Errorf("parameter '%s' not found after put", name)
	}
	record, err := parameterstore.FindOneName(ctx, pm.DB, updatedParam.Name)
	if err != nil {
		return "", errors.Wrapf(err, "finding parameter record for '%s'", updatedParam.Name)
	}
	if record == nil {
		return "", errors.Errorf("parameter record '%s' not found after put", updatedParam.Name)
	}
	return record.LastUpdated.String(), nil
}

// UpdateBucketLifecycle updates the lifecycle configuration for an admin-managed bucket in settings.
func UpdateBucketLifecycle(ctx context.Context, bucketField string, expirationDays, transitionToIADays, transitionToGlacierDays *int) error {
	bucketsKey := (&BucketsConfig{}).SectionId()
	set := bson.M{
		bsonutil.GetDottedKeyName(bucketField, bucketConfigLifecycleLastSyncedAtKey): time.Now(),
	}
	if expirationDays != nil {
		set[bsonutil.GetDottedKeyName(bucketField, bucketConfigExpirationDaysKey)] = *expirationDays
	}
	if transitionToIADays != nil {
		set[bsonutil.GetDottedKeyName(bucketField, bucketConfigTransitionToIADaysKey)] = *transitionToIADays
	}
	if transitionToGlacierDays != nil {
		set[bsonutil.GetDottedKeyName(bucketField, bucketConfigTransitionToGlacierDaysKey)] = *transitionToGlacierDays
	}

	return setConfigSection(ctx, bucketsKey, bson.M{
		"$set": set,
		"$unset": bson.M{
			bsonutil.GetDottedKeyName(bucketField, bucketConfigLifecycleSyncErrorKey): "",
		},
	})
}

// UpdateBucketLifecycleError records a sync error for an admin-managed bucket in settings.
func UpdateBucketLifecycleError(ctx context.Context, bucketField string, syncError string) error {
	bucketsKey := (&BucketsConfig{}).SectionId()
	return setConfigSection(ctx, bucketsKey, bson.M{
		"$set": bson.M{
			bsonutil.GetDottedKeyName(bucketField, bucketConfigLifecycleLastSyncedAtKey): time.Now(),
			bsonutil.GetDottedKeyName(bucketField, bucketConfigLifecycleSyncErrorKey):    syncError,
		},
	})
}
