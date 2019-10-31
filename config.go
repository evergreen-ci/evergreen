package evergreen

import (
	"context"
	"fmt"
	"io/ioutil"
	"reflect"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy/logger"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	yaml "gopkg.in/yaml.v2"
)

var (
	// Should be specified with -ldflags at build time
	BuildRevision = ""

	// Commandline Version String; used to control auto-updating.
	ClientVersion = "2019-10-15"
)

// ConfigSection defines a sub-document in the evegreen config
// any config sections must also be added to registry.go
type ConfigSection interface {
	// SectionId() returns the ID of the section to be used in the database document and struct tag
	SectionId() string
	// Get() populates the section from the DB
	Get(Environment) error
	// Set() upserts the section document into the DB
	Set() error
	// ValidateAndDefault() validates input and sets defaults
	ValidateAndDefault() error
}

// Settings contains all configuration settings for running Evergreen. Settings
// with the "id" struct tag should implement the ConfigSection interface.
type Settings struct {
	Id                 string                    `bson:"_id" json:"id" yaml:"id"`
	Alerts             AlertsConfig              `yaml:"alerts" bson:"alerts" json:"alerts" id:"alerts"`
	Amboy              AmboyConfig               `yaml:"amboy" bson:"amboy" json:"amboy" id:"amboy"`
	Api                APIConfig                 `yaml:"api" bson:"api" json:"api" id:"api"`
	ApiUrl             string                    `yaml:"api_url" bson:"api_url" json:"api_url"`
	AuthConfig         AuthConfig                `yaml:"auth" bson:"auth" json:"auth" id:"auth"`
	Banner             string                    `bson:"banner" json:"banner" yaml:"banner"`
	BannerTheme        BannerTheme               `bson:"banner_theme" json:"banner_theme" yaml:"banner_theme"`
	Bugsnag            string                    `yaml:"bugsnag" bson:"bugsnag" json:"bugsnag"`
	ClientBinariesDir  string                    `yaml:"client_binaries_dir" bson:"client_binaries_dir" json:"client_binaries_dir"`
	CommitQueue        CommitQueueConfig         `yaml:"commit_queue" bson:"commit_queue" json:"commit_queue" id:"commit_queue"`
	ConfigDir          string                    `yaml:"configdir" bson:"configdir" json:"configdir"`
	ContainerPools     ContainerPoolsConfig      `yaml:"container_pools" bson:"container_pools" json:"container_pools" id:"container_pools"`
	Credentials        map[string]string         `yaml:"credentials" bson:"credentials" json:"credentials"`
	CredentialsNew     util.KeyValuePairSlice    `yaml:"credentials_new" bson:"credentials_new" json:"credentials_new"`
	Database           DBSettings                `yaml:"database" json:"database" bson:"database"`
	DomainName         string                    `yaml:"domain_name" bson:"domain_name" json:"domain_name"`
	Expansions         map[string]string         `yaml:"expansions" bson:"expansions" json:"expansions"`
	ExpansionsNew      util.KeyValuePairSlice    `yaml:"expansions_new" bson:"expansions_new" json:"expansions_new"`
	GithubPRCreatorOrg string                    `yaml:"github_pr_creator_org" bson:"github_pr_creator_org" json:"github_pr_creator_org"`
	GithubOrgs         []string                  `yaml:"github_orgs" bson:"github_orgs" json:"github_orgs"`
	HostInit           HostInitConfig            `yaml:"hostinit" bson:"hostinit" json:"hostinit" id:"hostinit"`
	HostJasper         HostJasperConfig          `yaml:"host_jasper" bson:"host_jasper" json:"host_jasper" id:"host_jasper"`
	Jira               JiraConfig                `yaml:"jira" bson:"jira" json:"jira" id:"jira"`
	JIRANotifications  JIRANotificationsConfig   `yaml:"jira_notifications" json:"jira_notifications" bson:"jira_notifications" id:"jira_notifications"`
	Keys               map[string]string         `yaml:"keys" bson:"keys" json:"keys"`
	KeysNew            util.KeyValuePairSlice    `yaml:"keys_new" bson:"keys_new" json:"keys_new"`
	LoggerConfig       LoggerConfig              `yaml:"logger_config" bson:"logger_config" json:"logger_config" id:"logger_config"`
	LogPath            string                    `yaml:"log_path" bson:"log_path" json:"log_path"`
	Notify             NotifyConfig              `yaml:"notify" bson:"notify" json:"notify" id:"notify"`
	Plugins            PluginConfig              `yaml:"plugins" bson:"plugins" json:"plugins"`
	PluginsNew         util.KeyValuePairSlice    `yaml:"plugins_new" bson:"plugins_new" json:"plugins_new"`
	PprofPort          string                    `yaml:"pprof_port" bson:"pprof_port" json:"pprof_port"`
	Providers          CloudProviders            `yaml:"providers" bson:"providers" json:"providers" id:"providers"`
	RepoTracker        RepoTrackerConfig         `yaml:"repotracker" bson:"repotracker" json:"repotracker" id:"repotracker"`
	Scheduler          SchedulerConfig           `yaml:"scheduler" bson:"scheduler" json:"scheduler" id:"scheduler"`
	ServiceFlags       ServiceFlags              `bson:"service_flags" json:"service_flags" id:"service_flags" yaml:"service_flags"`
	Slack              SlackConfig               `yaml:"slack" bson:"slack" json:"slack" id:"slack"`
	Splunk             send.SplunkConnectionInfo `yaml:"splunk" bson:"splunk" json:"splunk"`
	SuperUsers         []string                  `yaml:"superusers" bson:"superusers" json:"superusers"`
	Triggers           TriggerConfig             `yaml:"triggers" bson:"triggers" json:"triggers" id:"triggers"`
	Ui                 UIConfig                  `yaml:"ui" bson:"ui" json:"ui" id:"ui"`
}

func (c *Settings) SectionId() string { return ConfigDocID }

func (c *Settings) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = Settings{}
			return nil
		}

		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *Settings) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			apiUrlKey:             c.ApiUrl,
			bannerKey:             c.Banner,
			bannerThemeKey:        c.BannerTheme,
			bugsnagKey:            c.Bugsnag,
			clientBinariesDirKey:  c.ClientBinariesDir,
			commitQueueKey:        c.CommitQueue,
			configDirKey:          c.ConfigDir,
			containerPoolsKey:     c.ContainerPools,
			credentialsKey:        c.Credentials,
			credentialsNewKey:     c.CredentialsNew,
			domainNameKey:         c.DomainName,
			expansionsKey:         c.Expansions,
			expansionsNewKey:      c.ExpansionsNew,
			githubPRCreatorOrgKey: c.GithubPRCreatorOrg,
			githubOrgsKey:         c.GithubOrgs,
			hostJasperKey:         c.HostJasper,
			keysKey:               c.Keys,
			keysNewKey:            c.KeysNew,
			logPathKey:            c.LogPath,
			pprofPortKey:          c.PprofPort,
			pluginsKey:            c.Plugins,
			pluginsNewKey:         c.PluginsNew,
			splunkKey:             c.Splunk,
			superUsersKey:         c.SuperUsers,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *Settings) ValidateAndDefault() error {
	var err error
	catcher := grip.NewSimpleCatcher()
	if c.ApiUrl == "" {
		catcher.Add(errors.New("API hostname must not be empty"))
	}
	if c.ConfigDir == "" {
		catcher.Add(errors.New("Config directory must not be empty"))
	}
	if len(c.CredentialsNew) > 0 {
		if c.Credentials, err = c.CredentialsNew.Map(); err != nil {
			catcher.Add(errors.Wrap(err, "error parsing credentials"))
		}
	}
	if len(c.ExpansionsNew) > 0 {
		if c.Expansions, err = c.ExpansionsNew.Map(); err != nil {
			catcher.Add(errors.Wrap(err, "error parsing expansions"))
		}
	}
	if len(c.KeysNew) > 0 {
		if c.Keys, err = c.KeysNew.Map(); err != nil {
			catcher.Add(errors.Wrap(err, "error parsing keys"))
		}
	}
	if len(c.PluginsNew) > 0 {
		tempPlugins, err := c.PluginsNew.NestedMap()
		if err != nil {
			catcher.Add(errors.Wrap(err, "error parsing plugins"))
		}
		c.Plugins = map[string]map[string]interface{}{}
		for k1, v1 := range tempPlugins {
			c.Plugins[k1] = map[string]interface{}{}
			for k2, v2 := range v1 {
				c.Plugins[k1][k2] = v2
			}
		}
	}
	if catcher.HasErrors() {
		return catcher.Resolve()
	}
	if c.ClientBinariesDir == "" {
		c.ClientBinariesDir = ClientDirectory
	}
	if c.LogPath == "" {
		c.LogPath = LocalLoggingOverride
	}
	return nil
}

// NewSettings builds an in-memory representation of the given settings file.
func NewSettings(filename string) (*Settings, error) {
	configData, err := ioutil.ReadFile(filename)
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

// GetConfig retrieves the Evergreen config document. If no document is
// present in the DB, it will return the defaults
func GetConfig() (*Settings, error) { return BootstrapConfig(GetEnvironment()) }

// Bootstrap config gets a config from the database defined in the environment.
func BootstrapConfig(env Environment) (*Settings, error) {
	config := &Settings{}

	// retrieve the root config document
	if err := config.Get(env); err != nil {
		return nil, err
	}

	// retrieve the other config sub-documents and form the whole struct
	catcher := grip.NewSimpleCatcher()
	sections := ConfigRegistry.GetSections()
	valConfig := reflect.ValueOf(*config)
	//iterate over each field in the config struct
	for i := 0; i < valConfig.NumField(); i++ {
		// retrieve the 'id' struct tag
		sectionId := valConfig.Type().Field(i).Tag.Get("id")
		if sectionId == "" { // no 'id' tag means this is a simple field that we can skip
			continue
		}

		// get the property name and find its corresponding section in the registry
		propName := valConfig.Type().Field(i).Name
		section, ok := sections[sectionId]
		if !ok {
			catcher.Add(fmt.Errorf("config section %s not found in registry", sectionId))
			continue
		}

		// retrieve the section's document from the db
		if err := section.Get(env); err != nil {
			catcher.Add(errors.Wrapf(err, "error populating section %s", sectionId))
			continue
		}

		// set the value of the section struct to the value of the corresponding field in the config
		sectionVal := reflect.ValueOf(section).Elem()
		propVal := reflect.ValueOf(config).Elem().FieldByName(propName)
		if !propVal.CanSet() {
			catcher.Add(fmt.Errorf("unable to set field %s in %s", propName, sectionId))
			continue
		}
		propVal.Set(sectionVal)
	}

	if catcher.HasErrors() {
		return nil, errors.WithStack(catcher.Resolve())
	}
	return config, nil

}

// UpdateConfig updates all evergreen settings documents in DB
func UpdateConfig(config *Settings) error {
	// update the root config document
	if err := config.Set(); err != nil {
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
			catcher.Add(fmt.Errorf("unable to convert config section %s", propName))
			continue
		}
		catcher.Add(section.Set())
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
			catcher.Add(fmt.Errorf("unable to convert config section %s", propName))
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
			catcher.Add(fmt.Errorf("unable to set field %s in %s", propName, sectionId))
			continue
		}
		propAddr.Set(sectionVal)
	}
	return errors.WithStack(catcher.Resolve())
}

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
		return nil, errors.Wrap(err, "problem configuring err fallback logger")
	}

	// setup the base/default logger (generally direct to systemd
	// or standard output)
	switch s.LogPath {
	case LocalLoggingOverride:
		// log directly to systemd if possible, and log to
		// standard output otherwise.
		sender = getSystemLogger()
	case StandardOutputLoggingOverride, "":
		sender = send.MakeNative()
	default:
		sender, err = send.MakeFileLogger(s.LogPath)
		if err != nil {
			return nil, errors.Wrap(err, "could not configure file logger")
		}
	}

	if err = sender.SetLevel(levelInfo); err != nil {
		return nil, errors.Wrap(err, "problem setting level")
	}
	if err = sender.SetErrorHandler(send.ErrorHandlerFromSender(fallback)); err != nil {
		return nil, errors.Wrap(err, "problem setting error handler")
	}
	senders = append(senders, sender)

	// set up external log aggregation services:
	//
	if endpoint, ok := s.Credentials["sumologic"]; ok {
		sender, err = send.NewSumo("", endpoint)
		if err == nil {
			if err = sender.SetLevel(levelInfo); err != nil {
				return nil, errors.Wrap(err, "problem setting level")
			}
			if err = sender.SetErrorHandler(send.ErrorHandlerFromSender(fallback)); err != nil {
				return nil, errors.Wrap(err, "problem setting error handler")
			}
			senders = append(senders,
				send.NewBufferedSender(sender,
					time.Duration(s.LoggerConfig.Buffer.DurationSeconds)*time.Second,
					s.LoggerConfig.Buffer.Count))
		}
	}

	if s.Splunk.Populated() {
		sender, err = send.NewSplunkLogger("", s.Splunk, grip.GetSender().Level())
		if err == nil {
			if err = sender.SetLevel(levelInfo); err != nil {
				return nil, errors.Wrap(err, "problem setting level")
			}
			if err = sender.SetErrorHandler(send.ErrorHandlerFromSender(fallback)); err != nil {
				return nil, errors.Wrap(err, "problem setting error handler")
			}
			senders = append(senders,
				send.NewBufferedSender(sender,
					time.Duration(s.LoggerConfig.Buffer.DurationSeconds)*time.Second,
					s.LoggerConfig.Buffer.Count))
		}
	}

	// the slack logging service is only for logging very high level alerts.
	if s.Slack.Token != "" {
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
				return nil, errors.Wrap(err, "problem setting error handler")
			}

			senders = append(senders, logger.MakeQueueSender(ctx, env.LocalQueue(), sender))
		}
		grip.Warning(errors.Wrap(err, "problem setting up slack alert logger"))
	}

	return send.NewConfiguredMultiSender(senders...), nil
}

func (s *Settings) GetGithubOauthString() (string, error) {
	token, ok := s.Credentials["github"]
	if ok && token != "" {
		return token, nil
	}

	return "", errors.New("no github token in settings")
}

func (s *Settings) GetGithubOauthToken() (string, error) {
	if s == nil {
		return "", errors.New("not defined")
	}

	oauthString, err := s.GetGithubOauthString()
	if err != nil {
		return "", err
	}
	splitToken := strings.Split(oauthString, " ")
	if len(splitToken) != 2 || splitToken[0] != "token" {
		return "", errors.New("token format was invalid, expected 'token [token]'")
	}
	return splitToken[1], nil
}

func GetServiceFlags() (*ServiceFlags, error) {
	section := ConfigRegistry.GetSection("service_flags")
	if section == nil {
		return nil, errors.New("unable to retrieve config section")
	}
	if err := section.Get(GetEnvironment()); err != nil {
		return nil, errors.Wrap(err, "error retrieving section from DB")
	}
	flags, ok := section.(*ServiceFlags)
	if !ok {
		return nil, errors.New("unable to convert config section to service flags")
	}
	return flags, nil
}

// PluginConfig holds plugin-specific settings, which are handled.
// manually by their respective plugins
type PluginConfig map[string]map[string]interface{}

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

type DBSettings struct {
	Url                  string       `yaml:"url"`
	DB                   string       `yaml:"db"`
	WriteConcernSettings WriteConcern `yaml:"write_concern"`
}

// supported banner themes in Evergreen
type BannerTheme string

const (
	Announcement BannerTheme = "announcement"
	Information              = "information"
	Warning                  = "warning"
	Important                = "important"
)

func IsValidBannerTheme(input string) (bool, BannerTheme) {
	switch input {
	case "":
		return true, ""
	case "announcement":
		return true, Announcement
	case "information":
		return true, Information
	case "warning":
		return true, Warning
	case "important":
		return true, Important
	default:
		return false, ""
	}
}
