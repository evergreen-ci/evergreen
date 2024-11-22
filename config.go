package evergreen

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"time"

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
	ClientVersion = "2024-11-07"

	// Agent version to control agent rollover. The format is the calendar date
	// (YYYY-MM-DD).
	AgentVersion = "2024-11-21"
)

const (
	mongoTimeout        = 5 * time.Minute
	mongoConnectTimeout = 5 * time.Second
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
	Id                  string                    `bson:"_id" json:"id" yaml:"id"`
	Amboy               AmboyConfig               `yaml:"amboy" bson:"amboy" json:"amboy" id:"amboy"`
	AmboyDB             AmboyDBConfig             `yaml:"amboy_db" bson:"amboy_db" json:"amboy_db" id:"amboy_db"`
	Api                 APIConfig                 `yaml:"api" bson:"api" json:"api" id:"api"`
	AuthConfig          AuthConfig                `yaml:"auth" bson:"auth" json:"auth" id:"auth"`
	AWSInstanceRole     string                    `yaml:"aws_instance_role" bson:"aws_instance_role" json:"aws_instance_role"`
	Banner              string                    `bson:"banner" json:"banner" yaml:"banner"`
	BannerTheme         BannerTheme               `bson:"banner_theme" json:"banner_theme" yaml:"banner_theme"`
	Buckets             BucketsConfig             `bson:"buckets" json:"buckets" yaml:"buckets" id:"buckets"`
	Cedar               CedarConfig               `bson:"cedar" json:"cedar" yaml:"cedar" id:"cedar"`
	CommitQueue         CommitQueueConfig         `yaml:"commit_queue" bson:"commit_queue" json:"commit_queue" id:"commit_queue"`
	ConfigDir           string                    `yaml:"configdir" bson:"configdir" json:"configdir"`
	ContainerPools      ContainerPoolsConfig      `yaml:"container_pools" bson:"container_pools" json:"container_pools" id:"container_pools"`
	Database            DBSettings                `yaml:"database" json:"database" bson:"database"`
	DomainName          string                    `yaml:"domain_name" bson:"domain_name" json:"domain_name"`
	Expansions          map[string]string         `yaml:"expansions" bson:"expansions" json:"expansions"`
	ExpansionsNew       util.KeyValuePairSlice    `yaml:"expansions_new" bson:"expansions_new" json:"expansions_new"`
	GithubPRCreatorOrg  string                    `yaml:"github_pr_creator_org" bson:"github_pr_creator_org" json:"github_pr_creator_org"`
	GithubOrgs          []string                  `yaml:"github_orgs" bson:"github_orgs" json:"github_orgs"`
	DisabledGQLQueries  []string                  `yaml:"disabled_gql_queries" bson:"disabled_gql_queries" json:"disabled_gql_queries"`
	HostInit            HostInitConfig            `yaml:"hostinit" bson:"hostinit" json:"hostinit" id:"hostinit"`
	HostJasper          HostJasperConfig          `yaml:"host_jasper" bson:"host_jasper" json:"host_jasper" id:"host_jasper"`
	Jira                JiraConfig                `yaml:"jira" bson:"jira" json:"jira" id:"jira"`
	JIRANotifications   JIRANotificationsConfig   `yaml:"jira_notifications" json:"jira_notifications" bson:"jira_notifications" id:"jira_notifications"`
	KanopySSHKeyPath    string                    `yaml:"kanopy_ssh_key_path" bson:"kanopy_ssh_key_path" json:"kanopy_ssh_key_path"`
	LoggerConfig        LoggerConfig              `yaml:"logger_config" bson:"logger_config" json:"logger_config" id:"logger_config"`
	LogPath             string                    `yaml:"log_path" bson:"log_path" json:"log_path"`
	NewRelic            NewRelicConfig            `yaml:"newrelic" bson:"newrelic" json:"newrelic" id:"newrelic"`
	Notify              NotifyConfig              `yaml:"notify" bson:"notify" json:"notify" id:"notify"`
	Plugins             PluginConfig              `yaml:"plugins" bson:"plugins" json:"plugins"`
	PluginsNew          util.KeyValuePairSlice    `yaml:"plugins_new" bson:"plugins_new" json:"plugins_new"`
	PodLifecycle        PodLifecycleConfig        `yaml:"pod_lifecycle" bson:"pod_lifecycle" json:"pod_lifecycle" id:"pod_lifecycle"`
	PprofPort           string                    `yaml:"pprof_port" bson:"pprof_port" json:"pprof_port"`
	ProjectCreation     ProjectCreationConfig     `yaml:"project_creation" bson:"project_creation" json:"project_creation" id:"project_creation"`
	Providers           CloudProviders            `yaml:"providers" bson:"providers" json:"providers" id:"providers"`
	RepoTracker         RepoTrackerConfig         `yaml:"repotracker" bson:"repotracker" json:"repotracker" id:"repotracker"`
	RuntimeEnvironments RuntimeEnvironmentsConfig `yaml:"runtime_environments" bson:"runtime_environments" json:"runtime_environments" id:"runtime_environments"`
	Scheduler           SchedulerConfig           `yaml:"scheduler" bson:"scheduler" json:"scheduler" id:"scheduler"`
	ServiceFlags        ServiceFlags              `bson:"service_flags" json:"service_flags" id:"service_flags" yaml:"service_flags"`
	SSHKeyDirectory     string                    `yaml:"ssh_key_directory" bson:"ssh_key_directory" json:"ssh_key_directory"`
	SSHKeyPairs         []SSHKeyPair              `yaml:"ssh_key_pairs" bson:"ssh_key_pairs" json:"ssh_key_pairs"`
	Slack               SlackConfig               `yaml:"slack" bson:"slack" json:"slack" id:"slack"`
	SleepSchedule       SleepScheduleConfig       `yaml:"sleep_schedule" bson:"sleep_schedule" json:"sleep_schedule" id:"sleep_schedule"`
	Splunk              SplunkConfig              `yaml:"splunk" bson:"splunk" json:"splunk" id:"splunk"`
	TaskLimits          TaskLimitsConfig          `yaml:"task_limits" bson:"task_limits" json:"task_limits" id:"task_limits"`
	Triggers            TriggerConfig             `yaml:"triggers" bson:"triggers" json:"triggers" id:"triggers"`
	Ui                  UIConfig                  `yaml:"ui" bson:"ui" json:"ui" id:"ui"`
	Spawnhost           SpawnHostConfig           `yaml:"spawnhost" bson:"spawnhost" json:"spawnhost" id:"spawnhost"`
	ShutdownWaitSeconds int                       `yaml:"shutdown_wait_seconds" bson:"shutdown_wait_seconds" json:"shutdown_wait_seconds"`
	Tracer              TracerConfig              `yaml:"tracer" bson:"tracer" json:"tracer" id:"tracer"`
	GitHubCheckRun      GitHubCheckRunConfig      `yaml:"github_check_run" bson:"github_check_run" json:"github_check_run" id:"github_check_run"`
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
			awsInstanceRoleKey:    c.AWSInstanceRole,
			bannerKey:             c.Banner,
			bannerThemeKey:        c.BannerTheme,
			commitQueueKey:        c.CommitQueue,
			configDirKey:          c.ConfigDir,
			domainNameKey:         c.DomainName,
			expansionsKey:         c.Expansions,
			expansionsNewKey:      c.ExpansionsNew,
			githubPRCreatorOrgKey: c.GithubPRCreatorOrg,
			githubOrgsKey:         c.GithubOrgs,
			disabledGQLQueriesKey: c.DisabledGQLQueries,
			kanopySSHKeyPathKey:   c.KanopySSHKeyPath,
			logPathKey:            c.LogPath,
			pprofPortKey:          c.PprofPort,
			pluginsKey:            c.Plugins,
			pluginsNewKey:         c.PluginsNew,
			splunkKey:             c.Splunk,
			sshKeyDirectoryKey:    c.SSHKeyDirectory,
			sshKeyPairsKey:        c.SSHKeyPairs,
			spawnhostKey:          c.Spawnhost,
			shutdownWaitKey:       c.ShutdownWaitSeconds,
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
	if len(c.PluginsNew) > 0 {
		tempPlugins, err := c.PluginsNew.NestedMap()
		if err != nil {
			catcher.Add(errors.Wrap(err, "parsing plugins"))
		}
		c.Plugins = map[string]map[string]interface{}{}
		for k1, v1 := range tempPlugins {
			c.Plugins[k1] = map[string]interface{}{}
			for k2, v2 := range v1 {
				c.Plugins[k1][k2] = v2
			}
		}
	}
	if len(c.SSHKeyPairs) != 0 && c.SSHKeyDirectory == "" {
		catcher.New("cannot use SSH key pairs without setting a directory for them")
	}

	for i := 0; i < len(c.SSHKeyPairs); i++ {
		catcher.NewWhen(c.SSHKeyPairs[i].Name == "", "must specify a name for SSH key pairs")
		catcher.ErrorfWhen(c.SSHKeyPairs[i].Public == "", "must specify a public key for SSH key pair '%s'", c.SSHKeyPairs[i].Name)
		catcher.ErrorfWhen(c.SSHKeyPairs[i].Private == "", "must specify a private key for SSH key pair '%s'", c.SSHKeyPairs[i].Name)
		// Avoid overwriting the filepath stored in Keys, which is a special
		// case for the path to the legacy SSH identity file.
		catcher.ErrorfWhen(c.SSHKeyPairs[i].PrivatePath(c) == c.KanopySSHKeyPath, "cannot overwrite the legacy SSH key at path '%s'", c.KanopySSHKeyPath)

		// ValidateAndDefault can be called before the environment has been
		// initialized.
		if env := GetEnvironment(); env != nil {
			// Ensure we are not modify any existing keys.
			if settings := env.Settings(); settings != nil {
				for _, key := range env.Settings().SSHKeyPairs {
					if key.Name == c.SSHKeyPairs[i].Name {
						catcher.ErrorfWhen(c.SSHKeyPairs[i].Public != key.Public, "cannot modify public key for existing SSH key pair '%s'", key.Name)
						catcher.ErrorfWhen(c.SSHKeyPairs[i].Private != key.Private, "cannot modify private key for existing SSH key pair '%s'", key.Name)
					}
				}
			}
		}
		if c.SSHKeyPairs[i].EC2Regions == nil {
			c.SSHKeyPairs[i].EC2Regions = []string{}
		}
	}
	// ValidateAndDefault can be called before the environment has been
	// initialized.
	if env := GetEnvironment(); env != nil {
		if settings := env.Settings(); settings != nil {
			// Ensure we are not deleting any existing keys.
			for _, key := range GetEnvironment().Settings().SSHKeyPairs {
				var found bool
				for _, newKey := range c.SSHKeyPairs {
					if newKey.Name == key.Name {
						found = true
						break
					}
				}
				catcher.ErrorfWhen(!found, "cannot find existing SSH key '%s'", key.Name)
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

// GetConfig returns the complete Evergreen configuration which is comprised of the shared
// configuration from the config database with overrides from the local [ConfigCollection]
// collection. Use [GetSharedConfig] to get a configuration that reflects only the shared
// configuration.
func GetConfig(ctx context.Context) (*Settings, error) {
	return getSettings(ctx, true)
}

// GetSharedConfig returns only the Evergreen configuration which is shared among all instances
// reading from a single shared database. Use [GetConfig] to get a complete configuration that
// includes overrides from the local database.
func GetSharedConfig(ctx context.Context) (*Settings, error) {
	return getSettings(ctx, false)
}

func getSettings(ctx context.Context, includeOverrides bool) (*Settings, error) {
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

	if catcher.HasErrors() {
		return nil, errors.WithStack(catcher.Resolve())
	}
	return baseConfig, nil
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
			catcher.Add(err)
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
type PluginConfig map[string]map[string]interface{}

// SSHKeyPair represents a public and private SSH key pair.
type SSHKeyPair struct {
	Name    string `bson:"name" json:"name" yaml:"name"`
	Public  string `bson:"public" json:"public" yaml:"public"`
	Private string `bson:"private" json:"private" yaml:"private"`
	// EC2Regions contains all EC2 regions that have stored this SSH key.
	EC2Regions []string `bson:"ec2_regions" json:"ec2_regions" yaml:"ec2_regions"`
}

// AddEC2Region adds the given EC2 region to the set of regions containing the
// SSH key.
func (p *SSHKeyPair) AddEC2Region(region string) error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	query := bson.M{
		idKey: ConfigDocID,
		sshKeyPairsKey: bson.M{
			"$elemMatch": bson.M{
				sshKeyPairNameKey: p.Name,
			},
		},
	}
	var update bson.M
	if len(p.EC2Regions) == 0 {
		// In case this is the first element, we have to push to create the
		// array first.
		update = bson.M{
			"$push": bson.M{bsonutil.GetDottedKeyName(sshKeyPairsKey, "$", sshKeyPairEC2RegionsKey): region},
		}
	} else {
		update = bson.M{
			"$addToSet": bson.M{bsonutil.GetDottedKeyName(sshKeyPairsKey, "$", sshKeyPairEC2RegionsKey): region},
		}
	}
	if _, err := coll.UpdateOne(ctx, query, update); err != nil {
		return errors.WithStack(err)
	}

	if !utility.StringSliceContains(p.EC2Regions, region) {
		p.EC2Regions = append(p.EC2Regions, region)
	}

	return nil
}

func (p *SSHKeyPair) PrivatePath(settings *Settings) string {
	return filepath.Join(settings.SSHKeyDirectory, p.Name)
}

type WriteConcern struct {
	W        int    `yaml:"w"`
	WMode    string `yaml:"wmode"`
	WTimeout int    `yaml:"wtimeout"`
	J        bool   `yaml:"j"`
}

func (wc WriteConcern) Resolve() *writeconcern.WriteConcern {
	opts := []writeconcern.Option{}

	if wc.J {
		opts = append(opts, writeconcern.J(true))
	}
	if wc.WMode == "majority" {
		opts = append(opts, writeconcern.WMajority())
	} else if wc.W > 0 {
		opts = append(opts, writeconcern.W(wc.W))
	}

	if wc.WTimeout > 0 {
		opts = append(opts, writeconcern.WTimeout(time.Duration(wc.WTimeout)*time.Millisecond))
	}

	return writeconcern.New().WithOptions(opts...)
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
		// SetSocketTimeout will be deprecated in future Go driver releases, though at the time being there
		// isn't any other way to enforce a time limit on how long the client waits when trying to R/W data
		// over a connection, so we are including it until Go driver finalizes their timeout API.
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

// supported banner themes in Evergreen
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
